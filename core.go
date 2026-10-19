package footswitch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
)

// pedalSpec is one pedal's resolved config.
type pedalSpec struct {
	name    string
	slot    int
	control input.Control
	keycode *keyDef // set if the config programs this pedal's key
}

// pedal is a pedal's spec plus its live state.
type pedal struct {
	pedalSpec
	key       keyDef
	source    string // "config" or "device"
	mode      string
	pressed   bool // state last reported
	raw       bool // state last seen from the device
	changedAt time.Time
	pressedAt time.Time
	settle    *time.Timer
}

func (p *pedal) describe() string {
	return fmt.Sprintf("slot %d, %s", p.slot, p.key.name)
}

// coreOptions configures a controller core.
type coreOptions struct {
	model        string
	pedals       []pedalSpec
	debounce     time.Duration
	open         func() (*openedDevice, error)
	pollInterval time.Duration
}

// controllerCore is the hardware-independent part of the input controller:
// key-to-pedal map, debounce, state, callbacks and hotplug.
type controllerCore struct {
	opts   coreOptions
	logger logging.Logger

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// dispatchMu serializes state changes with their callbacks, so callbacks
	// see events in order. It is taken before mu.
	dispatchMu sync.Mutex

	mu        sync.Mutex
	pedals    []*pedal
	keyMap    map[uint16]*pedal
	callbacks map[input.Control]map[input.EventType]input.ControlFunction
	events    map[input.Control]input.Event
	dev       *openedDevice
	info      inputDevice
	connected bool
	closed    bool
	warned    map[uint16]bool
}

func newControllerCore(opts coreOptions, logger logging.Logger) (*controllerCore, error) {
	if opts.pollInterval == 0 {
		opts.pollInterval = time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &controllerCore{
		opts:      opts,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		callbacks: map[input.Control]map[input.EventType]input.ControlFunction{},
		events:    map[input.Control]input.Event{},
		warned:    map[uint16]bool{},
	}
	for _, s := range opts.pedals {
		c.pedals = append(c.pedals, &pedal{pedalSpec: s})
	}

	dev, err := c.connect()
	if err != nil {
		cancel()
		return nil, err
	}
	c.attach(dev)
	go c.run(dev)
	return c, nil
}

// pedalSetup is what setup learned about one pedal.
type pedalSetup struct {
	key    keyDef
	source string
	mode   string
}

// connect opens the device and applies startup steps 2-6.
func (c *controllerCore) connect() (*openedDevice, error) {
	dev, err := c.opts.open()
	if err != nil {
		return nil, err
	}
	setups, err := c.setup(dev)
	if err != nil {
		dev.close()
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keyMap = map[uint16]*pedal{}
	for i, p := range c.pedals {
		p.key, p.source, p.mode = setups[i].key, setups[i].source, setups[i].mode
		p.pressed, p.raw = false, false
		c.keyMap[p.key.code] = p
	}
	return dev, nil
}

// setup reads each slot, programs configured keys and checks the result.
func (c *controllerCore) setup(dev *openedDevice) ([]pedalSetup, error) {
	var stored [pcsensorSlots]slotSetting
	canRead := false
	if dev.settings != nil {
		s, err := dev.settings.readSlots()
		if err != nil {
			c.logger.Warnf("can't read the pedal's settings (%v); using configured keycodes only", err)
		} else {
			stored, canRead = s, true
		}
	} else {
		c.logger.Warnf("the pedal at %s has no settings interface; using configured keycodes only", dev.info.stablePath())
	}

	setups := make([]pedalSetup, len(c.pedals))
	desired := stored
	needWrite := false
	for i, p := range c.pedals {
		cur := stored[p.slot-1]
		if p.keycode != nil {
			setups[i] = pedalSetup{key: *p.keycode, source: "config", mode: "key"}
			if !canRead {
				setups[i].mode = "unknown"
				continue
			}
			if !cur.isPlainKey(p.keycode.hid) {
				desired[p.slot-1] = keySetting(p.keycode.hid)
				needWrite = true
			}
			continue
		}
		if !canRead {
			return nil, fmt.Errorf("can't read which key pedal %s (slot %d) sends; set its keycode", p.name, p.slot)
		}
		switch cur.mode() {
		case "key":
		case "mouse", "macro":
			return nil, fmt.Errorf("pedal %s (slot %d) is in %s mode; set a keycode for it so the module can program a key",
				p.name, p.slot, cur.mode())
		default:
			return nil, fmt.Errorf("pedal %s (slot %d) has no key stored (%s); set a keycode for it", p.name, p.slot, cur.mode())
		}
		k, ok := keysByHID[cur.hidKey()]
		if !ok {
			return nil, fmt.Errorf("pedal %s (slot %d) sends HID key 0x%02x, which this module doesn't know; set a keycode for it",
				p.name, p.slot, cur.hidKey())
		}
		if cur.modifiers() != 0 {
			c.logger.Warnf("pedal %s (slot %d) also sends modifier keys (0x%02x); only %s is used", p.name, p.slot, cur.modifiers(), k.name)
		}
		setups[i] = pedalSetup{key: k, source: "device", mode: "key"}
	}

	seen := map[uint16]string{}
	for i, p := range c.pedals {
		if other, ok := seen[setups[i].key.code]; ok {
			return nil, fmt.Errorf("pedals %s and %s both send %s; give one of them a different keycode", other, p.name, setups[i].key.name)
		}
		seen[setups[i].key.code] = p.name
	}

	if needWrite {
		for i := range desired {
			desired[i] = desired[i].normalized()
		}
		if err := dev.settings.writeSlots(desired); err != nil {
			return nil, fmt.Errorf("programming the pedal's keys: %w", err)
		}
		for _, p := range c.pedals {
			if p.keycode != nil && !stored[p.slot-1].isPlainKey(p.keycode.hid) {
				c.logger.Infof("programmed pedal %s (slot %d) to send %s", p.name, p.slot, p.keycode.name)
			}
		}
		if after, err := dev.settings.readSlots(); err == nil {
			for _, p := range c.pedals {
				if p.keycode != nil && !after[p.slot-1].isPlainKey(p.keycode.hid) {
					c.logger.Warnf("pedal %s (slot %d) reads back as %s 0x%02x after programming %s",
						p.name, p.slot, after[p.slot-1].mode(), after[p.slot-1].hidKey(), p.keycode.name)
				}
			}
		}
	}
	return setups, nil
}

// attach makes dev the current device and emits Connect.
func (c *controllerCore) attach(dev *openedDevice) {
	c.dispatchMu.Lock()
	defer c.dispatchMu.Unlock()
	c.mu.Lock()
	c.dev, c.info, c.connected = dev, dev.info, true
	now := time.Now()
	var fire []firing
	for _, p := range c.pedals {
		p.changedAt = time.Time{}
		fire = append(fire, c.record(input.Event{Time: now, Event: input.Connect, Control: p.control})...)
	}
	c.mu.Unlock()
	c.fire(fire)
}

// run reads events and handles unplug and replug until Close.
func (c *controllerCore) run(dev *openedDevice) {
	defer close(c.done)
	for {
		err := c.readLoop(dev)
		if c.ctx.Err() != nil {
			return
		}
		dev.close()
		c.disconnected(err)

		var lastErr string
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.opts.pollInterval):
			}
			d, err := c.connect()
			if err == nil {
				dev = d
				break
			}
			if !errors.Is(err, errNotFound) && err.Error() != lastErr {
				c.logger.Errorf("pedal is back but can't be used: %v", err)
				lastErr = err.Error()
			}
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			dev.close()
			return
		}
		c.mu.Unlock()
		c.logger.Infof("pedal reconnected at %s", dev.info.stablePath())
		c.attach(dev)
	}
}

