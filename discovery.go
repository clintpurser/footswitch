package footswitch

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// DiscoverResources suggests a pcsensor-3-pedal component for each attached
// pedal, including pedals already in use.
func (d *footswitchDiscovery) DiscoverResources(ctx context.Context, extra map[string]any) ([]resource.Config, error) {
	devs, err := findPedals(d.paths)
	if err != nil {
		return nil, err
	}
	return suggestConfigs(devs), nil
}

func suggestConfigs(devs []inputDevice) []resource.Config {
	out := make([]resource.Config, 0, len(devs))
	for _, dev := range devs {
		name := fmt.Sprintf("footswitch-%s-%s", strings.ReplaceAll(dev.usbID, ":", "-"), dev.port)
		out = append(out, resource.Config{
			Name:       name,
			API:        input.API,
			Model:      Pcsensor3Pedal,
			Attributes: utils.AttributeMap{"device_path": dev.stablePath()},
		})
	}
	return out
}

func (d *footswitchDiscovery) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("not supported; use DiscoverResources")
}

func (d *footswitchDiscovery) Status(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
