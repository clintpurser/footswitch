package footswitch

import (
	"strings"
	"testing"
	"time"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/utils"
)

func TestConvertConfigRejectsUnknownKeys(t *testing.T) {
	if _, err := convertConfig(utils.AttributeMap{"left": map[string]interface{}{"keycode": "KEY_F13"}, "debounce_ms": 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := convertConfig(nil); err != nil {
		t.Fatal(err)
	}
	for _, attrs := range []utils.AttributeMap{
		{"middle": map[string]interface{}{}},
		{"left": map[string]interface{}{"key": "KEY_F13"}},
		{"debounce_ms": 2.5},
	} {
		if _, err := convertConfig(attrs); err == nil {
			t.Errorf("%v: expected an error", attrs)
		}
	}
}

func TestValidate(t *testing.T) {
	n := 0
	cases := []struct {
		cfg  Config
		want string
	}{
		{Config{}, ""},
		{Config{Left: &PedalConfig{Keycode: "KEY_F13"}, Right: &PedalConfig{Control: "ButtonEStop"}}, ""},
		{Config{Left: &PedalConfig{Keycode: "KEY_NOPE"}}, "left.keycode"},
		{Config{Left: &PedalConfig{Keycode: "KEY_LEFTCTRL"}}, "modifier"},
		{Config{Left: &PedalConfig{Control: "Jump"}}, "left.control"},
		{Config{Left: &PedalConfig{Control: "ButtonRT"}}, "both use control ButtonRT"},
		{Config{Left: &PedalConfig{Keycode: "KEY_F13"}, Center: &PedalConfig{Keycode: "KEY_F13"}}, "both KEY_F13"},
		{Config{DeviceConfig: DeviceConfig{USBID: "3553-b001"}}, "usb_id"},
		{Config{DeviceConfig: DeviceConfig{USBID: "3553:B001"}}, ""},
		{Config{DeviceConfig: DeviceConfig{DebounceMS: &n}}, "debounce_ms"},
	}
	for _, c := range cases {
		deps, _, err := c.cfg.Validate("components.0")
		if len(deps) != 0 {
			t.Errorf("unexpected deps %v", deps)
		}
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%+v: %v", c.cfg, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%+v: got %v, want error containing %q", c.cfg, err, c.want)
		}
	}
}

func TestValidateActions(t *testing.T) {
	press := func(a Action) map[string]ControlActions {
		return map[string]ControlActions{"ButtonLT": {OnPress: &a}}
	}
	zero := 0
	cases := []struct {
		cfg  ActionsConfig
		want string
	}{
		{ActionsConfig{}, "input_controller"},
		{ActionsConfig{InputController: "pedals"}, ""},
		{ActionsConfig{InputController: "pedals", Controls: map[string]ControlActions{"Foo": {}}}, "controls.Foo"},
		{ActionsConfig{InputController: "pedals", Controls: map[string]ControlActions{"ButtonLt": {}}}, "not a pedal name"},
		{ActionsConfig{InputController: "pedals", Controls: map[string]ControlActions{"pedal": {}, "left": {}}}, ""},
		{ActionsConfig{InputController: "pedals", Controls: press(Action{Method: "do_command"})}, "component"},
		{ActionsConfig{InputController: "pedals", Controls: press(Action{Component: "arm"})}, "method"},
		{ActionsConfig{InputController: "pedals", Controls: press(Action{Component: "arm", Method: "stop"})}, "only do_command"},
		{ActionsConfig{InputController: "pedals", Controls: press(Action{Component: "arm", Method: "do_command", Args: "x"})}, "args"},
		{ActionsConfig{InputController: "pedals", ActionTimeoutMS: &zero}, "action_timeout_ms"},
	}
	for _, c := range cases {
		_, _, err := c.cfg.Validate("services.0")
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%+v: %v", c.cfg, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%+v: got %v, want error containing %q", c.cfg, err, c.want)
		}
	}

	cfg := ActionsConfig{InputController: "pedals", Controls: map[string]ControlActions{
		"ButtonLT":     {OnPress: &Action{Component: "arm", Method: "do_command"}},
		"ButtonSelect": {OnPress: &Action{Component: "gripper", Method: "do_command"}, OnRelease: &Action{Component: "gripper", Method: "do_command"}},
	}}
	deps, _, err := cfg.Validate("services.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deps, ",") != "pedals,arm,gripper" {
		t.Fatalf("deps = %v", deps)
	}
}

