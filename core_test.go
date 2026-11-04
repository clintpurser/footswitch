package footswitch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
)

type fakeEvents struct {
	ch     chan rawEvent
	gone   chan struct{}
	once   sync.Once
	closed chan struct{}
}

func newFakeEvents() *fakeEvents {
	return &fakeEvents{ch: make(chan rawEvent, 64), gone: make(chan struct{}), closed: make(chan struct{})}
}

func (f *fakeEvents) readEvent() (rawEvent, error) {
	select {
	case ev := <-f.ch:
		return ev, nil
	case <-f.gone:
		return rawEvent{}, errors.New("no such device")
	case <-f.closed:
		return rawEvent{}, errors.New("file already closed")
	}
}

func (f *fakeEvents) close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeEvents) unplug() { close(f.gone) }

func (f *fakeEvents) key(code uint16, value int32) {
	f.ch <- rawEvent{typ: evKey, code: code, value: value}
}

type fakeSlots struct {
	mu     sync.Mutex
	slots  [3]slotSetting
	writes int
}

func (f *fakeSlots) readSlots() ([3]slotSetting, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.slots, nil
}

func (f *fakeSlots) writeSlots(s [3]slotSetting) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slots = s
	f.writes++
	return nil
}

func (f *fakeSlots) close() error { return nil }

var (
	f13 = keysByName["KEY_F13"]
	f14 = keysByName["KEY_F14"]
	f15 = keysByName["KEY_F15"]
)

func defaultSpecs(t *testing.T, cfg *Config) []pedalSpec {
	t.Helper()
	specs, err := cfg.pedalSpecs("test")
	if err != nil {
		t.Fatal(err)
	}
	return specs
}

// harness wires a core to fake devices; each open hands out the next one.
type harness struct {
	t      *testing.T
	core   *controllerCore
	slots  *fakeSlots
	mu     sync.Mutex
	evs    []*fakeEvents
	avail  bool
	events chan input.Event
}

func (h *harness) open() (*openedDevice, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.avail {
		return nil, errNotFound
	}
	ev := newFakeEvents()
	h.evs = append(h.evs, ev)
	return &openedDevice{info: inputDevice{usbID: "3553:b001", eventPath: "/dev/input/event9"}, events: ev, settings: h.slots}, nil
}

func (h *harness) current() *fakeEvents {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.evs[len(h.evs)-1]
}

func newHarness(t *testing.T, cfg *Config, slots [3]slotSetting, debounce time.Duration) (*harness, error) {
	return newHarnessSpecs(t, "pcsensor-3-pedal", defaultSpecs(t, cfg), slots, debounce)
}

func onePedalSpecs(t *testing.T, cfg *OnePedalConfig) []pedalSpec {
	t.Helper()
	specs, err := cfg.pedalSpecs("test")
	if err != nil {
		t.Fatal(err)
	}
	return specs
}

func newOnePedalHarness(t *testing.T, cfg *OnePedalConfig, slots [3]slotSetting, debounce time.Duration) (*harness, error) {
	return newHarnessSpecs(t, "pcsensor-1-pedal", onePedalSpecs(t, cfg), slots, debounce)
}