func (c *controllerCore) readLoop(dev *openedDevice) error {
	for {
		ev, err := dev.events.readEvent()
		if err != nil {
			return err
		}
		c.handleEvent(ev)
	}
}

const (
	keyUp     = 0
	keyDown   = 1
	keyRepeat = 2
)

func (c *controllerCore) handleEvent(ev rawEvent) {
	if ev.typ != evKey {
		return
	}
	if ev.value == keyRepeat {
		c.logger.Debugf("ignored autorepeat of %s", codeName(ev.code))
		return
	}
	if ev.value != keyUp && ev.value != keyDown {
		return
	}

	c.dispatchMu.Lock()
	defer c.dispatchMu.Unlock()
	c.mu.Lock()
	if c.closed || !c.connected {
		c.mu.Unlock()
		return
	}
	p := c.keyMap[ev.code]
	if p == nil {
		if !c.warned[ev.code] {
			c.warned[ev.code] = true
			c.logger.Warnf("ignoring %s from the pedal: no pedal is mapped to it", codeName(ev.code))
		}
		c.mu.Unlock()
		return
	}
	now := time.Now()
	p.raw = ev.value == keyDown
	if p.raw == p.pressed {
		c.mu.Unlock()
		return
	}
	if wait := c.opts.debounce - now.Sub(p.changedAt); wait > 0 {
		c.logger.Debugf("pedal %s %s ignored by debounce (%s)", p.name, map[bool]string{true: "press", false: "release"}[p.raw], p.describe())
		// If the pedal settles in the other state, report it once the
		// debounce window ends, so a pedal is never left stuck down.
		if p.settle == nil {
			p.settle = time.AfterFunc(wait, func() { c.settle(p) })
		}
		c.mu.Unlock()
		return
	}
	fire := c.change(p, now, "")
	c.mu.Unlock()
	c.fire(fire)
}

func (c *controllerCore) settle(p *pedal) {
	c.dispatchMu.Lock()
	defer c.dispatchMu.Unlock()
	c.mu.Lock()
	p.settle = nil
	if c.closed || !c.connected || p.raw == p.pressed {
		c.mu.Unlock()
		return
	}
	fire := c.change(p, time.Now(), "")
	c.mu.Unlock()
	c.fire(fire)
}

