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
	"go.viam.com/rdk/resource"
)

// fakeController is an input controller whose events the test triggers.
type fakeController struct {
	resource.Named
	resource.TriviallyCloseable
	resource.TriviallyReconfigurable

	mu        sync.Mutex
	callbacks map[input.Control]map[input.EventType]input.ControlFunction
	// pedals, when set, is what list_pedals returns, in the shape the pedal
	// models produce. When nil the controller has no DoCommands, like a gamepad.
	pedals []interface{}
}

func (f *fakeController) Controls(context.Context, map[string]interface{}) ([]input.Control, error) {
	return []input.Control{input.ButtonLT, input.ButtonSelect, input.ButtonRT, input.ButtonSouth}, nil
}

func (f *fakeController) Events(context.Context, map[string]interface{}) (map[input.Control]input.Event, error) {
	return nil, nil
}

func (f *fakeController) RegisterControlCallback(_ context.Context, c input.Control, ts []input.EventType, fn input.ControlFunction, _ map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.callbacks[c] == nil {
		f.callbacks[c] = map[input.EventType]input.ControlFunction{}
	}
	for _, t := range ts {
		if fn == nil {
			delete(f.callbacks[c], t)
		} else {
			f.callbacks[c][t] = fn
		}
	}
	return nil
}

func (f *fakeController) DoCommand(_ context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	if f.pedals == nil || cmd["command"] != "list_pedals" {
		return nil, errors.New("unknown command")
	}
	return map[string]interface{}{"model": "pcsensor-1-pedal", "pedals": f.pedals}, nil
}

func pedalEntry(name string, control input.Control) interface{} {
	return map[string]interface{}{"name": name, "slot": 2, "control": string(control), "keycode": "KEY_B"}
}

func newFakeController(pedals ...interface{}) *fakeController {
	return &fakeController{Named: input.Named("footswitch").AsNamed(),
		callbacks: map[input.Control]map[input.EventType]input.ControlFunction{}, pedals: pedals}
}

func (f *fakeController) Status(context.Context) (map[string]interface{}, error) { return nil, nil }

func (f *fakeController) trigger(c input.Control, t input.EventType) {
	f.mu.Lock()
	fn := f.callbacks[c][t]
	f.mu.Unlock()
	if fn != nil {
		fn(context.Background(), input.Event{Time: time.Now(), Event: t, Control: c})
	}
}

// recorder is a target resource that records DoCommand calls.
type recorder struct {
	resource.Named
	resource.TriviallyCloseable
	resource.TriviallyReconfigurable

	delay time.Duration
	calls chan map[string]interface{}
}

