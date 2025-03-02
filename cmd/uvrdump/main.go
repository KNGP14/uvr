package main

import (
	"flag"
	"github.com/brutella/can"
	"github.com/brutella/uvr"
	"log"
	"fmt"
)

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
		if float, ok := value.(float32); ok == true {
			val = float
		}
	}

	return
}

func readOutlets(client *uvr.Client, serverid int) {
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
		fmt.Printf("{\"KNOTEN\":\"%d\",\"AUSGANG\":\"%d\",\"BEZEICHNUNG\":\"%s\",\"MODUS\":\"%s\",\"WERT\":\"%s\"},", serverid, index+1, descr, mode, val)
	}

}

func readInlets(client *uvr.Client, serverid int) {
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
		fmt.Printf("{\"KNOTEN\":\"%d\",\"EINGANG\":\"%d\",\"BEZEICHNUNG\":\"%s\",\"MODUS\":\"%s\",\"WERT\":\"%f\"},", serverid, index+1, descr, state, val)
	}

}

func HandleCANopen(frame can.Frame) {
	fmt.Printf("%X % X\n", frame.ID, frame.Data)
}

func main() {
	var (
		clientId = flag.Int("client_id", 16, "id of the client; range from [1...254]")
		serveraId = flag.Int("server_a_id", 1, "id of the server to which the client connects to: range from [1...254]")
		serverbId = flag.Int("server_b_id", 2, "id of the server to which the client connects to: range from [1...254]")
		iface    = flag.String("if", "can0", "name of the can network interface")
	)

	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	bus, err := can.NewBusForInterfaceWithName(*iface)

	if err != nil {
		log.Fatal(err)
	}
	// bus.SubscribeFunc(HandleCANopen)
	go bus.ConnectAndPublish()

	nodeID := uint8(*clientId)
	uvrID := uint8(*serveraId)

	c := uvr.NewClient(nodeID, bus)
	c.Connect(uvrID)

	fmt.Printf("{\"values\": [")
	readInlets(c,*serveraId)
	readOutlets(c,*serveraId)

	c.Disconnect(uvrID)

	uvrIDB := uint8(*serverbId)
	cb := uvr.NewClient(nodeID, bus)
	cb.Connect(uvrIDB)
	readInlets(cb,*serverbId)
	readOutlets(cb,*serverbId)
	cb.Disconnect(uvrIDB)

	fmt.Printf("]}")

	bus.Disconnect()
}
