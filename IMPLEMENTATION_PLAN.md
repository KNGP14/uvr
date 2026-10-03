# Implementation Plan: anstehende Code-Änderungen uvr2json

Stand: 2026-10-03. Entstanden aus der Analyse des Fehlers `write fd N: no buffer space available` (ENOBUFS).
Bereits umgesetzt (Branch `fix/enobufs-diagnose`): korrekte Beschriftung Eingang/Ausgang, Auswertung des Connect-Fehlers,
nicht blockierender Heartbeat-Stopp, Erkennung von ENOBUFS mit Wiederholung und klarer Meldung, robuste PID-Datei.

**Grundsatz für alle Punkte:** keine neuen Frame-Typen, keine Schreibzugriffe auf die UVR. Jede Änderung wird mit einem simulierten
CAN-Bus getestet (ohne Socket), bevor sie auf den Pi kommt. Deploy auf den Pi nur nach ausdrücklicher Freigabe.

| # | Thema | Priorität | Aufwand |
|---|---|---|---|
| 1 | Warte-/Request-Logik der Bibliothek (veraltete Waiter blockieren den Empfang) | hoch | mittel |
| 2 | `log.Fatal` in `canopen/sdo` (Prozessabbruch ohne Aufräumen) | hoch (durch 1 erledigt) | – |
| 3 | Data Race beim An-/Abmelden von Handlern in `can.Bus` | mittel (mit 1) | klein |
| 4 | Schreibfunktionen aus dem Paket entfernen | mittel | klein |
| 5 | `data.json` atomar schreiben | niedrig | klein |
| 6 | `go vet`-Warnung in `conn_management.go` | niedrig | klein |
| 7 | Abhängigkeiten modernisieren bzw. übernehmen | niedrig | mittel |

---

## 1. Warte-/Request-Logik der Bibliothek

**Problem:** `can.Wait` (github.com/brutella/can v0.0.2, `wait.go`) meldet den Waiter nur ab, wenn ein passender Frame ankommt,
aber **nicht** bei Timeout und nicht, wenn das Senden fehlschlägt (`canopen/client.go:19-23`). Der veraltete Waiter bleibt registriert.
Der nächste Frame mit derselben COB-ID (z. B. `0x5D0`, SDO-Antwort an Knoten 16) geht an ihn:

- `waiter.Handle` ruft `w.wait <- …` auf. Die zugehörige Goroutine hat nach dem Timeout bereits auf `ch` gesendet oder blockiert dort,
  es gibt also keinen Empfänger mehr. `Handle` blockiert **dauerhaft**.
- `Handle` läuft synchron in der Empfangs-Goroutine des Busses (`bus.go: publishNextFrame → publish`). Damit steht der Empfang
  für den Rest der Lebenszeit dieses Busses.
- Die neue Anfrage bekommt ihre Antwort nie und läuft in den Timeout.

**Folge:** Nach einem **einzigen** SDO-Timeout oder Sendefehler scheitern alle weiteren Abfragen dieses Knotens im selben Lauf.
Das erklärt Fehlerketten („Abbruch aufgrund zu vieler Fehler“) im Normalbetrieb. Der Stillstand des CAN-Controllers wird dadurch
**nicht** erklärt (der liegt in der Kernel-Queue bzw. im MCP2515). Zusätzlich verbraucht jede verwaiste Goroutine Speicher bis Prozessende.

**Vorschlag:** Eigene Request-Funktion im Paket `uvr`, die `canopen.Client.Do` und `sdo.Upload.Do` ersetzt:

```go
// request sendet frm und wartet auf den ersten Frame mit respID.
// Der Handler wird in jedem Fall wieder abgemeldet (Antwort, Timeout, Sendefehler).
func request(bus *can.Bus, frm canopen.Frame, respID uint32, timeout time.Duration) (canopen.Frame, error)
```

- Antwortkanal **gepuffert** (Kapazität 1), Senden im Handler nicht blockierend (`select { case ch <- f: default: }`).
- Abmelden per `defer`, auch bei Sendefehler und Timeout.
- SDO-Upload (Initiate + Segmente) im Paket `uvr` nachbauen (ca. 60 Zeilen, nur **Upload**, kein Download).
  Unerwartete Antworten liefern einen Fehler statt `log.Fatal` (siehe 2).
- `sendConnManagementData`, `ReadFromIndex` und `ReadStringAtIndex` auf `request` umstellen.

**Tests (Fake-Bus wie in `client_test.go`):**
- Timeout bei Anfrage 1, danach gelingt Anfrage 2 mit derselben COB-ID.
- Verspätete Antwort auf Anfrage 1 kommt an, nachdem Anfrage 2 gesendet wurde: Anfrage 2 bekommt nicht die falsche Antwort
  (Toggle-Bit/Index prüfen) bzw. der Fehler wird erkannt.
