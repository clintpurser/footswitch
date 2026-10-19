package footswitch

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// pcsensorUSBIDs are the USB IDs of PCsensor-protocol pedals, as listed by
// https://github.com/rgerganov/footswitch.
var pcsensorUSBIDs = []string{"1a86:e026", "3553:b001", "0c45:7403", "0c45:7404", "413d:2107"}

func isPCsensorUSBID(id string) bool {
	id = strings.ToLower(id)
	for _, s := range pcsensorUSBIDs {
		if s == id {
			return true
		}
	}
	return false
}

const pcsensorSlots = 3

// Slot types, from the second byte of a slot's settings.
const (
	slotTypeUnconfigured = 0x00
	slotTypeKey          = 0x01
	slotTypeMouse        = 0x02
	slotTypeKeyMouse     = 0x03
	slotTypeString       = 0x04
)

// slotSetting is the first 8-byte report of a slot's settings:
// length, type, modifiers, HID key, then mouse fields.
type slotSetting [8]byte

func keySetting(hid byte) slotSetting {
	return slotSetting{0x08, slotTypeKey, 0x00, hid}
}

// mode names the slot's mode for list_pedals.
func (s slotSetting) mode() string {
	switch s[1] & 0x7f {
	case slotTypeUnconfigured:
		return "unconfigured"
	case slotTypeKey:
		return "key"
	case slotTypeMouse, slotTypeKeyMouse:
		return "mouse"
	case slotTypeString:
		return "macro"
	default:
		return fmt.Sprintf("unknown (0x%02x)", s[1])
	}
}

func (s slotSetting) modifiers() byte { return s[2] }
func (s slotSetting) hidKey() byte    { return s[3] }

// isPlainKey reports whether the slot sends exactly one key with no modifiers.
func (s slotSetting) isPlainKey(hid byte) bool {
	return s.mode() == "key" && s.modifiers() == 0 && s.hidKey() == hid
}

// normalized is the setting to write back for a slot the config leaves alone.
func (s slotSetting) normalized() slotSetting {
	if s.mode() == "key" {
		return slotSetting{0x08, slotTypeKey, s.modifiers(), s.hidKey()}
	}
	return s
}

// reportIO is an open settings HID interface (a hidraw node).
type reportIO interface {
	writeReport(b []byte) error
	readReport(timeout time.Duration) ([]byte, error)
	close() error
}

// slotIO is the driver interface the controller core uses to read and
// program each slot's stored key.
type slotIO interface {
	readSlots() ([pcsensorSlots]slotSetting, error)
	writeSlots(s [pcsensorSlots]slotSetting) error
	close() error
}

var errReadTimeout = errors.New("timed out waiting for the pedal's settings report")

// pcsensorDriver speaks the PCsensor settings protocol over interface 1.
type pcsensorDriver struct {
	io    reportIO
	sleep func(time.Duration)
}

func newPCsensorDriver(io reportIO) *pcsensorDriver {
	return &pcsensorDriver{io: io, sleep: time.Sleep}
}

func (d *pcsensorDriver) write(b []byte) error {
	if err := d.io.writeReport(b); err != nil {
		return err
	}
	// The footswitch tool waits 30 ms after every report; the firmware needs it.
	d.sleep(30 * time.Millisecond)
	return nil
}

// drain discards reports left over from an earlier exchange.
func (d *pcsensorDriver) drain() {
	for i := 0; i < 16; i++ {
		if _, err := d.io.readReport(10 * time.Millisecond); err != nil {
			return
		}
	}
}

func (d *pcsensorDriver) readSlots() ([pcsensorSlots]slotSetting, error) {
	var out [pcsensorSlots]slotSetting
	d.drain()
	for i := 0; i < pcsensorSlots; i++ {
		query := []byte{0x01, 0x82, 0x08, byte(i + 1), 0x00, 0x00, 0x00, 0x00}
		if err := d.write(query); err != nil {
			return out, fmt.Errorf("querying slot %d: %w", i+1, err)
		}
		resp, err := d.io.readReport(time.Second)
		if err != nil {
			return out, fmt.Errorf("reading slot %d: %w", i+1, err)
		}
		if len(resp) < 8 {
			return out, fmt.Errorf("reading slot %d: short report of %d bytes", i+1, len(resp))
		}
		copy(out[i][:], resp[:8])
		// A macro's text continues in further 8-byte reports; skip them.
		if out[i][1]&0x7f == slotTypeString {
			for extra := (int(out[i][0]) - 1) / 8; extra > 0; extra-- {
				if _, err := d.io.readReport(time.Second); err != nil {
					break
				}
			}
		}
	}
	return out, nil
}

// writeSlots programs all three slots. The protocol always rewrites every
// slot, so callers pass the current setting for slots they don't change.
func (d *pcsensorDriver) writeSlots(s [pcsensorSlots]slotSetting) error {
	if err := d.write([]byte{0x01, 0x80, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00}); err != nil {
		return fmt.Errorf("starting write: %w", err)
	}
	d.sleep(time.Second)
	for i := 0; i < pcsensorSlots; i++ {
		data := s[i]
		if data[0] == 0 {
			data[0] = 0x08
		}
		header := []byte{0x01, 0x81, 0x08, byte(i + 1), 0x00, 0x00, 0x00, 0x00}
		if err := d.write(header); err != nil {
			return fmt.Errorf("writing slot %d: %w", i+1, err)
		}
		if err := d.write(data[:]); err != nil {
			return fmt.Errorf("writing slot %d: %w", i+1, err)
		}
	}
	return nil
}

func (d *pcsensorDriver) close() error { return d.io.close() }