func (r *recorder) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	select {
	case <-time.After(r.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	r.calls <- cmd
	return nil, nil
}

func (r *recorder) Status(context.Context) (map[string]interface{}, error) { return nil, nil }

func TestActionsRunInOrderPerControl(t *testing.T) {
	ctrl := &fakeController{Named: input.Named("pedals").AsNamed(), callbacks: map[input.Control]map[input.EventType]input.ControlFunction{}}
	gripper := &recorder{Named: resource.NewName(resource.APINamespaceRDK.WithComponentType("gripper"), "gripper").AsNamed(),
		delay: 30 * time.Millisecond, calls: make(chan map[string]interface{}, 32)}
	deps := resource.Dependencies{ctrl.Name(): ctrl, gripper.Name(): gripper}

	timeout := 1000
	cfg := &ActionsConfig{
		InputController: "pedals",
		ActionTimeoutMS: &timeout,
		Controls: map[string]ControlActions{
			"ButtonSelect": {
				OnPress:   &Action{Component: "gripper", Method: "do_command", Args: map[string]interface{}{"cmd": "close"}},
				OnRelease: &Action{Component: "gripper", Method: "do_command", Args: map[string]interface{}{"cmd": "open"}},
			},
		},
	}
	if _, _, err := cfg.Validate("services.0"); err != nil {
		t.Fatal(err)
	}
	svc, err := NewActions(context.Background(), deps, resource.NewName(resource.APINamespaceRDK.WithServiceType("generic"), "a"), cfg, logging.NewTestLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close(context.Background())

	// Callbacks must return immediately even though actions take 30 ms.
	start := time.Now()
	ctrl.trigger(input.ButtonSelect, input.ButtonPress)
	ctrl.trigger(input.ButtonSelect, input.ButtonRelease)
	if time.Since(start) > 10*time.Millisecond {
		t.Fatal("callback blocked on the action")
	}
	for _, want := range []string{"close", "open"} {
		select {
		case got := <-gripper.calls:
			if got["cmd"] != want {
				t.Fatalf("got %v, want %s", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

func TestActionsTimeoutAndDrop(t *testing.T) {
	ctrl := &fakeController{Named: input.Named("pedals").AsNamed(), callbacks: map[input.Control]map[input.EventType]input.ControlFunction{}}
	arm := &recorder{Named: resource.NewName(resource.APINamespaceRDK.WithComponentType("arm"), "arm").AsNamed(),
		delay: time.Hour, calls: make(chan map[string]interface{}, 64)}
	deps := resource.Dependencies{ctrl.Name(): ctrl, arm.Name(): arm}
	timeout := 20
	cfg := &ActionsConfig{
		InputController: "pedals",
		ActionTimeoutMS: &timeout,
		Controls:        map[string]ControlActions{"ButtonLT": {OnPress: &Action{Component: "arm", Method: "do_command"}}},
	}
	svc, err := NewActions(context.Background(), deps, resource.NewName(resource.APINamespaceRDK.WithServiceType("generic"), "a"), cfg, logging.NewTestLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	// Overfill the queue; extra presses are dropped instead of blocking.
	for i := 0; i < actionQueueSize+10; i++ {
		ctrl.trigger(input.ButtonLT, input.ButtonPress)
	}
	time.Sleep(50 * time.Millisecond)
	if err := svc.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(arm.calls) != 0 {
		t.Fatal("timed-out actions should not complete")
	}
}

func TestActionsByPedalName(t *testing.T) {
	ctrl := newFakeController(pedalEntry("pedal", input.ButtonSouth))
	teleop := &recorder{Named: resource.NewName(resource.APINamespaceRDK.WithServiceType("generic"), "teleop-lf").AsNamed(),
		calls: make(chan map[string]interface{}, 8)}
	deps := resource.Dependencies{ctrl.Name(): ctrl, teleop.Name(): teleop}
	cfg := &ActionsConfig{
		InputController: "footswitch",
		Controls: map[string]ControlActions{
			"pedal": {
				OnPress:   &Action{Component: "teleop-lf", Method: "do_command", Args: map[string]interface{}{"command": "start"}},
				OnRelease: &Action{Component: "teleop-lf", Method: "do_command", Args: map[string]interface{}{"command": "stop"}},
			},
		},
	}
	if _, _, err := cfg.Validate("services.0"); err != nil {
		t.Fatal(err)
	}
	svc, err := NewActions(context.Background(), deps, resource.NewName(resource.APINamespaceRDK.WithServiceType("generic"), "a"), cfg, logging.NewTestLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close(context.Background())

	// The pedal named "pedal" reports as ButtonSouth.
	ctrl.trigger(input.ButtonSouth, input.ButtonPress)
	ctrl.trigger(input.ButtonSouth, input.ButtonRelease)
	for _, want := range []string{"start", "stop"} {
		select {
		case got := <-teleop.calls:
			if got["command"] != want {
				t.Fatalf("got %v, want %s", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s", want)
		}
	}
	st, err := svc.(*footswitchActions).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st["queued"].(map[string]interface{})["pedal"]; !ok {
		t.Fatalf("status should key queues by the configured name: %v", st)
	}
}

func TestResolveControls(t *testing.T) {
	ctx := context.Background()
	three := newFakeController(pedalEntry("left", input.ButtonLT), pedalEntry("center", input.ButtonSelect), pedalEntry("right", input.ButtonRT))

	got, err := resolveControls(ctx, three, "pedals", []string{"ButtonSouth", "center", "left"})
	if err != nil {
		t.Fatal(err)
	}
	if got["left"] != input.ButtonLT || got["center"] != input.ButtonSelect || got["ButtonSouth"] != input.ButtonSouth {
		t.Fatalf("resolved %v", got)
	}

	// Control names alone never call list_pedals, so gamepads keep working.
	if _, err := resolveControls(ctx, newFakeController(), "gamepad", []string{"ButtonLT"}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		ctrl *fakeController
		keys []string
		want string
	}{
		{three, []string{"pedal"}, `has no pedal named "pedal" (it has center, left, right)`},
		{newFakeController(), []string{"pedal"}, "doesn't list pedal names"},
		{three, []string{"ButtonLT", "left"}, "both refer to control ButtonLT"},
	} {
		_, err := resolveControls(ctx, c.ctrl, "pedals", c.keys)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want error containing %q", c.keys, err, c.want)
		}
	}
}
