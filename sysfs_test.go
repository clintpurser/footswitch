package footswitch

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeTree builds a sysfs and /dev tree for one iKKEGOL pedal on port 6:
// a keyboard and a mouse on interface 0, and the settings interface 1.
func fakeTree(t *testing.T) sysPaths {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := sysPaths{sys: filepath.Join(root, "sys"), dev: filepath.Join(root, "dev")}
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	usb := filepath.Join(p.sys, "devices/pci0000:00/0000:00:14.0/usb1/1-6")
	inputDev := func(iface, hid, input, event, caps string) {
		dir := filepath.Join(usb, iface, hid, "input", input)
		write(filepath.Join(dir, "capabilities/ev"), caps)
		write(filepath.Join(dir, "id/vendor"), "3553")
		write(filepath.Join(dir, "id/product"), "b001")
		link(dir, filepath.Join(p.sys, "class/input", event, "device"))
		write(filepath.Join(p.dev, "input", event), "")
	}
	write(filepath.Join(usb, "1-6:1.0/bInterfaceNumber"), "00")
	write(filepath.Join(usb, "1-6:1.1/bInterfaceNumber"), "01")
	inputDev("1-6:1.0", "0003:3553:B001.0001", "input5", "event5", "120013")
	inputDev("1-6:1.0", "0003:3553:B001.0001", "input6", "event6", "17")
	inputDev("1-6:1.1", "0003:3553:B001.0002", "input7", "event7", "120013")
	link(filepath.Join(usb, "1-6:1.1/0003:3553:B001.0002"), filepath.Join(p.sys, "class/hidraw/hidraw3/device"))
	link(filepath.Join(p.dev, "input/event5"), filepath.Join(p.dev, "input/by-path/pci-0000:00:14.0-usb-0:6:1.0-event-kbd"))
	link(filepath.Join(p.dev, "input/event6"), filepath.Join(p.dev, "input/by-path/pci-0000:00:14.0-usb-0:6:1.0-event-mouse"))
	return p
}

func TestScanFindsKeyboardInterface(t *testing.T) {
	p := fakeTree(t)
	devs, err := findPedals(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("found %d devices: %+v", len(devs), devs)
	}
	d := devs[0]
	if d.usbID != "3553:b001" || filepath.Base(d.eventPath) != "event5" || d.port != "6" ||
		filepath.Base(d.hidrawPath) != "hidraw3" || filepath.Base(d.byPath) != "pci-0000:00:14.0-usb-0:6:1.0-event-kbd" {
		t.Fatalf("device = %+v", d)
	}

	if _, err := findDevice(p, "", "1a86:e026"); err == nil {
		t.Fatal("expected not found for another usb_id")
	}
	got, err := findDevice(p, d.byPath, "")
	if err != nil || got.eventPath != d.eventPath {
		t.Fatalf("by device_path: %+v, %v", got, err)
	}

	cfgs := suggestConfigs(devs)
	if cfgs[0].Name != "footswitch-3553-b001-6" || cfgs[0].Attributes["device_path"] != d.byPath || cfgs[0].Model != Pcsensor3Pedal {
		t.Fatalf("suggested %+v", cfgs[0])
	}
}
