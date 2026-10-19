//go:build !linux

package footswitch

import (
	"errors"
	"os"
)

func grab(*os.File, bool) error {
	return errors.New("footswitch pedals are supported on Linux only")
}