func TestConvertOnePedalConfigRejectsUnknownKeys(t *testing.T) {
	cfg, err := convertOnePedalConfig(utils.AttributeMap{"pedal": map[string]interface{}{"keycode": "KEY_F14"}, "slot": 2, "debounce_ms": 20})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Pedal == nil || cfg.Pedal.Keycode != "KEY_F14" || cfg.Slot == nil || *cfg.Slot != 2 || cfg.DebounceMS == nil || *cfg.DebounceMS != 20 {
		t.Fatalf("decoded %+v", cfg)
	}
	if _, err := convertOnePedalConfig(nil); err != nil {
		t.Fatal(err)
	}
	for _, attrs := range []utils.AttributeMap{
		{"left": map[string]interface{}{}},
		{"pedal": map[string]interface{}{"key": "x"}},
		{"slot": "two"},
	} {
		if _, err := convertOnePedalConfig(attrs); err == nil {
			t.Errorf("%v: expected an error", attrs)
		}
	}
}

func TestValidateOnePedal(t *testing.T) {
	zero, one, three, four := 0, 1, 3, 4
	cases := []struct {
		cfg  OnePedalConfig
		want string
	}{
		{OnePedalConfig{}, ""},
		{OnePedalConfig{Slot: &one}, ""},
		{OnePedalConfig{Slot: &three}, ""},
		{OnePedalConfig{Pedal: &PedalConfig{Keycode: "KEY_F14", Control: "ButtonEStop"}}, ""},
		{OnePedalConfig{Slot: &zero}, "slot"},
		{OnePedalConfig{Slot: &four}, "slot"},
		{OnePedalConfig{Pedal: &PedalConfig{Keycode: "KEY_NOPE"}}, "pedal.keycode"},
		{OnePedalConfig{Pedal: &PedalConfig{Keycode: "KEY_LEFTCTRL"}}, "modifier"},
		{OnePedalConfig{Pedal: &PedalConfig{Control: "Jump"}}, "pedal.control"},
		{OnePedalConfig{DeviceConfig: DeviceConfig{USBID: "3553-b001"}}, "usb_id"},
		{OnePedalConfig{DeviceConfig: DeviceConfig{DebounceMS: &zero}}, "debounce_ms"},
	}
	for _, c := range cases {
		deps, _, err := c.cfg.Validate("components.0")
		if len(deps) != 0 {
			t.Errorf("unexpected deps %v", deps)
		}
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%+v: %v", c.cfg, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%+v: got %v, want error containing %q", c.cfg, err, c.want)
		}
	}
}

func TestLayouts(t *testing.T) {
	specs, err := (&Config{}).pedalSpecs("test")
	if err != nil {
		t.Fatal(err)
	}
	want := []pedalSpec{
		{name: "left", label: "pedal left", slot: 1, control: input.ButtonLT},
		{name: "center", label: "pedal center", slot: 2, control: input.ButtonSelect},
		{name: "right", label: "pedal right", slot: 3, control: input.ButtonRT},
	}
	if len(specs) != 3 {
		t.Fatalf("3-pedal specs = %+v", specs)
	}
	for i := range want {
		if specs[i] != want[i] {
			t.Errorf("3-pedal spec %d = %+v, want %+v", i, specs[i], want[i])
		}
	}

	specs, err = (&OnePedalConfig{}).pedalSpecs("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0] != (pedalSpec{name: "pedal", label: "pedal", slot: 2, control: input.ButtonSouth}) {
		t.Fatalf("1-pedal specs = %+v", specs)
	}
	three := 3
	specs, err = (&OnePedalConfig{Slot: &three}).pedalSpecs("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].slot != 3 {
		t.Fatalf("1-pedal slot 3 specs = %+v", specs)
	}
}

func TestDebounceDefault(t *testing.T) {
	if d := (&DeviceConfig{}).debounce(); d != 25*time.Millisecond {
		t.Fatalf("default debounce = %s", d)
	}
	ten := 10
	if d := (&DeviceConfig{DebounceMS: &ten}).debounce(); d != 10*time.Millisecond {
		t.Fatalf("debounce = %s", d)
	}
}
