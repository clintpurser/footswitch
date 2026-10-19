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

// Config is the pcsensor-3-pedal attributes.
type Config struct {
	Left       *PedalConfig `json:"left,omitempty"`
	Center     *PedalConfig `json:"center,omitempty"`
	Right      *PedalConfig `json:"right,omitempty"`
	DevicePath string       `json:"device_path,omitempty"`
	USBID      string       `json:"usb_id,omitempty"`
	DebounceMS *int         `json:"debounce_ms,omitempty"`
}

const defaultDebounceMS = 25

// convertConfig decodes attributes strictly, so a misspelled attribute is
// reported instead of silently ignored.
func convertConfig(attrs utils.AttributeMap) (*Config, error) {
	raw, err := json.Marshal(attrs)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid attributes (allowed: left, center, right, device_path, usb_id, debounce_ms; "+
			"pedal blocks allow keycode and control): %w", err)
	}
	return &cfg, nil
}

// pedalLayout is the model's pedal names and default controls, by slot.
var pedalLayout = []struct {
	name    string
	control input.Control
}{
	{"left", input.ButtonLT},
	{"center", input.ButtonSelect},
	{"right", input.ButtonRT},
}

func (cfg *Config) pedalBlocks() []*PedalConfig {
	return []*PedalConfig{cfg.Left, cfg.Center, cfg.Right}
}

var usbIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{4}$`)

// Validate checks the config. The input controller has no dependencies.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	_, err := cfg.pedalSpecs(path)
	if err != nil {
		return nil, nil, err
	}
	if cfg.USBID != "" && !usbIDPattern.MatchString(cfg.USBID) {
		return nil, nil, fmt.Errorf("%s: usb_id %q must be in vvvv:pppp hex form, e.g. 3553:b001", path, cfg.USBID)
	}
	if cfg.DebounceMS != nil && *cfg.DebounceMS <= 0 {
		return nil, nil, fmt.Errorf("%s: debounce_ms must be a positive integer, got %d", path, *cfg.DebounceMS)
	}
	return nil, nil, nil
}

// pedalSpecs resolves the pedal blocks against the model's defaults.
func (cfg *Config) pedalSpecs(path string) ([]pedalSpec, error) {
	var specs []pedalSpec
	controls := map[input.Control]string{}
	keys := map[string]string{}
	for i, block := range cfg.pedalBlocks() {
		layout := pedalLayout[i]
		s := pedalSpec{name: layout.name, slot: i + 1, control: layout.control}
		if block != nil {
			if block.Keycode != "" {
				k, err := lookupKeycode(block.Keycode)
				if err != nil {
					return nil, fmt.Errorf("%s: %s.keycode: %w", path, layout.name, err)
				}
				if other, ok := keys[k.name]; ok {
					return nil, fmt.Errorf("%s: %s.keycode and %s.keycode are both %s", path, other, layout.name, k.name)
				}
				keys[k.name] = layout.name
				s.keycode = &k
			}
			if block.Control != "" {
				if !isButtonControl(block.Control) {
					return nil, fmt.Errorf("%s: %s.control %q is not a Viam button control (one of %s)",
						path, layout.name, block.Control, strings.Join(controlNames(buttonControls), ", "))
				}
				s.control = input.Control(block.Control)
			}
		}
		if other, ok := controls[s.control]; ok {
			return nil, fmt.Errorf("%s: pedals %s and %s both use control %s", path, other, layout.name, s.control)
		}
		controls[s.control] = layout.name
		specs = append(specs, s)
	}
	return specs, nil
}

type footswitchPcsensor3Pedal struct {
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
	debounce := defaultDebounceMS
	if conf.DebounceMS != nil {
		debounce = *conf.DebounceMS
	}
	core, err := newControllerCore(coreOptions{
		model:    Pcsensor3Pedal.Name,
		pedals:   specs,
		debounce: time.Duration(debounce) * time.Millisecond,
		open: func() (*openedDevice, error) {
			return openPedal(hostPaths, conf.DevicePath, conf.USBID)
		},
	}, logger)
	if err != nil {
		return nil, err
	}
	return &footswitchPcsensor3Pedal{name: name, logger: logger, core: core}, nil
}

func (s *footswitchPcsensor3Pedal) Name() resource.Name {
	return s.name
}

// Controls returns one control per pedal.
func (s *footswitchPcsensor3Pedal) Controls(ctx context.Context, extra map[string]interface{}) ([]input.Control, error) {
	return s.core.controls(), nil
}

// Events returns the latest event for each pedal.
func (s *footswitchPcsensor3Pedal) Events(ctx context.Context, extra map[string]interface{}) (map[input.Control]input.Event, error) {
	return s.core.latestEvents(), nil
}

// RegisterControlCallback calls back on ButtonPress, ButtonRelease, Connect and Disconnect.
func (s *footswitchPcsensor3Pedal) RegisterControlCallback(
	ctx context.Context,
	control input.Control,
	triggers []input.EventType,
	ctrlFunc input.ControlFunction,
	extra map[string]interface{},
) error {
	return s.core.registerCallback(control, triggers, ctrlFunc)
}

var pedalCommands = []string{"list_pedals"}

func (s *footswitchPcsensor3Pedal) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	switch cmd["command"] {
	case "list_pedals":
		return s.core.listPedals(), nil
	default:
		return nil, fmt.Errorf("unknown command %v; supported: %s (as {\"command\": \"list_pedals\"})",
			cmd["command"], strings.Join(pedalCommands, ", "))
	}
}

func (s *footswitchPcsensor3Pedal) Status(ctx context.Context) (map[string]interface{}, error) {
	return s.core.listPedals(), nil
}

func (s *footswitchPcsensor3Pedal) Close(context.Context) error {
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
