package footswitch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sysPaths lets tests point device scanning at a fake tree.
type sysPaths struct {
	sys string // normally /sys
	dev string // normally /dev
}

var hostPaths = sysPaths{sys: "/sys", dev: "/dev"}

// inputDevice is one USB keyboard event device, with what we know about the
// USB device it belongs to.
type inputDevice struct {
	usbID      string // "3553:b001"
	eventPath  string // /dev/input/event5
	byPath     string // stable /dev/input/by-path/... link, or "" if udev made none
	port       string // USB port path, e.g. "1.2"
	usbDevice  string // sysfs dir of the USB device
	hidrawPath string // settings interface (interface 1), or "" if absent
}

// stablePath is the path to put in configs.
func (d inputDevice) stablePath() string {
	if d.byPath != "" {
		return d.byPath
	}
	return d.eventPath
}

func readSysString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// usbInterfaceDir walks up from a sysfs device to its USB interface dir.
func usbInterfaceDir(dir string) string {
	for d := dir; d != "/" && d != "." && d != ""; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "bInterfaceNumber")); err == nil {
			return d
		}
	}
	return ""
}

// evBits parses a sysfs capability bitmap (space-separated hex longs, most
// significant first) and reports whether bit n is set.
func evBit(bitmap string, n int) bool {
	words := strings.Fields(bitmap)
	idx := len(words) - 1 - n/64
	if idx < 0 || idx >= len(words) {
		return false
	}
	v, err := strconv.ParseUint(words[idx], 16, 64)
	if err != nil {
		return false
	}
	return v&(1<<(uint(n)%64)) != 0
}

const (
	evKey = 0x01
	evRel = 0x02
)

// scanKeyboards lists the keyboard event devices on interface 0 of every
// USB device. Pedals' mouse and settings event devices are left out.
func scanKeyboards(p sysPaths) ([]inputDevice, error) {
	events, err := filepath.Glob(filepath.Join(p.sys, "class/input/event*"))
	if err != nil {
		return nil, err
	}
	byPath := byPathLinks(p)
	hidraws := hidrawNodes(p)

	var out []inputDevice
	for _, ev := range events {
		inputDir, err := filepath.EvalSymlinks(filepath.Join(ev, "device"))
		if err != nil {
			continue
		}
		caps := readSysString(filepath.Join(inputDir, "capabilities/ev"))
		if !evBit(caps, evKey) || evBit(caps, evRel) {
			continue
		}
		iface := usbInterfaceDir(inputDir)
		if iface == "" || readSysString(filepath.Join(iface, "bInterfaceNumber")) != "00" {
			continue
		}
		vendor := strings.ToLower(readSysString(filepath.Join(inputDir, "id/vendor")))
		product := strings.ToLower(readSysString(filepath.Join(inputDir, "id/product")))
		if vendor == "" || product == "" {
			continue
		}
		usbDev := filepath.Dir(iface)
		eventPath := filepath.Join(p.dev, "input", filepath.Base(ev))
		out = append(out, inputDevice{
			usbID:      vendor + ":" + product,
			eventPath:  eventPath,
			byPath:     byPath[eventPath],
			port:       usbPort(filepath.Base(usbDev)),
			usbDevice:  usbDev,
			hidrawPath: hidraws[usbDev],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].eventPath < out[j].eventPath })
	return out, nil
}

// usbPort turns a sysfs USB device name like "1-6.2" into its port path "6.2".
func usbPort(name string) string {
	if i := strings.IndexByte(name, '-'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// byPathLinks maps /dev/input/eventN to its /dev/input/by-path keyboard link.
func byPathLinks(p sysPaths) map[string]string {
	out := map[string]string{}
	links, _ := filepath.Glob(filepath.Join(p.dev, "input/by-path/*"))
	for _, l := range links {
		if !strings.Contains(filepath.Base(l), "event") {
			continue
		}
		target, err := filepath.EvalSymlinks(l)
		if err != nil {
			continue
		}
		// Prefer the "-event-kbd" link if a device has several.
		if _, ok := out[target]; !ok || strings.HasSuffix(l, "-event-kbd") {
			out[target] = l
		}
	}
	return out
}

// hidrawNodes maps a USB device's sysfs dir to the hidraw node of its
// interface 1, the PCsensor settings interface.
func hidrawNodes(p sysPaths) map[string]string {
	out := map[string]string{}
	nodes, _ := filepath.Glob(filepath.Join(p.sys, "class/hidraw/hidraw*"))
	for _, n := range nodes {
		dir, err := filepath.EvalSymlinks(filepath.Join(n, "device"))
		if err != nil {
			continue
		}
		iface := usbInterfaceDir(dir)
		if iface == "" || readSysString(filepath.Join(iface, "bInterfaceNumber")) != "01" {
			continue
		}
		out[filepath.Dir(iface)] = filepath.Join(p.dev, filepath.Base(n))
	}
	return out
}

// findDevice picks the pedal to use. With devicePath set, that device is
// used even if its USB ID isn't a known PCsensor ID.
func findDevice(p sysPaths, devicePath, usbID string) (inputDevice, error) {
	devs, err := scanKeyboards(p)
	if err != nil {
		return inputDevice{}, err
	}
	if devicePath != "" {
		target, err := filepath.EvalSymlinks(devicePath)
		if err != nil {
			return inputDevice{}, fmt.Errorf("device_path %s: %w", devicePath, err)
		}
		for _, d := range devs {
			if d.eventPath == target {
				d.byPath = devicePath
				return d, nil
			}
		}
		// Not a USB keyboard we recognize; read it as a bare event device.
		return inputDevice{eventPath: target, byPath: devicePath}, nil
	}
	for _, d := range devs {
		if usbID != "" {
			if strings.EqualFold(d.usbID, usbID) {
				return d, nil
			}
		} else if isPCsensorUSBID(d.usbID) {
			return d, nil
		}
	}
	want := "any of " + strings.Join(pcsensorUSBIDs, ", ")
	if usbID != "" {
		want = usbID
	}
	return inputDevice{}, fmt.Errorf("%w: no pedal with USB ID %s is plugged in", errNotFound, want)
}

// findPedals lists every plugged-in pedal with a supported USB ID.
func findPedals(p sysPaths) ([]inputDevice, error) {
	devs, err := scanKeyboards(p)
	if err != nil {
		return nil, err
	}
	var out []inputDevice
	for _, d := range devs {
		if isPCsensorUSBID(d.usbID) {
			out = append(out, d)
		}
	}
	return out, nil
}
