// Command cli opens the pedal directly and logs presses, for testing on a
// Linux machine without viam-server:
//
//	go run ./cmd/cli            # 3-pedal unit (pcsensor-3-pedal)
//	go run ./cmd/cli -pedals 1  # single-pedal unit (pcsensor-1-pedal)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"footswitch"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain() error {
	pedals := flag.Int("pedals", 3, "how many pedals the unit has: 1 or 3")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	logger := logging.NewDebugLogger("cli")
	name := input.Named("pedals")

	var thing input.Controller
	var err error
	switch *pedals {
	case 3:
		thing, err = footswitch.NewPcsensor3Pedal(ctx, resource.Dependencies{}, name, &footswitch.Config{}, logger)
	case 1:
		thing, err = footswitch.NewPcsensor1Pedal(ctx, resource.Dependencies{}, name, &footswitch.OnePedalConfig{}, logger)
	default:
		return fmt.Errorf("-pedals must be 1 or 3, got %d", *pedals)
	}
	if err != nil {
		return err
	}
	defer thing.Close(context.Background())

	list, err := thing.DoCommand(ctx, map[string]interface{}{"command": "list_pedals"})
	if err != nil {
		return err
	}
	logger.Infof("%v", list)
	<-ctx.Done()
	return nil
}
