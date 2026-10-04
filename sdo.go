package uvr

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/brutella/can"
	"github.com/brutella/canopen"
)

const (
	sdoClientInitiateUpload = 0x40
	sdoClientSegmentUpload  = 0x60
	sdoServerInitiateUpload = 0x40
	sdoServerSegmentUpload  = 0x00
	sdoAbort                = 0x80

	sdoMaskCommand   = 0xE0
	sdoToggle        = 0x10
	sdoExpedited     = 0x02
	sdoSizeIndicated = 0x01
	sdoLastSegment   = 0x01

	// maxSegments limits the segments of one upload (7 bytes each).
	maxSegments = 64
)

// SDOTimeout is the time to wait for a SDO response.
var SDOTimeout = 2 * time.Second

// upload reads the object at idx from the UVR via SDO (expedited or segmented upload).
// It only sends upload requests, never a download (write).
func upload(idx canopen.ObjectIndex, nodeID uint8, bus *can.Bus) ([]byte, error) {
	reqID := uint16(SSDOClientToServer2) + uint16(nodeID)
	respID := uint32(SSDOServerToClient2) + uint32(nodeID)

	// Initiate
	frm := canopen.Frame{
		CobID: reqID,
		Data: []byte{
			sdoClientInitiateUpload,
			idx.Index.B0, idx.Index.B1,
			idx.SubIndex,
			0x0, 0x0, 0x0, 0x0,
		},
	}

	sameIndex := func(resp canopen.Frame) bool {
		return len(resp.Data) == 8 && resp.Data[1] == idx.Index.B0 && resp.Data[2] == idx.Index.B1 && resp.Data[3] == idx.SubIndex
	}

	resp, err := request(bus, frm, respID, SDOTimeout, func(resp canopen.Frame) bool {
		return sameIndex(resp) && (resp.Data[0]&sdoMaskCommand == sdoServerInitiateUpload || resp.Data[0] == sdoAbort)
	})
	if err != nil {
		return nil, err
	}

	if resp.Data[0] == sdoAbort {
		return nil, abortError(idx, resp)
	}

	if resp.Data[0]&sdoExpedited == sdoExpedited {
		// number of bytes without data
		n := 0
		if resp.Data[0]&sdoSizeIndicated == sdoSizeIndicated {
			n = int(resp.Data[0] >> 2 & 0x3)
		}
		return resp.Data[4 : 8-n], nil
	}

	// Segments
	var b []byte
	var toggle byte
	for i := 0; i < maxSegments; i++ {
		frm = canopen.Frame{
			CobID: reqID,
			Data:  []byte{sdoClientSegmentUpload | toggle, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		}

		t := toggle
		resp, err = request(bus, frm, respID, SDOTimeout, func(resp canopen.Frame) bool {
			if len(resp.Data) != 8 {
				return false
			}
			if resp.Data[0] == sdoAbort {
				return sameIndex(resp)
			}
			return resp.Data[0]&sdoMaskCommand == sdoServerSegmentUpload && resp.Data[0]&sdoToggle == t
		})
		if err != nil {
			return nil, err
		}

		if resp.Data[0] == sdoAbort {
			return nil, abortError(idx, resp)
		}

		// number of bytes without data
		n := int(resp.Data[0] >> 1 & 0x7)
		b = append(b, resp.Data[1:8-n]...)

		if resp.Data[0]&sdoLastSegment == sdoLastSegment {
			return b, nil
		}

		toggle ^= sdoToggle
	}

	return nil, fmt.Errorf("Zu viele Segmente beim Lesen von %04X/%d", uint16(idx.Index.B1)<<8|uint16(idx.Index.B0), idx.SubIndex)
}

func abortError(idx canopen.ObjectIndex, resp canopen.Frame) error {
	code := binary.LittleEndian.Uint32(resp.Data[4:8])
	return fmt.Errorf("Lesen von %04X/%d von der UVR abgebrochen (Code %08X)", uint16(idx.Index.B1)<<8|uint16(idx.Index.B0), idx.SubIndex, code)
}
