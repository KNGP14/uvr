package uvr

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/canopen"
)

func shortSDOTimeout(t *testing.T) {
	timeout := SDOTimeout
	SDOTimeout = 50 * time.Millisecond
	t.Cleanup(func() { SDOTimeout = timeout })
}

func sdoResponse(nodeID uint8, data ...byte) can.Frame {
	return canopen.Frame{CobID: uint16(SSDOServerToClient2) + uint16(nodeID), Data: data}.CANFrame()
}

func isInitiate(frm can.Frame) bool {
	return frm.ID == 0x650 && frm.Data[0]&0xE0 == 0x40
}

// int16Value returns the 7 bytes of a 16-bit integer with one decimal place.
func int16Value(v int16) [7]byte {
	return [7]byte{byte(v), byte(v >> 8), 0, 0, 0x01, 0, 0x40}
}

func TestReadAfterTimeout(t *testing.T) {
	shortSDOTimeout(t)
	bus, rwc := newFakeBus(0, nil)
	defer bus.Disconnect()

	// the first request is not answered, all further requests are
	server := sdoServer(16, int16Value(215))
	var mu sync.Mutex
	first := true
	rwc.respond = func(frm can.Frame) []can.Frame {
		mu.Lock()
		defer mu.Unlock()
		if first && isInitiate(frm) {
			first = false
			return nil
		}
		return server(frm)
	}

	client := NewClient(16, bus)
	if _, err := client.Read(NewInlet(1).Value); err == nil || !strings.Contains(err.Error(), "Timeout") {
		t.Fatalf("expected timeout, got %v", err)
	}

	v, err := client.Read(NewInlet(1).Value)
	if err != nil {
		t.Fatalf("read after timeout failed: %v", err)
	}
	if v != float32(21.5) {
		t.Fatalf("value = %v, want 21.5", v)
	}
	if n := dispatcherFor(bus).pendingCount(); n != 0 {
		t.Fatalf("%d requests still registered", n)
	}
}

func TestLateResponseIsSkipped(t *testing.T) {
	timeout := SDOTimeout
	SDOTimeout = 100 * time.Millisecond
	t.Cleanup(func() { SDOTimeout = timeout })
	bus, rwc := newFakeBus(0, nil)
	defer bus.Disconnect()

	values := map[uint8][7]byte{1: int16Value(111), 2: int16Value(222)}
	pending := map[uint8][7]byte{}
	var mu sync.Mutex
	var last uint8

	rwc.respond = func(frm can.Frame) []can.Frame {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case isInitiate(frm):
			sub := frm.Data[3]
			last = sub
			pending[sub] = values[sub]
			resp := sdoResponse(16, 0x41, frm.Data[1], frm.Data[2], sub, 7, 0, 0, 0)
			if sub == 1 {
				// response to the first request arrives after its timeout (100 ms),
				// while the second request is waiting (sent at ~100 ms, answered at ~180 ms)
				rwc.deliverAfter(150*time.Millisecond, resp)
			} else {
				rwc.deliverAfter(80*time.Millisecond, resp)
			}
		case frm.ID == 0x650 && frm.Data[0]&0xE0 == 0x60:
			v := pending[last]
			return []can.Frame{sdoResponse(16, append([]byte{0x01 | frm.Data[0]&0x10}, v[:]...)...)}
		}
		return nil
	}

	client := NewClient(16, bus)
	if _, err := client.Read(NewInlet(1).Value); err == nil {
		t.Fatal("expected timeout for first request")
	}

	v, err := client.Read(NewInlet(2).Value)
	if err != nil {
		t.Fatal(err)
	}
	if v != float32(22.2) {
		t.Fatalf("value = %v, want 22.2 (late response of first request was used)", v)
	}
}

