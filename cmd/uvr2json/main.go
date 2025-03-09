package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/uvr"
)

type inletStruct struct {
	EingangID   int     `json:"Eingang-ID"`
	Bezeichnung string  `json:"Bezeichnung"`
	Modus       string  `json:"Modus"`
	Wert        float32 `json:"Wert"`
}

type outletStruct struct {
	AusgangID   int    `json:"Ausgang-ID"`
	Bezeichnung string `json:"Bezeichnung"`
	Modus       string `json:"Modus"`
	Wert        string `json:"Wert"`
}

type serverStruct struct {
	KnotenID  int            `json:"Knoten-ID"`
	Eingaenge []inletStruct  `json:"Eingänge"`
	Ausgaenge []outletStruct `json:"Ausgänge"`
}

type DataStruct struct {
	Zeitstempel time.Time      `json:"Zeitstempel"`
	Fehler      []string       `json:"Fehler"`
	Knoten      []serverStruct `json:"Knoten"`
}

func readOutlet(outlet uvr.Outlet, client *uvr.Client) (return_description string, return_mode string, return_value string, return_error error) {
	return_error = nil

	if value, err := client.Read(outlet.Description); err == nil {
		return_description = value.(string)
	} else {
		return_error = fmt.Errorf("Beschreibung konnte nicht abgerufen werden (%s)", err)
		return
	}

	if value, err := client.Read(outlet.Mode); err == nil {
		return_mode = value.(string)
	}

	if value, err := client.Read(outlet.State); err == nil {
		return_value = value.(string)
	} else {
		return_error = fmt.Errorf("Wert konnte nicht abgerufen werden (%s)", err)
		return
	}

	return
}

func readInlet(inlet uvr.Inlet, client *uvr.Client) (return_description string, return_state string, return_value float32, return_error error) {
	return_error = nil

	if value, err := client.Read(inlet.Description); err == nil {
		return_description = value.(string)
	} else {
		return_error = fmt.Errorf("Beschreibung konnte nicht abgerufen werden (%s)", err)
		return
	}

	if value, err := client.Read(inlet.State); err == nil {
		return_state = value.(string)
	} else {
		return_error = fmt.Errorf("Status konnte nicht abgerufen werden (%s)", err)
		return
	}

	if value, err := client.Read(inlet.Value); err == nil {
		if float, ok := value.(float32); ok {
			return_value = float
		}
	} else {
		return_error = fmt.Errorf("Wert konnte nicht abgerufen werden (%s)", err)
		return
	}

	return
}

func readOutlets(client *uvr.Client, serverid int, verbose bool) (outletData []outletStruct, errors []error) {
	outlets := []uvr.Outlet{
		uvr.NewOutlet(0x1),
		uvr.NewOutlet(0x2),
		uvr.NewOutlet(0x3),
		uvr.NewOutlet(0x4),
		uvr.NewOutlet(0x5),
		uvr.NewOutlet(0x6),
		uvr.NewOutlet(0x7),
		uvr.NewOutlet(0x8),
		uvr.NewOutlet(0x9),
		uvr.NewOutlet(0xa),
		uvr.NewOutlet(0xb),
		uvr.NewOutlet(0xc),
		uvr.NewOutlet(0xd),
	}

	for index, outlet := range outlets {

		if len(errors) > 2 {
			errors = append(errors, fmt.Errorf("Abbruch aufgrund zu vieler Fehler beim Abfragen der Eingänge"))
			return
		}

		descr, mode, val, err := readOutlet(outlet, client)

		if err == nil {

			outlet := outletStruct{
				AusgangID:   index + 1,
				Bezeichnung: descr,
				Modus:       mode,
				Wert:        val,
			}
			outletData = append(outletData, outlet)

			if verbose {
				log.Printf("KNOTEN: \"%d\", AUSGANG: \"%d\", BEZEICHNUNG: \"%s\", MODUS: \"%s\", WERT: \"%s\"", serverid, index+1, descr, mode, val)
			}

		} else {
			errors = append(errors, fmt.Errorf("Fehler bei Eingang %d: %s", index+1, err))
		}
	}

	return

}