- Sendefehler (ENOBUFS): Kein Handler bleibt registriert.
- Abort-Antwort des Servers (`0x80`) liefert einen Fehler, kein Prozessabbruch.
- Nur Upload-Kommandos (`0x40`, `0x60`/`0x70`) auf `0x640+id`.

**CAN-Auswirkung:** gleiche Frames wie bisher. Eher **weniger** Verkehr, weil Fehlerketten mit wiederholten Timeouts entfallen.

## 2. `log.Fatal` in `canopen/sdo/upload.go`

**Problem:** `upload.go:59` (`log.Fatalf` bei unerwartetem Command Specifier) und `upload.go:100` (`log.Fatal` bei Timeout eines Segments)
beenden den Prozess sofort. `data.json` wird dann nicht geschrieben, und die PID-Datei bleibt liegen. Letzteres fängt der neue Code
(veraltete PID-Datei wird erkannt) bereits ab.

**Vorschlag:** Mit Punkt 1 erledigt, weil `sdo.Upload` nicht mehr verwendet wird.
**Test:** Segment-Timeout und unerwartete Antwort führen zu einem Eintrag in `Fehler`, `data.json` wird geschrieben.

## 3. Data Race beim An-/Abmelden von Handlern

**Problem:** `can.Bus.Subscribe/Unsubscribe` ändern das Slice `handler` ohne Lock. Gleichzeitig iteriert die Empfangs-Goroutine
darüber (`publish`), und `waiter.Handle` meldet sich während dieser Iteration selbst ab. Das ist ein Data Race und kann Handler überspringen.

**Vorschlag:** Im Zuge von 1 einen eigenen, per Mutex geschützten Dispatcher im Paket `uvr` verwenden: genau **ein**
`bus.SubscribeFunc`, das intern über eine Map `COB-ID → wartende Anfrage` verteilt.
**Test:** `go test -race` mit parallelen Anfragen und Heartbeat.

## 4. Schreibfunktionen aus dem Paket entfernen

**Problem:** `write.go` (`WriteToIndex`, SDO-Download) und `Client.Write` sind im Paket vorhanden, werden von `uvr2json` aber nicht genutzt.
Heute garantiert nur „wird nicht aufgerufen“, dass nichts zur UVR geschrieben wird.

**Vorschlag:** `write.go` und `Client.Write` löschen. Damit ist strukturell ausgeschlossen, dass ein späterer Code-Pfad
Parameter in der UVR ändert. Optional einen Test ergänzen, der alle gesendeten COB-IDs/Kommandos gegen eine Whitelist prüft
(Heartbeat `0x710`, Verbindung `0x410`, SDO-Upload `0x650`).
**Risiko:** keins, sofern niemand außerhalb von `uvr2json` das Paket zum Schreiben nutzt.

## 5. `data.json` atomar schreiben

**Problem:** `os.Create` kürzt die Datei sofort und schreibt danach. Liest Home Assistant genau in diesem Moment, bekommt es leeres oder halbes JSON.
Zusätzlich hängt der Cron nach erfolgreichem Lauf eine Leerzeile an (`&& echo "" >> …`).

**Vorschlag:** In `data.json.tmp` im selben Verzeichnis schreiben, `Sync`, dann `os.Rename`. Rechte explizit `0644`.
Die angehängte Leerzeile im Cron wird dann überflüssig (Cron-Änderung = Freigabe am Pi).
**Test:** Ausgabe in `t.TempDir()`, Datei ist immer gültiges JSON, Rechte 0644.

## 6. `go vet`-Warnung

`conn_management.go:40`: `canopen.Client{bus, time.Second * 2}` mit Feldnamen schreiben (`Bus:`, `Timeout:`). Entfällt ggf. mit 1.

## 7. Abhängigkeiten

`brutella/can` und `brutella/canopen` (v0.0.2) werden nicht mehr gepflegt, `golang.org/x/sys` ist von 2018.
**Vorschlag:** Nach 1 bis 3 werden aus `canopen` nur noch Typen (`Frame`, `ObjectIndex`) benötigt. Diese und den Socket-Code
(`readwritecloser_linux.go`, ca. 20 Zeilen) als `internal/` übernehmen, `x/sys` aktualisieren. Build und Tests offline prüfen.

---

## Außerhalb des Codes (Betrieb auf dem Pi, jeweils Freigabe nötig)

- Watchdog: Stillstand erkennen (qdisc `backlog` > 0 und `tx_packets` unverändert über mehrere Läufe) und Besitzer benachrichtigen bzw. `can0` neu starten.
- `berr-reporting on` für aussagekräftige Fehlerzähler.
- Hardware-Ursache des Controller-Stillstands klären (MCP2515-Modul, Quarz, Transceiver, Spannungsversorgung). Siehe Analysebericht.
