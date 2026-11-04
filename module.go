package footswitch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/utils"
)

var Pcsensor3Pedal = resource.NewModel("clint", "footswitch", "pcsensor-3-pedal")

func init() {
	resource.RegisterComponent(input.API, Pcsensor3Pedal,
		resource.Registration[input.Controller, *Config]{
			Constructor:           newFootswitchPcsensor3Pedal,
			AttributeMapConverter: convertConfig,
		},
	)
}

// PedalConfig is one pedal's block in the config.
type PedalConfig struct {
	Keycode string `json:"keycode,omitempty"`
	Control string `json:"control,omitempty"`
}

// DeviceConfig is the attributes every pedal model shares: how to find the
// device and how to debounce it.
type DeviceConfig struct {
	DevicePath string `json:"device_path,omitempty"`
	USBID      string `json:"usb_id,omitempty"`
	DebounceMS *int   `json:"debounce_ms,omitempty"`
}

// Config is the pcsensor-3-pedal attributes.
type Config struct {
	DeviceConfig
	Left   *PedalConfig `json:"left,omitempty"`
	Center *PedalConfig `json:"center,omitempty"`
	Right  *PedalConfig `json:"right,omitempty"`
}

const defaultDebounceMS = 25

var usbIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{4}$`)

// validateDevice checks the shared device attributes.
func (d *DeviceConfig) validateDevice(path string) error {
	if d.USBID != "" && !usbIDPattern.MatchString(d.USBID) {
		return fmt.Errorf("%s: usb_id %q must be in vvvv:pppp hex form, e.g. 3553:b001", path, d.USBID)
	}
	if d.DebounceMS != nil && *d.DebounceMS <= 0 {
		return fmt.Errorf("%s: debounce_ms must be a positive integer, got %d", path, *d.DebounceMS)
	}
	return nil
}

// debounce returns the configured debounce window or the default.
func (d *DeviceConfig) debounce() time.Duration {
	ms := defaultDebounceMS
	if d.DebounceMS != nil {
		ms = *d.DebounceMS
	}
	return time.Duration(ms) * time.Millisecond
}

// openDevice finds and grabs the configured pedal on the host.
func (d *DeviceConfig) openDevice() (*openedDevice, error) {
	return openPedal(hostPaths, d.DevicePath, d.USBID)
}

// convertConfig decodes attributes strictly, so a misspelled attribute is
// reported instead of silently ignored.
func convertConfig(attrs utils.AttributeMap) (*Config, error) {
	var cfg Config
	if err := decodeStrict(attrs, &cfg); err != nil {
		return nil, fmt.Errorf("invalid attributes (allowed: left, center, right, device_path, usb_id, debounce_ms; "+
			"pedal blocks allow keycode and control): %w", err)
	}
	return &cfg, nil
}

func decodeStrict(attrs utils.AttributeMap, into interface{}) error {
	raw, err := json.Marshal(attrs)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}

// pedalLayoutEntry is one pedal of a model: its config block name, the
// label used in logs and errors, the firmware slot it reads and programs,
// and its default control.
type pedalLayoutEntry struct {
	name    string
	label   string
	slot    int
	control input.Control
}

// threePedalLayout is the pcsensor-3-pedal model's pedals, in slot order.
var threePedalLayout = []pedalLayoutEntry{
	{"left", "pedal left", 1, input.ButtonLT},
	{"center", "pedal center", 2, input.ButtonSelect},
	{"right", "pedal right", 3, input.ButtonRT},
}

func (cfg *Config) pedalBlocks() []*PedalConfig {
	return []*PedalConfig{cfg.Left, cfg.Center, cfg.Right}
}

// Validate checks the config. The input controller has no dependencies.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if _, err := cfg.pedalSpecs(path); err != nil {
		return nil, nil, err
	}
	if err := cfg.validateDevice(path); err != nil {
		return nil, nil, err
	}
	return nil, nil, nil
}

// pedalSpecs resolves the pedal blocks against the model's defaults.
func (cfg *Config) pedalSpecs(path string) ([]pedalSpec, error) {
	return resolveSpecs(path, threePedalLayout, cfg.pedalBlocks())
}

// resolveSpecs walks a model's layout and applies the matching pedal
// blocks (blocks[i] goes with layout[i]; nil means all defaults).
func resolveSpecs(path string, layout []pedalLayoutEntry, blocks []*PedalConfig) ([]pedalSpec, error) {
	var specs []pedalSpec
	controls := map[input.Control]string{}
	keys := map[string]string{}
	for i, entry := range layout {
		var block *PedalConfig
		if i < len(blocks) {
			block = blocks[i]
		}
		s := pedalSpec{name: entry.name, label: entry.label, slot: entry.slot, control: entry.control}
		if block != nil {
			if block.Keycode != "" {
				k, err := lookupKeycode(block.Keycode)
				if err != nil {
					return nil, fmt.Errorf("%s: %s.keycode: %w", path, entry.name, err)
				}
				if other, ok := keys[k.name]; ok {
					return nil, fmt.Errorf("%s: %s.keycode and %s.keycode are both %s", path, other, entry.name, k.name)
				}
				keys[k.name] = entry.name
				s.keycode = &k
			}
			if block.Control != "" {
				if !isButtonControl(block.Control) {
					return nil, fmt.Errorf("%s: %s.control %q is not a Viam button control (one of %s)",
						path, entry.name, block.Control, strings.Join(controlNames(buttonControls), ", "))
				}
				s.control = input.Control(block.Control)
			}
		}
		if other, ok := controls[s.control]; ok {
			return nil, fmt.Errorf("%s: pedals %s and %s both use control %s", path, other, entry.name, s.control)
		}
		controls[s.control] = entry.name
		specs = append(specs, s)
	}
	return specs, nil
}

// pedalController is the input controller resource both pedal models return.
// It carries no model-specific state; the core holds the model name and pedals.
type pedalController struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	core   *controllerCore
}

func newFootswitchPcsensor3Pedal(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (input.Controller, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}
	return NewPcsensor3Pedal(ctx, deps, rawConf.ResourceName(), conf, logger)
}

// NewPcsensor3Pedal finds, grabs and configures the pedal.
func NewPcsensor3Pedal(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (input.Controller, error) {
	specs, err := conf.pedalSpecs(name.ShortName())
	if err != nil {
		return nil, err
	}
	return newPedalController(name, Pcsensor3Pedal, specs, &conf.DeviceConfig, logger)
}

func newPedalController(name resource.Name, model resource.Model, specs []pedalSpec, dev *DeviceConfig, logger logging.Logger) (input.Controller, error) {
	core, err := newControllerCore(coreOptions{
		model:    model.Name,
		pedals:   specs,
		debounce: dev.debounce(),
		open:     dev.openDevice,
	}, logger)
	if err != nil {
		return nil, err
	}
	return &pedalController{name: name, logger: logger, core: core}, nil
}

func (s *pedalController) Name() resource.Name {
	return s.name
}

// Controls returns one control per pedal.
func (s *pedalController) Controls(ctx context.Context, extra map[string]interface{}) ([]input.Control, error) {
	return s.core.controls(), nil
}

// Events returns the latest event for each pedal.
func (s *pedalController) Events(ctx context.Context, extra map[string]interface{}) (map[input.Control]input.Event, error) {
	return s.core.latestEvents(), nil
}

// RegisterControlCallback calls back on ButtonPress, ButtonRelease, Connect and Disconnect.
func (s *pedalController) RegisterControlCallback(
	ctx context.Context,
	control input.Control,
	triggers []input.EventType,
	ctrlFunc input.ControlFunction,
	extra map[string]interface{},
) error {
	return s.core.registerCallback(control, triggers, ctrlFunc)
}

var pedalCommands = []string{"list_pedals"}

func (s *pedalController) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	switch cmd["command"] {
	case "list_pedals":
		return s.core.listPedals(), nil
	default:
		return nil, fmt.Errorf("unknown command %v; supported: %s (as {\"command\": \"list_pedals\"})",
			cmd["command"], strings.Join(pedalCommands, ", "))
	}
}

func (s *pedalController) Status(ctx context.Context) (map[string]interface{}, error) {
	return s.core.listPedals(), nil
}

func (s *pedalController) Close(context.Context) error {
	s.core.close()
	return nil
}

// Viam control names, for validation.
var (
	buttonControls = []input.Control{
		input.ButtonSouth, input.ButtonEast, input.ButtonWest, input.ButtonNorth,
		input.ButtonLT, input.ButtonRT, input.ButtonLT2, input.ButtonRT2,
		input.ButtonLThumb, input.ButtonRThumb, input.ButtonSelect, input.ButtonStart,
		input.ButtonMenu, input.ButtonRecord, input.ButtonEStop,
	}
	otherControls = []input.Control{
		input.AbsoluteX, input.AbsoluteY, input.AbsoluteZ, input.AbsoluteRX, input.AbsoluteRY,
		input.AbsoluteRZ, input.AbsoluteHat0X, input.AbsoluteHat0Y,
		input.AbsolutePedalAccelerator, input.AbsolutePedalBrake, input.AbsolutePedalClutch,
	}
)

func isButtonControl(name string) bool {
	for _, c := range buttonControls {
		if string(c) == name {
			return true
		}
	}
	return false
}

func isControl(name string) bool {
	if isButtonControl(name) {
		return true
	}
	for _, c := range otherControls {
		if string(c) == name {
			return true
		}
	}
	return false
}

func controlNames(cs []input.Control) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = string(c)
	}
	sort.Strings(out)
	return out
}
