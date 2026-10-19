//go:build linux

package footswitch

import (
	"os"

	"golang.org/x/sys/unix"
)

// eviocgrab is _IOW('E', 0x90, int).
const eviocgrab = 0x40044590

// grab takes or releases exclusive access to an evdev device, so the
// pedal's keys reach only this module and never type into other programs.
func grab(f *os.File, on bool) error {
	arg := 0
	if on {
		arg = 1
	}
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ioctlErr error
	if err := conn.Control(func(fd uintptr) {
		ioctlErr = unix.IoctlSetInt(int(fd), eviocgrab, arg)
	}); err != nil {
		return err
	}
	return ioctlErr
}
