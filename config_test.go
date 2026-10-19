package footswitch

import (
	"strings"
	"testing"

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
		{Config{USBID: "3553-b001"}, "usb_id"},
		{Config{USBID: "3553:B001"}, ""},
		{Config{DebounceMS: &n}, "debounce_ms"},
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
