package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/canopen"
	"github.com/brutella/uvr"
)

// fullQueueRWC simulates a CAN socket whose transmit queue is full:
// every write fails with ENOBUFS, nothing is received.
type fullQueueRWC struct {
	closed chan struct{}
	once   sync.Once
}

func (rwc *fullQueueRWC) Read(b []byte) (int, error) {
	<-rwc.closed
	return 0, io.EOF
}

func (rwc *fullQueueRWC) Write(b []byte) (int, error) {
	return 0, &os.PathError{Op: "write", Path: "fd 6", Err: syscall.ENOBUFS}
}

func (rwc *fullQueueRWC) Close() error {
	rwc.once.Do(func() { close(rwc.closed) })
	return nil
}

func newFullQueueClient(t *testing.T) *uvr.Client {
	delay := uvr.ReadRetryDelay
	uvr.ReadRetryDelay = time.Millisecond
	t.Cleanup(func() { uvr.ReadRetryDelay = delay })

	bus := can.NewBus(can.NewReadWriteCloser(&fullQueueRWC{closed: make(chan struct{})}))
	client := uvr.NewClient(16, bus)
	go bus.ConnectAndPublish()
	t.Cleanup(func() { bus.Disconnect() })

	return client
}

func messages(errs []error) string {
	var s []string
	for _, err := range errs {
		s = append(s, err.Error())
	}
	return strings.Join(s, "\n")
}

func TestReadInletsAbortsIfBufferFull(t *testing.T) {
	_, errs := readInlets(newFullQueueClient(t), 1, false)

	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got:\n%s", messages(errs))
	}
	if !strings.HasPrefix(errs[0].Error(), "Fehler bei Eingang 1: ") {
		t.Errorf("wrong label: %s", errs[0])
	}
	if errs[1] != errBufferFull {
		t.Errorf("expected errBufferFull, got %s", errs[1])
	}
}

func TestReadOutletsAbortsIfBufferFull(t *testing.T) {
	_, errs := readOutlets(newFullQueueClient(t), 1, false)

	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got:\n%s", messages(errs))
	}
	if !strings.HasPrefix(errs[0].Error(), "Fehler bei Ausgang 1: ") {
		t.Errorf("wrong label: %s", errs[0])
	}
	if errs[1] != errBufferFull {
		t.Errorf("expected errBufferFull, got %s", errs[1])
	}
}

func TestGetServerDataIfBufferFull(t *testing.T) {
	start := time.Now()
	data, errs := getServerData(newFullQueueClient(t), 1, false)

	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v, expected no 5 s disconnect timeout", d)
	}
	if data.KnotenID != 1 || data.Eingaenge != nil || data.Ausgaenge != nil {
		t.Errorf("unexpected data %+v", data)
	}

	msg := messages(errs)
	if !strings.Contains(msg, "Fehler beim Verbinden mit Knoten 1") || !strings.Contains(msg, errBufferFull.Error()) {
		t.Errorf("unexpected errors:\n%s", msg)
	}
	if strings.Contains(msg, "Timeout") {
		t.Errorf("unexpected timeout:\n%s", msg)
	}
}

func TestAcquirePIDFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "uvr2json.pid")

	file, err := acquirePIDFile(name)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	data, _ := os.ReadFile(name)
	if strings.TrimSpace(string(data)) != strconv.Itoa(os.Getpid()) {
		t.Errorf("PID file contains %q", data)
	}

	// this process is still running
	if _, err := acquirePIDFile(name); !os.IsExist(err) {
		t.Errorf("expected IsExist, got %v", err)
	}
}

func TestAcquirePIDFileReplacesStaleFile(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-time.Hour)

	tests := []struct {
		name    string
		content string
		mtime   time.Time
		stale   bool
	}{
		{"dead process", "2147483646\n", time.Now(), true},
		{"empty and old", "", old, true},
		{"empty and new", "", time.Now(), false},
		{"running process", strconv.Itoa(os.Getpid()), old, false},
	}

	for i, tt := range tests {
		name := filepath.Join(dir, strconv.Itoa(i)+".pid")
		os.WriteFile(name, []byte(tt.content), 0666)
		os.Chtimes(name, tt.mtime, tt.mtime)

		file, err := acquirePIDFile(name)
		if tt.stale && err != nil {
			t.Errorf("%s: expected replacement, got %v", tt.name, err)
		}
		if !tt.stale && !os.IsExist(err) {
			t.Errorf("%s: expected IsExist, got %v", tt.name, err)
		}
		if file != nil {
			file.Close()
		}
	}
}

// fakeUVR simulates an UVR1611 with node id 1 for client 16. Responses are
// shaped like those recorded with candump (2026-10-03/04). All written frames are recorded.
type fakeUVR struct {
	mu      sync.Mutex
	written []can.Frame
	segment []byte // remaining data of the current segmented upload

	rx     chan []byte
	closed chan struct{}
	once   sync.Once
}

