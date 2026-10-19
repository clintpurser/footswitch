package footswitch

import (
	"bytes"
	"testing"
	"time"
)

// fakeHID answers PCsensor read queries from stored slot reports.
type fakeHID struct {
	slots   map[byte][][]byte
	pending [][]byte
	writes  [][]byte
}

func (f *fakeHID) writeReport(b []byte) error {
	f.writes = append(f.writes, append([]byte(nil), b...))
	if b[0] == 0x01 && b[1] == 0x82 {
		f.pending = append(f.pending, f.slots[b[3]]...)
	}
	return nil
}

func (f *fakeHID) readReport(time.Duration) ([]byte, error) {
	if len(f.pending) == 0 {
		return nil, errReadTimeout
	}
	r := f.pending[0]
	f.pending = f.pending[1:]
	return r, nil
}

func (f *fakeHID) close() error { return nil }

func TestPCsensorReadSlots(t *testing.T) {
	hid := &fakeHID{slots: map[byte][][]byte{
		1: {{0x08, 0x01, 0x00, 0x68, 0, 0, 0, 0}},
		// A macro spanning three reports; the extra two must be skipped.
		2: {{0x14, 0x04, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}, {1, 2, 3, 4, 5, 6, 7, 8}, {1, 2, 3, 4, 0, 0, 0, 0}},
		3: {{0x08, 0x02, 0, 0, 0, 0, 0, 0}},
	}}
	d := &pcsensorDriver{io: hid, sleep: func(time.Duration) {}}
	s, err := d.readSlots()
	if err != nil {
		t.Fatal(err)
	}
	if s[0].mode() != "key" || s[0].hidKey() != 0x68 || s[1].mode() != "macro" || s[2].mode() != "mouse" {
		t.Fatalf("slots = %v", s)
	}
	if !bytes.Equal(hid.writes[1], []byte{0x01, 0x82, 0x08, 0x02, 0, 0, 0, 0}) {
		t.Fatalf("query 2 = % x", hid.writes[1])
	}
}

func TestPCsensorWriteSlots(t *testing.T) {
	hid := &fakeHID{}
	var slept time.Duration
	d := &pcsensorDriver{io: hid, sleep: func(d time.Duration) { slept += d }}
	if err := d.writeSlots([3]slotSetting{keySetting(0x68), keySetting(0x69), {}}); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{
		{0x01, 0x80, 0x08, 0x00, 0, 0, 0, 0},
		{0x01, 0x81, 0x08, 0x01, 0, 0, 0, 0}, {0x08, 0x01, 0x00, 0x68, 0, 0, 0, 0},
		{0x01, 0x81, 0x08, 0x02, 0, 0, 0, 0}, {0x08, 0x01, 0x00, 0x69, 0, 0, 0, 0},
		{0x01, 0x81, 0x08, 0x03, 0, 0, 0, 0}, {0x08, 0x00, 0x00, 0x00, 0, 0, 0, 0},
	}
	if len(hid.writes) != len(want) {
		t.Fatalf("got %d writes, want %d", len(hid.writes), len(want))
	}
	for i := range want {
		if !bytes.Equal(hid.writes[i], want[i]) {
			t.Errorf("write %d = % x, want % x", i, hid.writes[i], want[i])
		}
	}
	if slept < time.Second {
		t.Errorf("slept %s; the start report needs a 1s pause", slept)
	}
}

func TestKeyTable(t *testing.T) {
	codes, hids := map[uint16]bool{}, map[byte]bool{}
	for _, k := range keyTable {
		if codes[k.code] || hids[k.hid] {
			t.Errorf("duplicate entry %v", k)
		}
		codes[k.code], hids[k.hid] = true, true
	}
	// From the hardware findings: MSC_SCAN 0x70068-0x7006a for F13-F15.
	if keysByName["KEY_F13"].hid != 0x68 || keysByName["KEY_F15"].hid != 0x6a || keysByName["KEY_F13"].code != 183 {
		t.Fatal("F13-F15 mapping wrong")
	}
}
