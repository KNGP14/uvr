package uvr

import (
	"fmt"
	"sync"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/canopen"
)

// dispatcher delivers received frames to the pending requests of a bus.
// It replaces can.Wait, which never unsubscribes a waiter after a timeout or
// a failed send. Such a stale waiter blocks the receive goroutine of the bus.
type dispatcher struct {
	mu      sync.Mutex
	pending map[uint32]chan canopen.Frame
}

var (
	dispatchersMu sync.Mutex
	dispatchers   = map[*can.Bus]*dispatcher{}
)

// dispatcherFor returns the dispatcher of bus and subscribes it on first use.
// Call it before bus.ConnectAndPublish is started, because can.Bus.Subscribe is not synchronized.
func dispatcherFor(bus *can.Bus) *dispatcher {
	dispatchersMu.Lock()
	defer dispatchersMu.Unlock()

	d, ok := dispatchers[bus]
	if !ok {
		d = &dispatcher{pending: map[uint32]chan canopen.Frame{}}
		bus.SubscribeFunc(d.handle)
		dispatchers[bus] = d
	}

	return d
}

func (d *dispatcher) handle(frm can.Frame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if ch, ok := d.pending[frm.ID&can.MaskIDSff]; ok {
		// never block the receive goroutine of the bus
		select {
		case ch <- canopen.CANopenFrame(frm):
		default:
		}
	}
}

func (d *dispatcher) register(id uint32) (chan canopen.Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.pending[id]; ok {
		return nil, fmt.Errorf("Anfrage mit Antwort-ID %X läuft bereits", id)
	}

	ch := make(chan canopen.Frame, 8)
	d.pending[id] = ch
	return ch, nil
}

func (d *dispatcher) unregister(id uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.pending, id)
}

func (d *dispatcher) pendingCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// request publishes frm and waits for the first frame with respID which is accepted.
// Frames which are not accepted (e.g. late responses to an earlier request) are skipped.
// The request is unregistered in any case: response, timeout or failed send.
func request(bus *can.Bus, frm canopen.Frame, respID uint32, timeout time.Duration, accept func(canopen.Frame) bool) (canopen.Frame, error) {
	d := dispatcherFor(bus)
	ch, err := d.register(respID)
	if err != nil {
		return canopen.Frame{}, err
	}
	defer d.unregister(respID)

	if err := bus.Publish(frm.CANFrame()); err != nil {
		return canopen.Frame{}, err
	}

	var skipped *canopen.Frame
	deadline := time.After(timeout)
	for {
		select {
		case resp := <-ch:
			if accept(resp) {
				return resp, nil
			}
			skipped = &resp
		case <-deadline:
			if skipped != nil {
				return canopen.Frame{}, fmt.Errorf("Timeout beim Warten auf %X (unpassende Antwort % X)", respID, skipped.Data)
			}
			return canopen.Frame{}, fmt.Errorf("Timeout beim Warten auf %X", respID)
		}
	}
}