func readInlets(client *uvr.Client, serverid int, verbose bool) (inletData []inletStruct, errors []error) {

	inlets := []uvr.Inlet{
		uvr.NewInlet(0x1),
		uvr.NewInlet(0x2),
		uvr.NewInlet(0x3),
		uvr.NewInlet(0x4),
		uvr.NewInlet(0x5),
		uvr.NewInlet(0x6),
		uvr.NewInlet(0x7),
		uvr.NewInlet(0x8),
		uvr.NewInlet(0x9),
		uvr.NewInlet(0xa),
		uvr.NewInlet(0xb),
		uvr.NewInlet(0xc),
		uvr.NewInlet(0xd),
		uvr.NewInlet(0xe),
		uvr.NewInlet(0xf),
		uvr.NewInlet(0x10),
	}

	for index, inlet := range inlets {

		if len(errors) > 2 {
			errors = append(errors, fmt.Errorf("Abbruch aufgrund zu vieler Fehler beim Abfragen der Ausgänge"))
			return
		}

		descr, state, val, err := readInlet(inlet, client)

		if err == nil {

			inlet := inletStruct{
				EingangID:   index + 1,
				Bezeichnung: descr,
				Modus:       state,
				Wert:        val,
			}
			inletData = append(inletData, inlet)

			if verbose {
				log.Printf("KNOTEN: \"%d\", EINGANG: \"%d\", BEZEICHNUNG: \"%s\", MODUS: \"%s\", WERT: \"%f\"", serverid, index+1, descr, state, val)
			}

		} else {
			errors = append(errors, fmt.Errorf("Fehler bei Ausgang %d: %s", index+1, err))
		}
	}

	return

}

func disconnectWithTimeout(client *uvr.Client, uvrID uint8) error {
	errChan := make(chan error, 1)

	// Starte Disconnect in einer Goroutine
	go func() {
		errChan <- client.Disconnect(uvrID)
	}()

	select {
	case err := <-errChan:
		// Disconnect abgeschlossen
		if err != nil {
			return fmt.Errorf("beim Trennen von %d ist ein Fehler aufgetreten (%v)", uvrID, err)
		}
		return nil
	case <-time.After(5 * time.Second):
		// Timeout erreicht
		return fmt.Errorf("beim Trennen von %d ist nach 5 Sekunden der Timeout abgelaufen", uvrID)
	}
}

func getServerData(client *uvr.Client, serverId int, verbose bool) (serverData serverStruct, errors []error) {

	// Verbindung zur UVR aufbauen
	uvrID := uint8(serverId)
	client.Connect(uvrID)

	// Rückgabe vorbereiten
	var inletData []inletStruct
	var outletData []outletStruct

	// Eingänge abfragen
	inlets, inletsErrors := readInlets(client, serverId, verbose)
	if len(inletsErrors) == 0 {
		inletData = inlets

		// Ausgänge abfragen
		outlets, outletsErrors := readOutlets(client, serverId, verbose)
		if len(outletsErrors) == 0 {
			outletData = outlets
		} else {
			errors = append(errors, outletsErrors...)
		}

	} else {
		errors = append(errors, inletsErrors...)
	}

	// Knoten-Daten hinzufügen
	serverData = serverStruct{
		KnotenID:  serverId,
		Eingaenge: inletData,
		Ausgaenge: outletData,
	}

	// Verbindung zur UVR trennen
	err := disconnectWithTimeout(client, uvrID)
	if err != nil {
		errors = append(errors, fmt.Errorf("Fehler beim Trennen der Verbindung zum Knoten %d: %s", serverId, err))
	}

	return

}

