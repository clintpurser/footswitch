// Command cli opens the pedal directly and logs presses, for testing on a
// Linux machine without viam-server: go run ./cmd/cli
package main

import (
	"context"
	"os"
	"os/signal"

	"footswitch"

	input "go.viam.com/rdk/components/input"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

func main() {
	if err := realMain(); err != nil {
		panic(err)
	}
}

func realMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	logger := logging.NewDebugLogger("cli")

	thing, err := footswitch.NewPcsensor3Pedal(ctx, resource.Dependencies{}, input.Named("pedals"), &footswitch.Config{}, logger)
	if err != nil {
		return err
	}
	defer thing.Close(context.Background())

	pedals, err := thing.DoCommand(ctx, map[string]interface{}{"command": "list_pedals"})
	if err != nil {
		return err
	}
	logger.Infof("%v", pedals)
	<-ctx.Done()
	return nil
}
