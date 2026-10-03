package uvr

import (
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/canopen"
)

// fakeRWC simulates a CAN socket without opening one.
// The first failWrites writes fail with ENOBUFS (-1: all writes fail).
// respond returns the frames which are received for a written frame.
type fakeRWC struct {
	mu         sync.Mutex
	failWrites int
	written    []can.Frame
	respond    func(can.Frame) []can.Frame

	rx     chan []byte
	closed chan struct{}
	once   sync.Once
}

func newFakeBus(failWrites int, respond func(can.Frame) []can.Frame) (*can.Bus, *fakeRWC) {
	rwc := &fakeRWC{
		failWrites: failWrites,
		respond:    respond,
		rx:         make(chan []byte, 16),
		closed:     make(chan struct{}),
	}
	bus := can.NewBus(can.NewReadWriteCloser(rwc))
	go bus.ConnectAndPublish()

	return bus, rwc
}

func (rwc *fakeRWC) Read(b []byte) (int, error) {
	select {
	case data := <-rwc.rx:
		return copy(b, data), nil
	case <-rwc.closed:
		return 0, io.EOF
	}
}

func (rwc *fakeRWC) Write(b []byte) (int, error) {
	var frm can.Frame
	if err := can.Unmarshal(b, &frm); err != nil {
		return 0, err
	}

	rwc.mu.Lock()
	rwc.written = append(rwc.written, frm)
	fail := rwc.failWrites != 0
	if rwc.failWrites > 0 {
		rwc.failWrites--
	}
	rwc.mu.Unlock()

	if fail {
		return 0, &os.PathError{Op: "write", Path: "fd 99", Err: syscall.ENOBUFS}
	}

	if rwc.respond != nil {
		for _, resp := range rwc.respond(frm) {
			data, _ := can.Marshal(resp)
			rwc.rx <- data
		}
	}

	return len(b), nil
}

func (rwc *fakeRWC) Close() error {
	rwc.once.Do(func() { close(rwc.closed) })
	return nil
}

func (rwc *fakeRWC) writes() int {
	rwc.mu.Lock()
	defer rwc.mu.Unlock()
	return len(rwc.written)
}

// sdoServer answers a segmented SDO upload with the 7 bytes of value.
func sdoServer(nodeID uint8, value [7]byte) func(can.Frame) []can.Frame {
	return func(frm can.Frame) []can.Frame {
		if frm.ID != uint32(SSDOClientToServer2)+uint32(nodeID) {
			return nil
		}

		resp := canopen.Frame{CobID: uint16(SSDOServerToClient2) + uint16(nodeID)}
		switch frm.Data[0] & 0xE0 {
		case 0x40: // initiate upload: segmented, size indicated, 7 bytes
			resp.Data = []byte{0x41, frm.Data[1], frm.Data[2], frm.Data[3], 7, 0, 0, 0}
		case 0x60: // upload segment: last segment with 7 bytes
			resp.Data = append([]byte{0x01 | frm.Data[0]&0x10}, value[:]...)
		default:
			return nil
		}

		return []can.Frame{resp.CANFrame()}
	}
}

func shortRetryDelay(t *testing.T) {
	delay := ReadRetryDelay
	ReadRetryDelay = time.Millisecond
	t.Cleanup(func() { ReadRetryDelay = delay })
}

func TestIsBufferFull(t *testing.T) {
	err := &os.PathError{Op: "write", Path: "fd 6", Err: syscall.ENOBUFS}
	if !IsBufferFull(err) {
		t.Fatal("ENOBUFS not detected")
	}
	if IsBufferFull(nil) || IsBufferFull(io.EOF) {
		t.Fatal("false positive")
	}
}

func TestReadRetriesIfBufferFull(t *testing.T) {
	shortRetryDelay(t)
	bus, rwc := newFakeBus(-1, nil)
	defer bus.Disconnect()

	client := NewClient(16, bus)
	_, err := client.Read(NewInlet(1).Value)

	if !IsBufferFull(err) {
		t.Fatalf("expected ENOBUFS, got %v", err)
	}
	if is, want := rwc.writes(), ReadAttempts; is != want {
		t.Fatalf("writes = %d, want %d", is, want)
	}
}

func TestReadSucceedsAfterTransientBufferFull(t *testing.T) {
	shortRetryDelay(t)
	// 16-bit integer 215 with one decimal place = 21.5
	bus, rwc := newFakeBus(1, sdoServer(16, [7]byte{0xD7, 0x00, 0, 0, 0x01, 0, 0x40}))
	defer bus.Disconnect()

	client := NewClient(16, bus)
	v, err := client.Read(NewInlet(1).Value)

	if err != nil {
		t.Fatal(err)
	}
	if v != float32(21.5) {
		t.Fatalf("value = %v, want 21.5", v)
	}
	// 1 failed + initiate + segment
	if is, want := rwc.writes(), 3; is != want {
		t.Fatalf("writes = %d, want %d", is, want)
	}
}

func TestReadSendsOnlyUploadRequests(t *testing.T) {
	bus, rwc := newFakeBus(0, sdoServer(16, [7]byte{0xD7, 0x00, 0, 0, 0x01, 0, 0x40}))
	defer bus.Disconnect()

	client := NewClient(16, bus)
	if _, err := client.Read(NewInlet(1).Value); err != nil {
		t.Fatal(err)
	}

	rwc.mu.Lock()
	defer rwc.mu.Unlock()
	for _, frm := range rwc.written {
		if frm.ID != 0x650 {
			t.Errorf("unexpected COB-ID %X", frm.ID)
		}
		if ccs := frm.Data[0] & 0xE0; ccs != 0x40 && ccs != 0x60 {
			t.Errorf("unexpected SDO command %X (no upload request)", frm.Data[0])
		}
	}
}

func TestDisconnectDoesNotBlockAfterHeartbeatFailed(t *testing.T) {
	bus, _ := newFakeBus(-1, nil)
	defer bus.Disconnect()

	client := NewClient(16, bus)
	if err := client.Connect(1); !IsBufferFull(err) {
		t.Fatalf("expected ENOBUFS, got %v", err)
	}

	// heartbeat goroutine returned after its failed write
	time.Sleep(50 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- client.Disconnect(1) }()

	select {
	case err := <-done:
		if !IsBufferFull(err) {
			t.Fatalf("expected ENOBUFS, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Disconnect blocks")
	}
}

func TestStopHeartbeatTwice(t *testing.T) {
	bus, _ := newFakeBus(-1, nil)
	defer bus.Disconnect()

	client := NewClient(16, bus)
	client.StopHeartbeat() // no heartbeat yet
	client.Connect(1)
	client.StopHeartbeat()
	client.StopHeartbeat()
}
