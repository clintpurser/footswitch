package footswitch

import (
	"context"
	"fmt"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/utils"
)

// Pcsensor1Pedal is the single-pedal model. Over USB a single-pedal unit is
// identical to a 3-pedal one (same ID, descriptors and three stored slots),
// so the user's choice of this model is what declares the pedal count.
var Pcsensor1Pedal = resource.NewModel("clint", "footswitch", "pcsensor-1-pedal")

func init() {
	resource.RegisterComponent(input.API, Pcsensor1Pedal,
		resource.Registration[input.Controller, *OnePedalConfig]{
			Constructor:           newFootswitchPcsensor1Pedal,
			AttributeMapConverter: convertOnePedalConfig,
		},
	)
}

// OnePedalConfig is the pcsensor-1-pedal attributes.
type OnePedalConfig struct {
	DeviceConfig
	Pedal *PedalConfig `json:"pedal,omitempty"`
	// Slot is the firmware slot the one switch is wired to. The tested unit
	// uses slot 2; this is only for units that turn out to be wired differently.
	Slot *int `json:"slot,omitempty"`
}

// defaultOnePedalSlot is the slot the tested single-pedal unit sends from.
const defaultOnePedalSlot = 2

// convertOnePedalConfig decodes attributes strictly, so a misspelled
// attribute is reported instead of silently ignored.
func convertOnePedalConfig(attrs utils.AttributeMap) (*OnePedalConfig, error) {
	var cfg OnePedalConfig
	if err := decodeStrict(attrs, &cfg); err != nil {
		return nil, fmt.Errorf("invalid attributes (allowed: pedal, slot, device_path, usb_id, debounce_ms; "+
			"the pedal block allows keycode and control): %w", err)
	}
	return &cfg, nil
}

func (cfg *OnePedalConfig) slotNumber() int {
	if cfg.Slot != nil {
		return *cfg.Slot
	}
	return defaultOnePedalSlot
}

// layout is the model's one pedal, on the configured slot.
func (cfg *OnePedalConfig) layout() []pedalLayoutEntry {
	return []pedalLayoutEntry{{"pedal", "pedal", cfg.slotNumber(), input.ButtonSouth}}
}

// Validate checks the config. The input controller has no dependencies.
func (cfg *OnePedalConfig) Validate(path string) ([]string, []string, error) {
	if _, err := cfg.pedalSpecs(path); err != nil {
		return nil, nil, err
	}
	if err := cfg.validateDevice(path); err != nil {
		return nil, nil, err
	}
	return nil, nil, nil
}

// pedalSpecs resolves the pedal block against the model's defaults.
func (cfg *OnePedalConfig) pedalSpecs(path string) ([]pedalSpec, error) {
	if s := cfg.slotNumber(); s < 1 || s > pcsensorSlots {
		return nil, fmt.Errorf("%s: slot must be 1, 2 or 3, got %d", path, s)
	}
	return resolveSpecs(path, cfg.layout(), []*PedalConfig{cfg.Pedal})
}

func newFootswitchPcsensor1Pedal(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (input.Controller, error) {
	conf, err := resource.NativeConfig[*OnePedalConfig](rawConf)
	if err != nil {
		return nil, err
	}
	return NewPcsensor1Pedal(ctx, deps, rawConf.ResourceName(), conf, logger)
}

// NewPcsensor1Pedal finds, grabs and configures a single-pedal unit.
func NewPcsensor1Pedal(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *OnePedalConfig, logger logging.Logger) (input.Controller, error) {
	specs, err := conf.pedalSpecs(name.ShortName())
	if err != nil {
		return nil, err
	}
	return newPedalController(name, Pcsensor1Pedal, specs, &conf.DeviceConfig, logger)
}
