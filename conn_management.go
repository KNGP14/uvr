package uvr

import (
	"fmt"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/canopen"
)

const (
	// Byte 7 of the response of the UVR (verified with candump of two UVR1611, 2026-10-03/04)
	connectAccepted    = 0x00
	disconnectAccepted = 0x80
)

func Disconnect(serverID uint8, clientID uint8, bus *can.Bus) error {
	b := []byte{
		0x80 + byte(serverID),
		0x01, 0x1F,
		0x00,
		byte(serverID),
		byte(clientID),
		0x80,
		0x12,
	}

	return sendConnManagementData(b, serverID, clientID, disconnectAccepted, bus)
}

func Connect(serverID uint8, clientID uint8, bus *can.Bus) error {
	b := []byte{
		0x80 + byte(serverID),
		0x00, 0x1F,
		0x00,
		byte(serverID),
		byte(clientID),
		0x80,
		0x12,
	}

	return sendConnManagementData(b, serverID, clientID, connectAccepted, bus)
}

// sendConnManagementData sends b and waits for the response of the server.
// Responses with an other byte 7 than accepted (e.g. a late response to an
// earlier request) are skipped. If no matching response arrives, the error
// contains the last skipped response.
func sendConnManagementData(b []byte, serverID uint8, clientID uint8, accepted byte, bus *can.Bus) error {
	frm := canopen.Frame{
		CobID: uint16(MPDOClientServerConnManagement) + uint16(clientID),
		Data:  b,
	}

	respID := uint32(MPDOClientServerConnManagement) + uint32(serverID)
	_, err := request(bus, frm, respID, time.Second*2, func(resp canopen.Frame) bool {
		return len(resp.Data) == 8 &&
			resp.Data[0] == 0x80+byte(clientID) &&
			resp.Data[4] == 0x40+byte(clientID) &&
			resp.Data[5] == 0x06 &&
			resp.Data[7] == accepted
	})

	if err != nil {
		return fmt.Errorf("Keine gültige Antwort von Knoten %d (%w)", serverID, err)
	}

	return nil
}
