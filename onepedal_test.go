package footswitch

import (
	"testing"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/resource"
)

// The registration is the one line that, if forgotten, fails only at
// runtime inside viam-server.
func TestPedalModelsRegistered(t *testing.T) {
	for _, m := range []resource.Model{Pcsensor3Pedal, Pcsensor1Pedal} {
		reg, ok := resource.LookupRegistration(input.API, m)
		if !ok {
			t.Fatalf("%s is not registered", m)
		}
		if reg.Constructor == nil || reg.AttributeMapConverter == nil {
			t.Fatalf("%s is registered without a constructor or converter", m)
		}
	}
}
