package footswitch

import (
	"context"
	"errors"
	"fmt"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/discovery"
	"go.viam.com/rdk/utils"
)

var Discovery = resource.NewModel("clint", "footswitch", "discovery")

func init() {
	resource.RegisterService(discovery.API, Discovery,
		resource.Registration[discovery.Service, resource.NoNativeConfig]{
			Constructor: newDiscovery,
		},
	)
}

type footswitchDiscovery struct {
	resource.Named
	resource.AlwaysRebuild
	resource.TriviallyCloseable

	logger logging.Logger
	paths  sysPaths
}

func newDiscovery(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (discovery.Service, error) {
	return &footswitchDiscovery{Named: conf.ResourceName().AsNamed(), logger: logger, paths: hostPaths}, nil
}

// DiscoverResources suggests a pcsensor-3-pedal and a pcsensor-1-pedal
// component for each attached pedal, including pedals already in use. Both
// are suggested because the device cannot report how many pedals it has.
func (d *footswitchDiscovery) DiscoverResources(ctx context.Context, extra map[string]any) ([]resource.Config, error) {
	devs, err := findPedals(d.paths)
	if err != nil {
		return nil, err
	}
	return suggestConfigs(devs), nil
}

// suggestConfigs emits two configs per device, 3-pedal first; the user adds
// the one that matches their hardware. Both are named "footswitch" for the
// first device and "footswitch-1", "footswitch-2" and so on for the others,
// the same scheme the app uses when it renames a duplicate.
//
// device_path is set only when several pedals are plugged in. With one pedal
// the component finds it by USB ID, so the config stays {} and keeps working
// if the pedal moves to another USB port.
func suggestConfigs(devs []inputDevice) []resource.Config {
	out := make([]resource.Config, 0, 2*len(devs))
	for i, dev := range devs {
		name := "footswitch"
		if i > 0 {
			name = fmt.Sprintf("footswitch-%d", i)
		}
		for _, model := range []resource.Model{Pcsensor3Pedal, Pcsensor1Pedal} {
			attrs := utils.AttributeMap{}
			if len(devs) > 1 {
				attrs["device_path"] = dev.stablePath()
			}
			out = append(out, resource.Config{Name: name, API: input.API, Model: model, Attributes: attrs})
		}
	}
	return out
}

func (d *footswitchDiscovery) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("not supported; use DiscoverResources")
}

func (d *footswitchDiscovery) Status(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