func (u *fakeUVR) Read(b []byte) (int, error) {
	select {
	case data := <-u.rx:
		return copy(b, data), nil
	case <-u.closed:
		return 0, io.EOF
	}
}

func (u *fakeUVR) Close() error {
	u.once.Do(func() { close(u.closed) })
	return nil
}

func (u *fakeUVR) Write(b []byte) (int, error) {
	var frm can.Frame
	if err := can.Unmarshal(b, &frm); err != nil {
		return 0, err
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	u.written = append(u.written, frm)

	var resp []byte
	var id uint32
	switch {
	case frm.ID == 0x410: // connect (byte 1 = 0x00) and disconnect (byte 1 = 0x01)
		id = 0x401
		b7 := byte(0x00)
		if frm.Data[1] == 0x01 {
			b7 = 0x80
		}
		resp = []byte{0x90, 0x80, 0x12, 0x01, 0x50, 0x06, 0x00, b7}

	case frm.ID == 0x650 && frm.Data[0]&0xE0 == 0x40: // initiate upload
		id = 0x5D0
		u.segment = u.object(binary.LittleEndian.Uint16(frm.Data[1:3]), frm.Data[3])
		resp = []byte{0x41, frm.Data[1], frm.Data[2], frm.Data[3], byte(len(u.segment)), 0, 0, 0}

	case frm.ID == 0x650 && frm.Data[0]&0xE0 == 0x60: // upload segment
		id = 0x5D0
		n := len(u.segment)
		if n > 7 {
			n = 7
		}
		cmd := frm.Data[0] & 0x10
		if n == len(u.segment) {
			cmd |= byte(7-n)<<1 | 0x01 // last segment
		}
		resp = make([]byte, 8)
		resp[0] = cmd
		copy(resp[1:], u.segment[:n])
		u.segment = u.segment[n:]

	default: // heartbeat 0x710
		return len(b), nil
	}

	data, _ := can.Marshal(canopen.Frame{CobID: uint16(id), Data: resp}.CANFrame())
	u.rx <- data
	return len(b), nil
}

// object returns the 7 data bytes of an object, or the text of a string object.
func (u *fakeUVR) object(index uint16, sub uint8) []byte {
	switch index {
	case 0x2084, 0x208e, 0x20a5, 0x20a1, 0x20aa: // descriptions, states, modes: reference to string object 0x52BC
		return []byte{sub, 0, 0, 0, 0xBC, 0x52, 0x10}
	case 0x52BC:
		return []byte("AUTO")
	default: // values: 16-bit integer 215 with one decimal place
		return []byte{0xD7, 0x00, 0, 0, 0x01, 0, 0x40}
	}
}

func TestGetServerDataWithFakeUVR(t *testing.T) {
	uvrRWC := &fakeUVR{rx: make(chan []byte, 16), closed: make(chan struct{})}
	bus := can.NewBus(can.NewReadWriteCloser(uvrRWC))
	client := uvr.NewClient(16, bus)
	go bus.ConnectAndPublish()
	defer bus.Disconnect()

	data, errs := getServerData(client, 1, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors:\n%s", messages(errs))
	}
	if len(data.Eingaenge) != 16 || len(data.Ausgaenge) != 13 {
		t.Fatalf("got %d inlets, %d outlets", len(data.Eingaenge), len(data.Ausgaenge))
	}
	if in := data.Eingaenge[0]; in.Bezeichnung != "AUTO" || in.Wert != float32(21.5) {
		t.Errorf("unexpected inlet %+v", in)
	}

	// only heartbeat, connection management and upload requests are sent
	uvrRWC.mu.Lock()
	defer uvrRWC.mu.Unlock()
	for _, frm := range uvrRWC.written {
		switch frm.ID {
		case 0x710, 0x410:
		case 0x650:
			if cmd := frm.Data[0]; cmd != 0x40 && cmd != 0x60 && cmd != 0x70 {
				t.Errorf("unexpected SDO command %02X", cmd)
			}
		default:
			t.Errorf("unexpected COB-ID %X", frm.ID)
		}
	}
}

func TestWriteFileAtomic(t *testing.T) {
	name := filepath.Join(t.TempDir(), "data.json")
	os.WriteFile(name, []byte("alt"), 0646)

	want, _ := json.Marshal(DataStruct{Fehler: []string{"x"}})
	if err := writeFileAtomic(name, want, 0644); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(name)
	if string(got) != string(want) {
		t.Errorf("content = %s", got)
	}
	if info, _ := os.Stat(name); info.Mode().Perm() != 0644 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
	if _, err := os.Stat(name + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary file left: %v", err)
	}
}
