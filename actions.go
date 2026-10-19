package footswitch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	generic "go.viam.com/rdk/services/generic"
)

var Actions = resource.NewModel("clint", "footswitch", "actions")

func init() {
	resource.RegisterService(generic.API, Actions,
		resource.Registration[resource.Resource, *ActionsConfig]{
			Constructor: newFootswitchActions,
		},
	)
}

// Action calls a method on another resource.
type Action struct {
	Component string      `json:"component"`
	Method    string      `json:"method"`
	Args      interface{} `json:"args,omitempty"`
}

// ControlActions are the actions for one control.
type ControlActions struct {
	OnPress   *Action `json:"on_press,omitempty"`
	OnRelease *Action `json:"on_release,omitempty"`
}

// ActionsConfig is the actions service's attributes.
type ActionsConfig struct {
	InputController string                    `json:"input_controller"`
	Controls        map[string]ControlActions `json:"controls,omitempty"`
	ActionTimeoutMS *int                      `json:"action_timeout_ms,omitempty"`
}

const (
	defaultActionTimeoutMS = 5000
	actionQueueSize        = 16
)

// Validate checks the config and declares the input controller and every
// action's component as dependencies.
func (cfg *ActionsConfig) Validate(path string) ([]string, []string, error) {
	if cfg.InputController == "" {
		return nil, nil, resource.NewConfigValidationFieldRequiredError(path, "input_controller")
	}
	if cfg.ActionTimeoutMS != nil && *cfg.ActionTimeoutMS <= 0 {
		return nil, nil, fmt.Errorf("%s: action_timeout_ms must be a positive integer, got %d", path, *cfg.ActionTimeoutMS)
	}
	deps := []string{cfg.InputController}
	seen := map[string]bool{cfg.InputController: true}
	for _, control := range sortedKeys(cfg.Controls) {
		if !isControl(control) {
			return nil, nil, fmt.Errorf("%s: controls.%s is not a Viam control name (for example ButtonLT)", path, control)
		}
		ca := cfg.Controls[control]
		for _, a := range []struct {
			field  string
			action *Action
		}{{"on_press", ca.OnPress}, {"on_release", ca.OnRelease}} {
			if a.action == nil {
				continue
			}
			field := fmt.Sprintf("controls.%s.%s", control, a.field)
			if a.action.Component == "" {
				return nil, nil, resource.NewConfigValidationFieldRequiredError(path, field+".component")
			}
			if a.action.Method == "" {
				return nil, nil, resource.NewConfigValidationFieldRequiredError(path, field+".method")
			}
			if a.action.Method != "do_command" {
				return nil, nil, fmt.Errorf("%s: %s.method %q is not supported; only do_command is", path, field, a.action.Method)
			}
			if a.action.Args != nil {
				if _, ok := a.action.Args.(map[string]interface{}); !ok {
					return nil, nil, fmt.Errorf("%s: %s.args must be an object", path, field)
				}
			}
			if !seen[a.action.Component] {
				seen[a.action.Component] = true
				deps = append(deps, a.action.Component)
			}
		}
	}
	return deps, nil, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// boundAction is an action with its target resolved.
type boundAction struct {
	Action
	target resource.Resource
	args   map[string]interface{}
}

type queuedAction struct {
	control input.Control
	event   input.EventType
	action  *boundAction
}

type footswitchActions struct {
	resource.AlwaysRebuild

	name       resource.Name
	logger     logging.Logger
	controller input.Controller
	timeout    time.Duration
	controls   map[input.Control]*controlWorker

	cancelCtx  context.Context
	cancelFunc func()
	wg         sync.WaitGroup

	mu        sync.Mutex
	connected bool
}

type controlWorker struct {
	control   input.Control
	onPress   *boundAction
	onRelease *boundAction
	queue     chan queuedAction
}

func newFootswitchActions(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (resource.Resource, error) {
	conf, err := resource.NativeConfig[*ActionsConfig](rawConf)
	if err != nil {
		return nil, err
	}
	return NewActions(ctx, deps, rawConf.ResourceName(), conf, logger)
}

// lookupByShortName finds a dependency by its name in the config.
func lookupByShortName(deps resource.Dependencies, name string) (resource.Resource, error) {
	for n, r := range deps {
		if n.ShortName() == name || n.Name == name {
			return r, nil
		}
	}
	return nil, fmt.Errorf("resource %q not found in dependencies", name)
}

func bind(deps resource.Dependencies, a *Action) (*boundAction, error) {
	if a == nil {
		return nil, nil
	}
	target, err := lookupByShortName(deps, a.Component)
	if err != nil {
		return nil, err
	}
	args, _ := a.Args.(map[string]interface{})
	if args == nil {
		args = map[string]interface{}{}
	}
	return &boundAction{Action: *a, target: target, args: args}, nil
}

// NewActions resolves the controller and targets and registers callbacks.
func NewActions(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *ActionsConfig, logger logging.Logger) (resource.Resource, error) {
	res, err := lookupByShortName(deps, conf.InputController)
	if err != nil {
		return nil, err
	}
	controller, ok := res.(input.Controller)
	if !ok {
		return nil, fmt.Errorf("input_controller %q is not an input controller", conf.InputController)
	}
	timeout := defaultActionTimeoutMS
	if conf.ActionTimeoutMS != nil {
		timeout = *conf.ActionTimeoutMS
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	s := &footswitchActions{
		name:       name,
		logger:     logger,
		controller: controller,
		timeout:    time.Duration(timeout) * time.Millisecond,
		controls:   map[input.Control]*controlWorker{},
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		connected:  true,
	}
	for _, control := range sortedKeys(conf.Controls) {
		ca := conf.Controls[control]
		w := &controlWorker{control: input.Control(control), queue: make(chan queuedAction, actionQueueSize)}
		if w.onPress, err = bind(deps, ca.OnPress); err != nil {
			cancelFunc()
			return nil, fmt.Errorf("controls.%s.on_press: %w", control, err)
		}
		if w.onRelease, err = bind(deps, ca.OnRelease); err != nil {
			cancelFunc()
			return nil, fmt.Errorf("controls.%s.on_release: %w", control, err)
		}
		s.controls[w.control] = w
	}

	if available, err := controller.Controls(ctx, nil); err != nil {
		logger.Warnf("can't list %s's controls: %v", conf.InputController, err)
	} else {
		have := map[input.Control]bool{}
		for _, c := range available {
			have[c] = true
		}
		for c := range s.controls {
			if !have[c] {
				logger.Warnf("%s doesn't report control %s (it has %v); its actions won't run until it does",
					conf.InputController, c, available)
			}
		}
	}

	for _, w := range s.controls {
		s.wg.Add(1)
		go s.work(w)
	}
	triggers := []input.EventType{input.ButtonPress, input.ButtonRelease, input.Connect, input.Disconnect}
	for _, w := range s.controls {
		if err := controller.RegisterControlCallback(ctx, w.control, triggers, s.onEvent, nil); err != nil {
			logger.Warnf("can't listen to %s on %s: %v", w.control, conf.InputController, err)
		}
	}
	return s, nil
}

// onEvent queues the control's action. It never blocks the controller.
func (s *footswitchActions) onEvent(ctx context.Context, ev input.Event) {
	switch ev.Event {
	case input.Connect, input.Disconnect:
		s.mu.Lock()
		changed := s.connected != (ev.Event == input.Connect)
		s.connected = ev.Event == input.Connect
		s.mu.Unlock()
		if changed && ev.Event == input.Disconnect {
			s.logger.Warn("input controller disconnected; actions resume when it reconnects")
		} else if changed {
			s.logger.Info("input controller reconnected")
		}
		return
	}
	w := s.controls[ev.Control]
	if w == nil {
		return
	}
	var a *boundAction
	switch ev.Event {
	case input.ButtonPress:
		a = w.onPress
	case input.ButtonRelease:
		a = w.onRelease
	default:
		return
	}
	if a == nil || s.cancelCtx.Err() != nil {
		return
	}
	select {
	case w.queue <- queuedAction{control: w.control, event: ev.Event, action: a}:
	default:
		s.logger.Errorf("%s %s: dropped %s.%s; %d actions already queued", w.control, ev.Event, a.Component, a.Method, actionQueueSize)
	}
}

// work runs one control's actions in event order.
func (s *footswitchActions) work(w *controlWorker) {
	defer s.wg.Done()
	for {
		select {
		case <-s.cancelCtx.Done():
			return
		case q := <-w.queue:
			s.run(q)
		}
	}
}

func (s *footswitchActions) run(q queuedAction) {
	ctx, cancel := context.WithTimeout(s.cancelCtx, s.timeout)
	defer cancel()
	start := time.Now()
	_, err := q.action.target.DoCommand(ctx, q.action.args)
	took := time.Since(start).Round(time.Millisecond)
	switch {
	case err == nil:
		s.logger.Infof("%s %s: %s.%s ok in %s", q.control, q.event, q.action.Component, q.action.Method, took)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		s.logger.Errorf("%s %s: %s.%s timed out after %s and was cancelled", q.control, q.event, q.action.Component, q.action.Method, s.timeout)
	default:
		s.logger.Errorf("%s %s: %s.%s failed after %s: %v", q.control, q.event, q.action.Component, q.action.Method, took, err)
	}
}

func (s *footswitchActions) Name() resource.Name {
	return s.name
}

func (s *footswitchActions) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("the actions service has no commands; configure actions in its controls attribute")
}

func (s *footswitchActions) Status(ctx context.Context) (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queued := map[string]interface{}{}
	for c, w := range s.controls {
		queued[string(c)] = len(w.queue)
	}
	return map[string]interface{}{"controller_connected": s.connected, "queued": queued}, nil
}

func (s *footswitchActions) Close(ctx context.Context) error {
	for _, w := range s.controls {
		_ = s.controller.RegisterControlCallback(ctx, w.control,
			[]input.EventType{input.ButtonPress, input.ButtonRelease, input.Connect, input.Disconnect}, nil, nil)
	}
	s.cancelFunc()
	s.wg.Wait()
	return nil
}
