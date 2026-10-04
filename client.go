package uvr

import (
	"errors"
	"syscall"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/canopen"
)

var (
	// ReadAttempts is the number of attempts of Read if the CAN transmit queue is full.
	ReadAttempts = 3
	// ReadRetryDelay is the pause between two attempts of Read.
	ReadRetryDelay = 200 * time.Millisecond
)

type Client struct {
	id        uint8
	bus       *can.Bus
	heartbeat chan<- struct{}
}

// NewClient returns a client with node id on bus.
// Call it before bus.ConnectAndPublish is started.
func NewClient(id uint8, bus *can.Bus) *Client {
	dispatcherFor(bus)
	return &Client{id, bus, nil}
}

func (c *Client) Connect(id uint8) error {
	c.heartbeat = canopen.ProduceHeartbeat(c.id, canopen.Operational, c.bus, time.Second*5)
	return Connect(id, c.id, c.bus)
}

func (c *Client) Disconnect(id uint8) error {
	c.StopHeartbeat()
	return Disconnect(id, c.id, c.bus)
}

// StopHeartbeat stops sending heartbeat frames.
// Closing the channel never blocks, even if the heartbeat goroutine
// already returned because publishing a frame failed.
func (c *Client) StopHeartbeat() {
	if c.heartbeat != nil {
		close(c.heartbeat)
		c.heartbeat = nil
	}
}

// Read reads the value at index i. If the CAN transmit queue is full,
// the read is repeated up to ReadAttempts times.
func (c *Client) Read(i canopen.ObjectIndex) (v interface{}, err error) {
	for attempt := 0; attempt < ReadAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(ReadRetryDelay)
		}

		v, err = ReadFromIndex(i, c.id, c.bus)
		if !IsBufferFull(err) {
			return
		}
	}

	return
}

// IsBufferFull returns true if err was caused by a full CAN transmit queue (ENOBUFS).
func IsBufferFull(err error) bool {
	return errors.Is(err, syscall.ENOBUFS)
}
