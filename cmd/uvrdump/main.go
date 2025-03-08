package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/uvr"
)

var silentMode bool = false

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
	Fehler      string         `json:"Fehler"`
	Knoten      []serverStruct `json:"Knoten"`
}

func readOutlet(outlet uvr.Outlet, client *uvr.Client) (descr string, mode string, val string) {
	if value, err := client.Read(outlet.Description); err == nil {
		descr = value.(string)
	}

	if value, err := client.Read(outlet.Mode); err == nil {
		mode = value.(string)
	}

	if value, err := client.Read(outlet.State); err == nil {
		val = value.(string)
	}

	return
}

func readInlet(inlet uvr.Inlet, client *uvr.Client) (descr string, state string, val float32) {
	if value, err := client.Read(inlet.Description); err == nil {
		descr = value.(string)
	}

	if value, err := client.Read(inlet.State); err == nil {
		state = value.(string)
	}

	if value, err := client.Read(inlet.Value); err == nil {
		if float, ok := value.(float32); ok {
			val = float
		}
	}

	return
}

func readOutlets(client *uvr.Client, serverid int) (outletData []outletStruct) {
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
		descr, mode, val := readOutlet(outlet, client)
		if !silentMode {
			log.Printf("KNOTEN: \"%d\", AUSGANG: \"%d\", BEZEICHNUNG: \"%s\", MODUS: \"%s\", WERT: \"%s\"", serverid, index+1, descr, mode, val)
		}

		outlet := outletStruct{
			AusgangID:   index + 1,
			Bezeichnung: descr,
			Modus:       mode,
			Wert:        val,
		}
		outletData = append(outletData, outlet)
	}

	return outletData

}

func readInlets(client *uvr.Client, serverid int) (inletData []inletStruct) {

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
		descr, state, val := readInlet(inlet, client)
		if !silentMode {
			log.Printf("KNOTEN: \"%d\", EINGANG: \"%d\", BEZEICHNUNG: \"%s\", MODUS: \"%s\", WERT: \"%f\"", serverid, index+1, descr, state, val)
		}

		inlet := inletStruct{
			EingangID:   index + 1,
			Bezeichnung: descr,
			Modus:       state,
			Wert:        val,
		}
		inletData = append(inletData, inlet)
	}

	return inletData

}

func HandleCANopen(frame can.Frame) {
	if !silentMode {
		log.Printf("%X % X\n", frame.ID, frame.Data)
	}
}

func getServerData(client *uvr.Client, serverId int) (serverData serverStruct) {

	// Verbindung zur UVR aufbauen
	uvrID := uint8(serverId)
	client.Connect(uvrID)

	// Eingänge abfragen
	inletData := readInlets(client, serverId)

	// Ausgänge abfragen
	outletData := readOutlets(client, serverId)

	// Knoten-Daten hinzufügen
	serverData = serverStruct{
		KnotenID:  serverId,
		Eingaenge: inletData,
		Ausgaenge: outletData,
	}

	// Verbindung zur UVR trennen
	client.Disconnect(uvrID)

	return serverData

}

func main() {

	// Parmeter einlesen
	var (
		clientId          = flag.Int("client", 16, "Client-ID [1...254] -")
		singleServerId    = flag.Int("server_id", 1, "(einzelne UVR abfragen) Knoten-ID der abzufragenden UVR [1...254] -")
		multipleServerIds = flag.String("server_ids", "", "(mehrere UVRs abfragen) Kommagetrennte Liste von Knoten-IDs der abzufragenden UVR: 1,2,3,...")
		canInterface      = flag.String("interface", "can0", "Name des CAN-Bus Netzwerkinterface -")
		outputFile        = flag.String("output", "daten.json", "Pfad für Ausgabedatei -")
		silent            = flag.Bool("silent", false, "Ausgaben unterdrücken - default(false)")
	)
	flag.Parse()

	// Logging konfigurieren
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	silentMode = *silent

	if !silentMode {
		log.Printf("clientId:          %d", *clientId)
		log.Printf("singleServerId:    %d", *singleServerId)
		log.Printf("multipleServerIds: %s", *multipleServerIds)
		log.Printf("canInterface:      %s", *canInterface)
		log.Print("slientMode:        ", silentMode)
	}

	// CAN-Busverbindung aufbauen
	bus, err := can.NewBusForInterfaceWithName(*canInterface)
	if err != nil {
		if !silentMode {
			log.Fatal(err)
		}
		os.Exit(1)
	}
	go bus.ConnectAndPublish()
	nodeID := uint8(*clientId)
	client := uvr.NewClient(nodeID, bus)

	// Knoten-Daten anlegen
	var serverDataList []serverStruct

	// Einzel-Knotenabfrage oder mehrere Knoten abfragen
	if len(*multipleServerIds) == 0 {

		// Einzelnen UVR-Knoten abfragen
		serverData := getServerData(client, *singleServerId)
		serverDataList = append(serverDataList, serverData)

	} else {

		// Mehrere UVR-Knoten abfragen
		serverIds := strings.Split(*multipleServerIds, ",")
		for index, serverId := range serverIds {
			serverIdInt, err := strconv.Atoi(strings.ReplaceAll(serverId, " ", ""))
			if err != nil {
				if !silentMode {
					log.Printf("Fehler beim Einlesen der Knoten-IDs: [%d] %v", index, err)
				}
				os.Exit(1)
			}
			serverData := getServerData(client, serverIdInt)
			serverDataList = append(serverDataList, serverData)
		}

	}

	// CAN-Bus schließen
	bus.Disconnect()

	// Daten-Container anlegen
	dataContainer := DataStruct{
		Zeitstempel: time.Now(),
		Knoten:      serverDataList,
		Fehler:      "",
	}

	// Daten-Container in JSON umwandeln
	jsonData, err := json.MarshalIndent(dataContainer, "", "  ")
	if err != nil {
		if !silentMode {
			log.Print("Fehler beim Marshal: ", err)
		}
		os.Exit(1)
	}
	if !silentMode {
		log.Print(string(jsonData))
	}

	// Ausgabedatei erzeugen
	file, err := os.Create(*outputFile)
	if err != nil {
		if !silentMode {
			log.Print("Fehler beim Erstellen der Datei:", err)
		}
		os.Exit(1)
	}
	defer file.Close()

	// Daten-Container als JSON in Ausgabedatei schreiben
	_, err = file.Write(jsonData)
	if err != nil {
		if !silentMode {
			log.Print("Fehler beim Schreiben der Datei:", err)
		}
		os.Exit(1)
	}
	if !silentMode {
		log.Print("JSON erfolgreich in daten.json gespeichert.")
		os.Exit(0)
	}
}
