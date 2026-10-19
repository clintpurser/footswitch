# Footswitch module spec: PCsensor USB foot pedals

Oct 18, 2026 · @Clint Purser

## Overview

One Viam module, `footswitch`, exposes PCsensor-family 3-pedal USB foot pedals as input controllers. A separate actions service calls a method on another component when a control is pressed or released, in the same style as the streamdeck module. v1 ships one input controller model, a discovery service and the actions service.

| Field | Value |
| --- | --- |
| Module | `footswitch` |
| Input controller model | `<namespace>:footswitch:pcsensor-3-pedal` (API `rdk:component:input_controller`) |
| Actions service | `<namespace>:footswitch:actions` (API `rdk:service:generic`) |
| Discovery service | `<namespace>:footswitch:discovery` (API `rdk:service:discovery`) |
| Supported USB IDs | `1a86:e026`, `3553:b001`, `0c45:7403`, `0c45:7404`, `413d:2107` |
| Platforms | Linux only (evdev); builds for linux/amd64 and linux/arm64 |
| Registry keywords | iKKEGOL, PCsensor, USB foot pedal, footswitch |
| Settings protocol reference | [rgerganov/footswitch](https://github.com/rgerganov/footswitch) |

## Models

The input controller model is named for the protocol and the pedal count, not the brand. The same PCsensor hardware is sold as iKKEGOL, PCsensor and unbranded, so the brands go in the registry keywords instead. The pedal count is in the name because it decides the pedal names in the config, and `Validate` must know it without the device. The model is a pure hardware driver: it reports presses and releases and never calls other components.

|  | `pcsensor-3-pedal` |
| --- | --- |
| Pedal names | `left`, `center`, `right` |
| Default controls | `ButtonLT`, `ButtonSelect`, `ButtonRT` |
| Finds the device | Automatically by USB ID, or `device_path` |
| Knows which key each pedal sends | Yes, reads the device |
| Can program the pedal's keys | Yes |

1- and 2-pedal models are deferred until there's hardware to test; see Later.

## Architecture

The module follows Viam's split of components for hardware and services for behavior. The pedal component stays small and testable, and the actions service works with any input controller, gamepads included.

| Layer | Responsibility |
| --- | --- |
| Input controller core | evdev reading and grab, key-to-pedal map, debounce, press/release state, input controller API, hotplug |
| Driver interface | Match a USB ID, read and write each slot's stored key (internal code seam, not configured by users) |
| PCsensor driver | Implements the interface over the settings HID interface |
| Input controller model | Thin wrapper that translates pedal names to slots |
| Actions service | Subscribes to any input controller's events and runs the configured DoCommands; knows nothing about pedals |

The device sits behind an interface so the core can be unit-tested with a fake event stream; the actions service can be tested against Viam's fake input controller.

## Configuration

A typical machine has one input controller for the pedal and, optionally, one actions service pointing at it. Field names use snake\_case to match the streamdeck module's `component` / `method` / `args` action shape.

### Input controller

Every attribute is optional: with an empty config, the component finds the pedal, reads which key each pedal sends, and reports the pedals as buttons.

```json
{
  "name": "pedals",
  "api": "rdk:component:input_controller",
  "model": "<namespace>:footswitch:pcsensor-3-pedal",
  "attributes": {
    "left":   { "keycode": "KEY_F13" },
    "center": {},
    "right":  { "keycode": "KEY_F15", "control": "ButtonRT" }
  }
}
```

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `left`, `center`, `right` | pedal block | all defaults | One block per pedal. |
| `device_path` | string | auto-detect | A `/dev/input/by-path/…` path. Picks one device when several pedals are plugged in; the discovery service fills it in. |
| `usb_id` | string `vvvv:pppp` | any supported ID | Restricts detection to one USB ID. |
| `debounce_ms` | integer | 25 | Changes on the same pedal closer together than this are ignored. Keep below about 30; real taps measured 36 ms. |

**Pedal block**

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `keycode` | string, evdev name (e.g. `KEY_F13`) | none | If set, written to the pedal's slot on startup and reconfigure. If omitted, the key stored on the pedal is read and left unchanged. Also the escape hatch for clones whose settings can't be read. |
| `control` | string, Viam control name | per model, see Models | Overrides the control the pedal reports as. |

An omitted pedal block keeps the pedal's stored key and default control.

### Actions service

```json
{
  "name": "pedal-actions",
  "api": "rdk:service:generic",
  "model": "<namespace>:footswitch:actions",
  "attributes": {
    "input_controller": "pedals",
    "controls": {
      "ButtonLT":     { "on_press": { "component": "arm", "method": "do_command", "args": { "cmd": "stop" } } },
      "ButtonSelect": { "on_press":   { "component": "gripper", "method": "do_command", "args": { "cmd": "close" } },
                        "on_release": { "component": "gripper", "method": "do_command", "args": { "cmd": "open" } } }
    }
  }
}
```

| Attribute | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `input_controller` | string | Yes | — | Name of the input controller to listen to. Any model works, not only the pedals. |
| `controls` | map of control name to control block | No | `{}` | Actions per control, keyed by Viam control name (e.g. `ButtonLT`). |
| `action_timeout_ms` | integer | No | 5000 | An action that runs longer is cancelled and logged. |

**Control block**

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `on_press` | action or `null` | `null` | Called on `ButtonPress`. |
| `on_release` | action or `null` | `null` | Called on `ButtonRelease`. |

**Action**

| Attribute | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `component` | string | Yes | — | Name of another resource on the machine. Declared as a dependency. |
| `method` | string | Yes | — | `do_command` only in v1. |
| `args` | object | No | `{}` | Passed as the DoCommand payload. |

## Behavior

Pedals are identified by their slot on the device, never by key alone: slot 1 is left, slot 2 center and slot 3 right.

### Startup and reconfigure

1. Find the device: by `device_path` if set, otherwise by `usb_id` or the supported USB ID list. Open its evdev keyboard device.
2. Grab it with `EVIOCGRAB`, so its keys reach only the module and never type into other programs.
3. Read each slot's stored setting through the settings HID interface.
4. For each pedal with a `keycode`, write that key to its slot if it differs from the stored one. Skipping identical writes avoids needless flash writes. Talk to the settings interface (USB interface 1) through its hidraw node, not through libusb, so the kernel driver is never detached; writes don't re-enumerate the keyboard interface.
5. If a slot without a configured `keycode` is in mouse or macro mode, fail with a message naming the pedal and telling the user to set a `keycode`.
6. Build the key-to-pedal map from the slot settings. Fail if two pedals share a key.
7. Emit `Connect` and start reading events.

### Pedal events

- Key down (evdev value 1): mark the pedal pressed and emit `ButtonPress`.
- Key up (evdev value 0): mark the pedal released and emit `ButtonRelease`.
- Autorepeat (evdev value 2): ignored. It comes from the kernel, not the pedal.
- Changes closer together than `debounce_ms` on the same pedal are ignored, to filter contact bounce and phantom presses.
- Keys the map doesn't know are logged once per key and otherwise ignored.
- Several pedals can be held at once; each is tracked separately.

### Logging

Every press and release is logged at info level to the machine's logs, so pedal activity shows up in the app's Logs tab without extra config. Each line names the pedal, its slot and key; a release also gives how long the pedal was held.

```text
INFO  footswitch/pedals  pedal left pressed (slot 1, KEY_F13)
INFO  footswitch/pedals  pedal left released after 1.42s (slot 1, KEY_F13)
```

| Event | Level |
| --- | --- |
| Pedal pressed or released | info |
| Release synthesized on disconnect or close | info, marked `(device disconnected)` or `(closing)` |
| Change ignored by debounce, autorepeat | debug |
| Unknown key from the device | warn, once per key |
| Device disconnected or reconnected | warn on disconnect, info on reconnect |

Action logging belongs to the actions service; see below.

### Actions service

- On startup and reconfigure, resolve the input controller and every action's `component` from the dependencies, then register callbacks for `ButtonPress` and `ButtonRelease` on each configured control. Warn about configured controls the controller doesn't report.
- `ButtonPress` queues `on_press`; `ButtonRelease` queues `on_release`. Releases the pedal synthesizes on disconnect or close arrive as ordinary `ButtonRelease` events, so hold-to-run actions still stop.
- Each control has its own queue and worker, so a slow component never delays another control's actions.
- Actions for one control run in event order, so a release never overtakes its press.
- An action that runs longer than `action_timeout_ms` is cancelled.
- A control's queue holds 16 actions; when full, new actions are dropped.
- Every action is logged at info with the control, event, component and duration. Failures, timeouts and drops are logged at error. Nothing is retried.
- A `Disconnect` from the controller is logged at warn; the service keeps its callbacks and resumes when the controller reconnects.

### Unplug, replug and close

- When the device disappears, every held pedal is released as if lifted: the component emits `ButtonRelease` for it, so a listening actions service runs `on_release`. It then emits `Disconnect` and polls for the device every second.
- When the device returns, the component repeats the startup steps, then emits `Connect`.
- On close, the component releases held pedals the same way, then releases the grab. It leaves the pedal's stored keys as they are, so the pedal keeps working as a plain keyboard.

### Safety

A USB keyboard pedal can drop events, and the module can crash. The docs state that this module is a convenience control and does not replace a hardware e-stop.

## Input controller API

Other modules can subscribe to the pedals through the standard input controller API, the same way the actions service listens.

Default controls are listed under Models; a pedal's `control` attribute overrides them.

| Method | Behavior |
| --- | --- |
| `Controls()` | Returns one control per pedal. |
| `Events()` | Returns the latest event for each control, with its timestamp. |
| `RegisterControlCallback()` | Calls back on `ButtonPress`, `ButtonRelease`, `Connect` and `Disconnect`. |
| `TriggerEvent()` | Not supported. |
| `DoCommand()` | `{"command": "list_pedals"}` lists every pedal; see below. |

Hold duration is the time between a pedal's `ButtonPress` and `ButtonRelease`. A synthesized `ButtonHold` event is out of scope for v1.

### DoCommand: `list_pedals`

Returns every pedal with its names, keys, slots and live state, so apps and other modules can discover the pedals without knowing the model or its config. `Controls()` only returns control names.

```json
{
  "model": "pcsensor-3-pedal",
  "device": { "usb_id": "3553:b001", "path": "/dev/input/by-path/pci-0000:00:14.0-usb-0:6:1.0-event-kbd", "connected": true },
  "pedals": [
    { "name": "left",   "slot": 1, "control": "ButtonLT",     "keycode": "KEY_F13", "keycode_source": "config", "mode": "key", "pressed": false },
    { "name": "center", "slot": 2, "control": "ButtonSelect", "keycode": "KEY_F14", "keycode_source": "device", "mode": "key", "pressed": true },
    { "name": "right",  "slot": 3, "control": "ButtonRT",     "keycode": "KEY_F15", "keycode_source": "config", "mode": "key", "pressed": false }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `name` | The pedal's name in the config: `left` / `center` / `right`. |
| `slot` | Slot number on the device. |
| `control` | The Viam control the pedal reports as. |
| `keycode` | The key the pedal sends. |
| `keycode_source` | `config` if set in the config, `device` if read from the pedal. |
| `mode` | `key`, `mouse` or `macro`, as read from the device. |
| `pressed` | Whether the pedal is down right now. |

When the device is disconnected, `connected` is `false` and the pedals are listed from the last successful read, with `pressed` false. Unknown commands return an error naming the supported ones.

## Discovery service

The module's discovery service finds attached PCsensor pedals and returns a component config for each, following the [discovery service](https://docs.viam.com/reference/services/discovery) pattern used by the SO-101 and RealSense modules. The user adds the service, opens its Test panel, and adds a pedal from the suggested config instead of typing USB IDs or device paths.

| Field | Value |
| --- | --- |
| API | `rdk:service:discovery` |
| Model | `<namespace>:footswitch:discovery` |
| Attributes | None |
| Method | `DiscoverResources` returns suggested component configs for each device found |

For each device with a supported USB ID, it suggests a `pcsensor-3-pedal` component with `device_path` set and no pedal blocks; the keys are read from the device when it starts.

Each suggested component is named `footswitch-<usb_id>-<port>`, for example `footswitch-1a86-e026-1.2`, so two identical pedals get different names. Devices already grabbed by a configured footswitch component are still listed.

Discovery suggests only input controllers. Users add the actions service by hand, since what each pedal should do isn't something the hardware can tell.

## Validation

Each `Validate` rejects the config with a message naming the field.

**Input controller** rejects the config when:

- a top-level key isn't `left`, `center`, `right`, `device_path`, `usb_id` or `debounce_ms`;
- a `keycode` isn't a known evdev key name, or has no USB HID equivalent the pedal can store;
- a `control` isn't a Viam control name, or two pedals use the same control;
- two configured `keycode` values are the same (a clash with a key read from the device is caught at startup instead);
- `usb_id` isn't in `vvvv:pppp` hex form, or `debounce_ms` isn't a positive integer.

It declares no dependencies.

**Actions service** rejects the config when:

- `input_controller` is missing;
- a key in `controls` isn't a Viam control name;
- an action is missing `component` or `method`, `method` isn't `do_command`, or `args` is present but not an object;
- `action_timeout_ms` isn't a positive integer.

It declares `input_controller` and the `component` of every action as required dependencies.

## Hardware findings

Tested on an iKKEGOL 3-pedal unit (`3553:b001`, "PCsensor FootSwitch") on x86\_64 Pop!\_OS 24.04 with `footswitch`, `evtest` and `dmesg`.

| Question | Finding |
| --- | --- |
| USB interfaces | Interface 0 is the keyboard, which creates a keyboard event device (used by the module) and a mouse event device (used by pedals in mouse mode). Interface 1 is the settings interface; it also creates an event device, which the module ignores. |
| Serial number | None, so identical pedals can only be told apart by USB port (`/dev/input/by-path/`). |
| Factory keys | Slots 1–3 send a, b and c. |
| Writing keys | F13–F15 accepted and read back correctly. |
| Slot to position | Slot 1 is left (F13), slot 2 center (F14), slot 3 right (F15). |
| Press and release | Separate key-down and key-up events; held pedals stay down until lifted. Autorepeat (value 2) comes from the kernel after 250 ms. |
| Simultaneous pedals | Each pedal reports its own down and up, a few ms apart. |
| Fastest taps | 36–52 ms down; no bounce seen. Keep `debounce_ms` below about 30. |
| Scan codes | `MSC_SCAN` carries the HID usage (`0x70068`–`0x7006a` for F13–F15). |
| Re-enumeration after a write | The keyboard interface stays put. Only the settings interface rebinds, and only because the `footswitch` tool uses libusb. |
| Unplug and replug | New `eventN` and USB device number; the `/dev/input/by-path/` link stays the same. |

## Later: 1- and 2-pedal units

Deferred until there's hardware to test, because the implementation isn't known yet. Some 1-pedal units use the 3-pedal protocol on slot 2; others (`5131:2019`) use a separate protocol on interface 3, handled by the `footswitch1p` tool, and would need their own driver. Discovery would also need a way to tell pedal counts apart, such as absent slots reading as unconfigured (mode 0), the USB product string, or the HID descriptor. Adding `pcsensor-1-pedal` or `pcsensor-2-pedal` later is a new model and driver; it doesn't change `pcsensor-3-pedal` configs.

## Later: other pedals

Non-PCsensor pedals are out of scope for v1. When someone needs one, add a `generic` driver behind the existing driver interface: it reads keys through the same core but can't read or program the device, so its config needs `device_path` or `usb_id` and a `keycode` and `control` per pedal. It would ship as a new model plus an opt-in flag on the discovery service, without changing the PCsensor models' configs.