func newHarnessSpecs(t *testing.T, model string, specs []pedalSpec, slots [3]slotSetting, debounce time.Duration) (*harness, error) {
	h := &harness{t: t, slots: &fakeSlots{slots: slots}, avail: true, events: make(chan input.Event, 64)}
	core, err := newControllerCore(coreOptions{
		model:        model,
		pedals:       specs,
		debounce:     debounce,
		open:         h.open,
		pollInterval: 10 * time.Millisecond,
	}, logging.NewTestLogger(t))
	if err != nil {
		return nil, err
	}
	h.core = core
	for _, c := range core.controls() {
		if err := core.registerCallback(c, []input.EventType{input.AllEvents}, func(_ context.Context, ev input.Event) {
			h.events <- ev
		}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(core.close)
	return h, nil
}

func (h *harness) next() input.Event {
	h.t.Helper()
	select {
	case ev := <-h.events:
		return ev
	case <-time.After(2 * time.Second):
		h.t.Fatal("timed out waiting for an event")
		return input.Event{}
	}
}

func (h *harness) none() {
	h.t.Helper()
	select {
	case ev := <-h.events:
		h.t.Fatalf("unexpected event %+v", ev)
	case <-time.After(60 * time.Millisecond):
	}
}

func (h *harness) expect(typ input.EventType, control input.Control) {
	h.t.Helper()
	ev := h.next()
	if ev.Event != typ || ev.Control != control {
		h.t.Fatalf("got %s %s, want %s %s", ev.Event, ev.Control, typ, control)
	}
}

var programmedSlots = [3]slotSetting{keySetting(f13.hid), keySetting(f14.hid), keySetting(f15.hid)}

func TestPressRelease(t *testing.T) {
	h, err := newHarness(t, &Config{}, programmedSlots, 0)
	if err != nil {
		t.Fatal(err)
	}
	ev := h.current()
	ev.key(f13.code, keyDown)
	h.expect(input.ButtonPress, input.ButtonLT)
	ev.key(f13.code, keyRepeat)
	ev.key(f15.code, keyDown)
	h.expect(input.ButtonPress, input.ButtonRT)
	ev.key(f13.code, keyUp)
	h.expect(input.ButtonRelease, input.ButtonLT)
	ev.key(30, keyDown) // KEY_A, not mapped
	h.none()

	evs := h.core.latestEvents()
	if evs[input.ButtonRT].Event != input.ButtonPress || evs[input.ButtonLT].Event != input.ButtonRelease {
		t.Fatalf("latest events wrong: %+v", evs)
	}
	list := h.core.listPedals()
	pedals := list["pedals"].([]interface{})
	right := pedals[2].(map[string]interface{})
	if right["pressed"] != true || right["keycode"] != "KEY_F15" || right["keycode_source"] != "device" {
		t.Fatalf("list_pedals right = %v", right)
	}
	if h.slots.writes != 0 {
		t.Fatalf("wrote %d times with nothing to program", h.slots.writes)
	}
}

func TestDebounceSettles(t *testing.T) {
	h, err := newHarness(t, &Config{}, programmedSlots, 40*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ev := h.current()
	ev.key(f14.code, keyDown)
	h.expect(input.ButtonPress, input.ButtonSelect)
	// Bounce: up and down again inside the window is filtered out.
	ev.key(f14.code, keyUp)
	ev.key(f14.code, keyDown)
	h.none()
	// A short tap's release inside the window is reported once it ends.
	ev.key(f14.code, keyUp)
	h.expect(input.ButtonRelease, input.ButtonSelect)
}

func TestProgramsConfiguredKeys(t *testing.T) {
	factory := [3]slotSetting{keySetting(0x04), keySetting(0x05), keySetting(0x06)} // a, b, c
	cfg := &Config{Left: &PedalConfig{Keycode: "KEY_F13"}, Right: &PedalConfig{Keycode: "KEY_F15", Control: "ButtonEStop"}}
	h, err := newHarness(t, cfg, factory, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := [3]slotSetting{keySetting(f13.hid), keySetting(0x05), keySetting(f15.hid)}
	if h.slots.writes != 1 || h.slots.slots != want {
		t.Fatalf("writes=%d slots=%v, want %v", h.slots.writes, h.slots.slots, want)
	}
	h.current().key(f15.code, keyDown)
	h.expect(input.ButtonPress, input.ButtonEStop)
	h.current().key(48, keyDown) // KEY_B from the center pedal
	h.expect(input.ButtonPress, input.ButtonSelect)
}

func TestSetupErrors(t *testing.T) {
	mouse := programmedSlots
	mouse[1] = slotSetting{0x08, slotTypeMouse}
	_, err := newHarness(t, &Config{}, mouse, 0)
	if err == nil || !strings.Contains(err.Error(), "pedal center (slot 2) is in mouse mode") {
		t.Fatalf("mouse mode: got %v", err)
	}
	// A keycode fixes it.
	if _, err := newHarness(t, &Config{Center: &PedalConfig{Keycode: "KEY_F14"}}, mouse, 0); err != nil {
		t.Fatalf("mouse mode with keycode: %v", err)
	}

	clash := programmedSlots
	_, err = newHarness(t, &Config{Left: &PedalConfig{Keycode: "KEY_F15"}}, clash, 0)
	if err == nil || !strings.Contains(err.Error(), "both send KEY_F15") {
		t.Fatalf("clash: got %v", err)
	}
}

func TestUnplugReplugAndClose(t *testing.T) {
	h, err := newHarness(t, &Config{}, programmedSlots, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := h.current()
	first.key(f13.code, keyDown)
	h.expect(input.ButtonPress, input.ButtonLT)

	h.mu.Lock()
	h.avail = false
	h.mu.Unlock()
	first.unplug()
	h.expect(input.ButtonRelease, input.ButtonLT)
	for i := 0; i < 3; i++ {
		if ev := h.next(); ev.Event != input.Disconnect {
			t.Fatalf("got %s, want Disconnect", ev.Event)
		}
	}
	if h.core.listPedals()["device"].(map[string]interface{})["connected"] != false {
		t.Fatal("expected disconnected")
	}

	h.mu.Lock()
	h.avail = true
	h.mu.Unlock()
	for i := 0; i < 3; i++ {
		if ev := h.next(); ev.Event != input.Connect {
			t.Fatalf("got %s, want Connect", ev.Event)
		}
	}
	h.current().key(f15.code, keyDown)
	h.expect(input.ButtonPress, input.ButtonRT)

	h.core.close()
	h.expect(input.ButtonRelease, input.ButtonRT)
}

// factorySlots is what a PCsensor unit ships with: a, b, c in slots 1-3.
// A single-pedal unit has the same three stored, with only slot 2 wired up.
var factorySlots = [3]slotSetting{keySetting(0x04), keySetting(0x05), keySetting(0x06)}

const (
	keyA = 30
	keyB = 48
	keyC = 46
)

func TestOnePedalReadsSlot2(t *testing.T) {
	h, err := newOnePedalHarness(t, &OnePedalConfig{}, factorySlots, 0)
	if err != nil {
		t.Fatal(err)
	}
	if controls := h.core.controls(); len(controls) != 1 || controls[0] != input.ButtonSouth {
		t.Fatalf("controls = %v", controls)
	}
	// Connect fires before the harness registers its callbacks, so check it
	// through the stored events instead.
	if evs := h.core.latestEvents(); len(evs) != 1 || evs[input.ButtonSouth].Event != input.Connect {
		t.Fatalf("latest events after start = %+v", evs)
	}
	ev := h.current()
	ev.key(keyB, keyDown)
	h.expect(input.ButtonPress, input.ButtonSouth)
	ev.key(keyB, keyUp)
	h.expect(input.ButtonRelease, input.ButtonSouth)
	ev.key(keyA, keyDown) // slot 1 has no switch; nothing is mapped to it
	h.none()

	list := h.core.listPedals()
	if list["model"] != "pcsensor-1-pedal" {
		t.Fatalf("model = %v", list["model"])
	}
	pedals := list["pedals"].([]interface{})
	if len(pedals) != 1 {
		t.Fatalf("pedals = %v", pedals)
	}
	p := pedals[0].(map[string]interface{})
	if p["name"] != "pedal" || p["slot"] != 2 || p["control"] != "ButtonSouth" || p["keycode"] != "KEY_B" || p["keycode_source"] != "device" {
		t.Fatalf("list_pedals pedal = %v", p)
	}
	if h.slots.writes != 0 {
		t.Fatalf("wrote %d times with nothing to program", h.slots.writes)
	}
}

func TestOnePedalProgramsOnlyItsSlot(t *testing.T) {
	h, err := newOnePedalHarness(t, &OnePedalConfig{Pedal: &PedalConfig{Keycode: "KEY_F14"}}, factorySlots, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := [3]slotSetting{keySetting(0x04), keySetting(f14.hid), keySetting(0x06)}
	if h.slots.writes != 1 || h.slots.slots != want {
		t.Fatalf("writes=%d slots=%v, want %v", h.slots.writes, h.slots.slots, want)
	}
	ev := h.current()
	ev.key(f14.code, keyDown)
	h.expect(input.ButtonPress, input.ButtonSouth)
	ev.key(keyB, keyDown)
	h.none()
}

func TestOnePedalSlotOverride(t *testing.T) {
	three := 3
	h, err := newOnePedalHarness(t, &OnePedalConfig{Slot: &three}, factorySlots, 0)
	if err != nil {
		t.Fatal(err)
	}
	ev := h.current()
	ev.key(keyC, keyDown)
	h.expect(input.ButtonPress, input.ButtonSouth)
	ev.key(keyB, keyDown)
	h.none()
	p := h.core.listPedals()["pedals"].([]interface{})[0].(map[string]interface{})
	if p["slot"] != 3 || p["keycode"] != "KEY_C" {
		t.Fatalf("list_pedals pedal = %v", p)
	}
}

func TestOnePedalSetupErrors(t *testing.T) {
	mouse := factorySlots
	mouse[1] = slotSetting{0x08, slotTypeMouse}
	_, err := newOnePedalHarness(t, &OnePedalConfig{}, mouse, 0)
	if err == nil || !strings.Contains(err.Error(), "pedal (slot 2) is in mouse mode") {
		t.Fatalf("mouse mode: got %v", err)
	}
	if _, err := newOnePedalHarness(t, &OnePedalConfig{Pedal: &PedalConfig{Keycode: "KEY_F14"}}, mouse, 0); err != nil {
		t.Fatalf("mouse mode with keycode: %v", err)
	}

	// Unused slots never block startup.
	unusedMouse := factorySlots
	unusedMouse[0] = slotSetting{0x08, slotTypeMouse}
	if _, err := newOnePedalHarness(t, &OnePedalConfig{}, unusedMouse, 0); err != nil {
		t.Fatalf("slot 1 in mouse mode: %v", err)
	}
}

func TestOnePedalUnplugWhileHeld(t *testing.T) {
	h, err := newOnePedalHarness(t, &OnePedalConfig{}, factorySlots, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := h.current()
	first.key(keyB, keyDown)
	h.expect(input.ButtonPress, input.ButtonSouth)
	h.mu.Lock()
	h.avail = false
	h.mu.Unlock()
	first.unplug()
	h.expect(input.ButtonRelease, input.ButtonSouth)
	h.expect(input.Disconnect, input.ButtonSouth)
}