func main() {

	// Parmeter einlesen
	var (
		clientId     = flag.Int("client", 16, "Client-ID [1...254] -")
		serverIdList = flag.String("server_ids", "1", "Kommagetrennte Liste von Knoten-IDs der abzufragenden UVR: 1,2,3,... -")
		canInterface = flag.String("interface", "can0", "Name des CAN-Bus Netzwerkinterface -")
		outputFile   = flag.String("output", "daten.json", "Pfad für Ausgabedatei -")
		verbose      = flag.Bool("v", false, "Ausführliche Ausgaben erzeugen - default(false)")
		pidFileName  = flag.String("pidfile", "uvr2json.pid", "Pfad für PID-File zur Erkennung laufender Vorgänge -")
	)
	flag.Parse()

	// UVR-Knoten einlesen
	serverIds := strings.Split(*serverIdList, ",")

	// Logging konfigurieren
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Print("\n=================================================================================\n= UVR2JSON: Werkzeug zum Auslesen von UVR-Knoten und Exportieren als JSON-Datei =\n=================================================================================")
	log.Print("")
	log.Print("Vorgang gestartet. Bitte warten ...")
	if *verbose {
		log.Print("Folgende Parameter wurden eingelesen:")
		log.Printf(" - client:     %d", *clientId)
		log.Printf(" - server_ids: %s", serverIds)
		log.Printf(" - interface:  %s", *canInterface)
		log.Printf(" - output:     %s", *outputFile)
		log.Printf(" - pidfile:    %s", *pidFileName)
		log.Print(" - verbose:    ", *verbose)
	}

	// Vorgang abbrechen, falls bereits eine Instanz aktiv (bspw. via cron)
	pidFile, err := os.OpenFile(*pidFileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		if os.IsExist(err) {
			log.Print("Fehler: Es läuft bereits eine Instanz von uvr2json im Hintergrund.")
			os.Exit(1)
		}
	}
	defer pidFile.Close()

	// Knoten-Daten vorbereiten
	var serverDataList []serverStruct
	var errorMessages []string

	// UVR-Knoten nacheinander abfragen
	for _, serverId := range serverIds {

		// UVR-Knoten ID von Leerzeichen befreien und in int umwandeln
		serverIdInt, err := strconv.Atoi(strings.ReplaceAll(serverId, " ", ""))
		if err == nil {

			// CAN-Bus initialisieren
			if *verbose {
				log.Printf("CAN-Bus für Kommunikation mit Knoten %d wird initialisiert ...", serverIdInt)
			}
			bus, err := can.NewBusForInterfaceWithName(*canInterface)

			if err == nil {

				// CAN-Bus verbinden
				if *verbose {
					log.Printf("Verbindung mit CAN-Bus als Client %d aufbauen ...", clientId)
				}
				go bus.ConnectAndPublish()
				nodeID := uint8(*clientId)
				client := uvr.NewClient(nodeID, bus)

				// UVR-Knoten abfragen
				if *verbose {
					log.Print("UVR-Knoten abfragen ...")
				}
				serverData, serverDataErrors := getServerData(client, serverIdInt, *verbose)
				serverDataList = append(serverDataList, serverData)
				for _, serverDataError := range serverDataErrors {
					errorMessages = append(errorMessages, serverDataError.Error())
					if *verbose {
						log.Print(errorMessages)
					}
				}

				// CAN-Bus schließen
				if *verbose {
					log.Print("CAN-Bus wird geschlossen und 8 Sekunden warten (Heartbeat = 5 Sekunden) ...")
				}
				bus.Disconnect()
				time.Sleep(8 * time.Second)

			} else {
				errorMessages = append(errorMessages, fmt.Sprintf("Fehler bei NewBusForInterfaceWithName: %s", err))
				if *verbose {
					log.Print(errorMessages)
				}
			}

		} else {
			errorMessages = append(errorMessages, fmt.Sprintf("Format der angegebenen Knoten-ID [%s] fehlerhaft: %v", serverId, err))
			if *verbose {
				log.Print(errorMessages)
			}
		}

	}

	// Daten-Container anlegen
	dataContainer := DataStruct{
		Zeitstempel: time.Now(),
		Knoten:      serverDataList,
		Fehler:      errorMessages,
	}

	// Daten-Container in JSON umwandeln
	jsonData, err := json.MarshalIndent(dataContainer, "", "  ")
	if err == nil {

		if *verbose {
			log.Print("Ausgelesene Daten:\n" + string(jsonData))
		}

		// Ausgabedatei erzeugen
		file, err := os.Create(*outputFile)
		if err == nil {

			defer file.Close()

			// Daten-Container als JSON in Ausgabedatei schreiben
			_, err = file.Write(jsonData)
			if err == nil {

				log.Print("")
				if len(errorMessages) == 0 {
					log.Printf("Ergebnisse ohne Fehler in %s geschrieben.", *outputFile)
				} else {
					log.Printf("Ergebnisse mit %d Fehlern in %s geschrieben.", len(errorMessages), *outputFile)
				}
				log.Print("")

			} else {
				errorMessages = append(errorMessages, fmt.Sprintf("Fehler beim Schreiben der Datei: %s", err))
				if *verbose {
					log.Print(errorMessages)
				}
			}

		} else {
			errorMessages = append(errorMessages, fmt.Sprintf("Fehler beim Erstellen der Datei: %s", err))
			if *verbose {
				log.Print(errorMessages)
			}
		}

	} else {
		errorMessages = append(errorMessages, fmt.Sprintf("Fehler beim Umwandeln in JSON-Format: %s", err))
		if *verbose {
			log.Print(errorMessages)
		}
	}

	// Programm beenden
	err = os.Remove(*pidFileName)
	if err == nil {
		if len(errorMessages) == 0 {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	} else {
		log.Print("Fehler: PID-File zur Erkennung laufender Vorgänge konnte nicht gelöscht werden.")
		os.Exit(1)
	}
}
