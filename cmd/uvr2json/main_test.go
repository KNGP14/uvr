package main

import (
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
	go bus.ConnectAndPublish()
	t.Cleanup(func() { bus.Disconnect() })

	return uvr.NewClient(16, bus)
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
