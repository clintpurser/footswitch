package footswitch

import (
	"os"
	"path/filepath"
	"testing"

	"go.viam.com/rdk/resource"
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

	// One pedal: both models are suggested as "footswitch" with no
	// device_path, so the config is {} and survives a USB port change.
	cfgs := suggestConfigs(devs)
	if len(cfgs) != 2 || cfgs[0].Model != Pcsensor3Pedal || cfgs[1].Model != Pcsensor1Pedal {
		t.Fatalf("suggested %+v", cfgs)
	}
	for _, c := range cfgs {
		if c.Name != "footswitch" || len(c.Attributes) != 0 {
			t.Fatalf("suggested %+v", c)
		}
	}

}

func TestSuggestConfigsSeveralPedals(t *testing.T) {
	devs := []inputDevice{
		{usbID: "3553:b001", eventPath: "/dev/input/event5", byPath: "/dev/input/by-path/a-event-kbd"},
		{usbID: "3553:b001", eventPath: "/dev/input/event9", byPath: "/dev/input/by-path/b-event-kbd"},
		{usbID: "3553:b001", eventPath: "/dev/input/event12"},
	}
	cfgs := suggestConfigs(devs)
	want := []struct {
		name, path string
		model      resource.Model
	}{
		{"footswitch", "/dev/input/by-path/a-event-kbd", Pcsensor3Pedal},
		{"footswitch", "/dev/input/by-path/a-event-kbd", Pcsensor1Pedal},
		{"footswitch-1", "/dev/input/by-path/b-event-kbd", Pcsensor3Pedal},
		{"footswitch-1", "/dev/input/by-path/b-event-kbd", Pcsensor1Pedal},
		{"footswitch-2", "/dev/input/event12", Pcsensor3Pedal},
		{"footswitch-2", "/dev/input/event12", Pcsensor1Pedal},
	}
	if len(cfgs) != len(want) {
		t.Fatalf("suggested %d configs, want %d", len(cfgs), len(want))
	}
	for i, w := range want {
		c := cfgs[i]
		if c.Name != w.name || c.Model != w.model || c.Attributes["device_path"] != w.path {
			t.Errorf("config %d = %s %s %v, want %s %s %s", i, c.Name, c.Model, c.Attributes, w.name, w.model, w.path)
		}
	}
}
