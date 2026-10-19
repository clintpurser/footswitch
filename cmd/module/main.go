package main

import (
	"footswitch"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/discovery"
	generic "go.viam.com/rdk/services/generic"
)

func main() {
	module.ModularMain(
		resource.APIModel{API: input.API, Model: footswitch.Pcsensor3Pedal},
		resource.APIModel{API: generic.API, Model: footswitch.Actions},
		resource.APIModel{API: discovery.API, Model: footswitch.Discovery},
	)
}