func TestFailedSendUnregistersRequest(t *testing.T) {
	shortRetryDelay(t)
	bus, _ := newFakeBus(-1, nil)
	defer bus.Disconnect()

	client := NewClient(16, bus)
	if _, err := client.Read(NewInlet(1).Value); !IsBufferFull(err) {
		t.Fatalf("expected ENOBUFS, got %v", err)
	}
	if n := dispatcherFor(bus).pendingCount(); n != 0 {
		t.Fatalf("%d requests still registered", n)
	}
}

func TestServerAbortIsError(t *testing.T) {
	bus, rwc := newFakeBus(0, nil)
	defer bus.Disconnect()

	rwc.respond = func(frm can.Frame) []can.Frame {
		if isInitiate(frm) {
			// abort code 0x06020000: object does not exist
			return []can.Frame{sdoResponse(16, 0x80, frm.Data[1], frm.Data[2], frm.Data[3], 0x00, 0x00, 0x02, 0x06)}
		}
		return nil
	}

	_, err := NewClient(16, bus).Read(NewInlet(1).Value)
	if err == nil || !strings.Contains(err.Error(), "abgebrochen (Code 06020000)") {
		t.Fatalf("expected abort error, got %v", err)
	}
}

func TestUnexpectedResponseIsSkipped(t *testing.T) {
	shortSDOTimeout(t)
	bus, rwc := newFakeBus(0, nil)
	defer bus.Disconnect()

	rwc.respond = func(frm can.Frame) []can.Frame {
		if isInitiate(frm) {
			// download response instead of upload response (canopen/sdo would call log.Fatal)
			return []can.Frame{sdoResponse(16, 0x60, frm.Data[1], frm.Data[2], frm.Data[3], 0, 0, 0, 0)}
		}
		return nil
	}

	_, err := NewClient(16, bus).Read(NewInlet(1).Value)
	if err == nil || !strings.Contains(err.Error(), "unpassende Antwort") {
		t.Fatalf("expected timeout with unexpected response, got %v", err)
	}
}

// connResponse returns the response of the UVR to a connect or disconnect request (from candump).
func connResponse(serverID, clientID, b7 byte) can.Frame {
	return canopen.Frame{
		CobID: uint16(MPDOClientServerConnManagement) + uint16(serverID),
		Data:  []byte{0x80 + clientID, 0x80, 0x12, 0x01, 0x40 + clientID, 0x06, 0x00, b7},
	}.CANFrame()
}

func TestConnectAndDisconnect(t *testing.T) {
	tests := []struct {
		name              string
		connect, response byte
		ok                bool
	}{
		{"connect accepted", 0x00, 0x00, true},
		{"disconnect accepted", 0x01, 0x80, true},
		{"connect with 0x80", 0x00, 0x80, false},
		{"disconnect with 0x00", 0x01, 0x00, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus, rwc := newFakeBus(0, nil)
			defer bus.Disconnect()

			rwc.respond = func(frm can.Frame) []can.Frame {
				if frm.ID == 0x410 {
					return []can.Frame{connResponse(2, 16, tt.response)}
				}
				return nil
			}

			var err error
			if tt.connect == 0x00 {
				err = Connect(2, 16, bus)
			} else {
				err = Disconnect(2, 16, bus)
			}

			if tt.ok && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if !tt.ok && (err == nil || !strings.Contains(err.Error(), "unpassende Antwort")) {
				t.Fatalf("expected error with unexpected response, got %v", err)
			}
		})
	}
}

func TestConnectSkipsUnsolicitedDisconnectResponse(t *testing.T) {
	bus, rwc := newFakeBus(0, nil)
	defer bus.Disconnect()

	// UVR 2 sometimes sends a disconnect response (byte 7 = 0x80) on its own (candump 2026-10-04)
	rwc.respond = func(frm can.Frame) []can.Frame {
		if frm.ID == 0x410 {
			return []can.Frame{connResponse(2, 16, 0x80), connResponse(2, 16, 0x00)}
		}
		return nil
	}

	if err := Connect(2, 16, bus); err != nil {
		t.Fatal(err)
	}
}