// change reports the pedal's raw state and returns the callbacks to fire.
// Call with mu held.
func (c *controllerCore) change(p *pedal, now time.Time, note string) []firing {
	p.pressed = p.raw
	p.changedAt = now
	suffix := ""
	if note != "" {
		suffix = " (" + note + ")"
	}
	if p.pressed {
		p.pressedAt = now
		c.logger.Infof("pedal %s pressed (%s)%s", p.name, p.describe(), suffix)
		return c.record(input.Event{Time: now, Event: input.ButtonPress, Control: p.control, Value: 1})
	}
	c.logger.Infof("pedal %s released after %.2fs (%s)%s", p.name, now.Sub(p.pressedAt).Seconds(), p.describe(), suffix)
	return c.record(input.Event{Time: now, Event: input.ButtonRelease, Control: p.control, Value: 0})
}

// releaseHeld releases every held pedal as if lifted. Call with mu held.
func (c *controllerCore) releaseHeld(note string) []firing {
	var fire []firing
	now := time.Now()
	for _, p := range c.pedals {
		if p.settle != nil {
			p.settle.Stop()
			p.settle = nil
		}
		p.raw = false
		if p.pressed {
			fire = append(fire, c.change(p, now, note)...)
		}
	}
	return fire
}

func (c *controllerCore) disconnected(err error) {
	c.dispatchMu.Lock()
	defer c.dispatchMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.logger.Warnf("pedal disconnected (%v); waiting for it to come back", err)
	fire := c.releaseHeld("device disconnected")
	c.connected = false
	c.dev = nil
	now := time.Now()
	for _, p := range c.pedals {
		fire = append(fire, c.record(input.Event{Time: now, Event: input.Disconnect, Control: p.control})...)
	}
	c.mu.Unlock()
	c.fire(fire)
}

type firing struct {
	fn input.ControlFunction
	ev input.Event
}

// record stores an event and returns the callbacks it triggers. Call with mu held.
func (c *controllerCore) record(ev input.Event) []firing {
	c.events[ev.Control] = ev
	var out []firing
	if fn := c.callbacks[ev.Control][ev.Event]; fn != nil {
		out = append(out, firing{fn, ev})
	}
	if fn := c.callbacks[ev.Control][input.AllEvents]; fn != nil {
		out = append(out, firing{fn, ev})
	}
	return out
}

// fire runs callbacks. Call with dispatchMu held and mu not held.
func (c *controllerCore) fire(fs []firing) {
	for _, f := range fs {
		f.fn(c.ctx, f.ev)
	}
}

func (c *controllerCore) controls() []input.Control {
	out := make([]input.Control, 0, len(c.pedals))
	for _, p := range c.pedals {
		out = append(out, p.control)
	}
	return out
}

func (c *controllerCore) latestEvents() map[input.Control]input.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[input.Control]input.Event, len(c.events))
	for k, v := range c.events {
		out[k] = v
	}
	return out
}

func (c *controllerCore) registerCallback(control input.Control, triggers []input.EventType, fn input.ControlFunction) error {
	known := false
	for _, p := range c.pedals {
		if p.control == control {
			known = true
		}
	}
	if !known {
		return fmt.Errorf("control %q isn't one of this pedal's controls %v", control, c.controls())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.callbacks[control] == nil {
		c.callbacks[control] = map[input.EventType]input.ControlFunction{}
	}
	for _, t := range triggers {
		ts := []input.EventType{t}
		if t == input.ButtonChange {
			ts = []input.EventType{input.ButtonPress, input.ButtonRelease}
		}
		for _, t := range ts {
			if fn == nil {
				delete(c.callbacks[control], t)
			} else {
				c.callbacks[control][t] = fn
			}
		}
	}
	return nil
}

func (c *controllerCore) listPedals() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	pedals := make([]interface{}, 0, len(c.pedals))
	for _, p := range c.pedals {
		pedals = append(pedals, map[string]interface{}{
			"name":           p.name,
			"slot":           p.slot,
			"control":        string(p.control),
			"keycode":        p.key.name,
			"keycode_source": p.source,
			"mode":           p.mode,
			"pressed":        c.connected && p.pressed,
		})
	}
	return map[string]interface{}{
		"model": c.opts.model,
		"device": map[string]interface{}{
			"usb_id":    c.info.usbID,
			"path":      c.info.stablePath(),
			"connected": c.connected,
		},
		"pedals": pedals,
	}
}

func (c *controllerCore) close() {
	c.dispatchMu.Lock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		c.dispatchMu.Unlock()
		return
	}
	var fire []firing
	if c.connected {
		fire = c.releaseHeld("closing")
	}
	c.closed = true
	dev := c.dev
	c.mu.Unlock()
	c.fire(fire)
	c.dispatchMu.Unlock()

	// Releasing the grab leaves the pedal's stored keys as they are, so it
	// keeps working as a plain keyboard.
	c.cancel()
	if dev != nil {
		dev.close()
	}
	<-c.done
}
