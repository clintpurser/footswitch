package footswitch

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

var errNotFound = errors.New("pedal not found")

// rawEvent is one evdev input_event, without its timestamp.
type rawEvent struct {
	typ   uint16
	code  uint16
	value int32
}

// eventSource is the pedal's grabbed keyboard event device.
type eventSource interface {
	readEvent() (rawEvent, error)
	close() error
}

// openedDevice is a found, opened and grabbed pedal.
type openedDevice struct {
	info     inputDevice
	events   eventSource
	settings slotIO // nil when the settings interface can't be opened
}

func (d *openedDevice) close() {
	if d.settings != nil {
		_ = d.settings.close()
	}
	if d.events != nil {
		_ = d.events.close()
	}
}

// openPedal finds the pedal, grabs its keyboard and opens its settings
// interface through hidraw (never libusb, so no kernel driver is detached).
func openPedal(p sysPaths, devicePath, usbID string) (*openedDevice, error) {
	info, err := findDevice(p, devicePath, usbID)
	if err != nil {
		return nil, err
	}
	events, err := openEvdev(info.eventPath)
	if err != nil {
		return nil, err
	}
	dev := &openedDevice{info: info, events: events}
	if info.hidrawPath != "" {
		if h, err := openHidraw(info.hidrawPath); err == nil {
			dev.settings = newPCsensorDriver(h)
		}
	}
	return dev, nil
}

// evdev input_event is a timeval followed by type, code and value.
var inputEventSize = 2*strconv.IntSize/8 + 8

type evdevFile struct {
	f   *os.File
	buf []byte
}

func openEvdev(path string) (*evdevFile, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if err := grab(f, true); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("grabbing %s (is another program using the pedal?): %w", path, err)
	}
	return &evdevFile{f: f, buf: make([]byte, inputEventSize)}, nil
}

func (e *evdevFile) readEvent() (rawEvent, error) {
	if _, err := io.ReadFull(e.f, e.buf); err != nil {
		return rawEvent{}, err
	}
	b := e.buf[inputEventSize-8:]
	return rawEvent{
		typ:   binary.LittleEndian.Uint16(b[0:2]),
		code:  binary.LittleEndian.Uint16(b[2:4]),
		value: int32(binary.LittleEndian.Uint32(b[4:8])),
	}, nil
}

func (e *evdevFile) close() error {
	_ = grab(e.f, false)
	return e.f.Close()
}

type hidrawFile struct {
	f *os.File
}

func openHidraw(path string) (*hidrawFile, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &hidrawFile{f: f}, nil
}

func (h *hidrawFile) writeReport(b []byte) error {
	_, err := h.f.Write(b)
	return err
}

func (h *hidrawFile) readReport(timeout time.Duration) ([]byte, error) {
	if err := h.f.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	buf := make([]byte, 64)
	n, err := h.f.Read(buf)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, errReadTimeout
	}
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (h *hidrawFile) close() error { return h.f.Close() }
