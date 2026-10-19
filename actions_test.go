package footswitch

import (
	"context"
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
}

func (f *fakeController) Controls(context.Context, map[string]interface{}) ([]input.Control, error) {
	return []input.Control{input.ButtonLT, input.ButtonSelect, input.ButtonRT}, nil
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

func (f *fakeController) DoCommand(context.Context, map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
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
