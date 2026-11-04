# footswitch Module

The `clint:footswitch` module turns PCsensor-family USB foot pedals (sold as iKKEGOL, PCsensor and unbranded) into Viam input controllers, and runs `do_command` on other resources when a pedal is pressed or released:

1. **`clint:footswitch:pcsensor-1-pedal`**: an input controller for a single-pedal unit.
2. **`clint:footswitch:pcsensor-3-pedal`**: an input controller for a 3-pedal unit.
3. **`clint:footswitch:actions`**: a generic service that runs `do_command` on press and release.
4. **`clint:footswitch:discovery`**: a discovery service that suggests a component for each attached pedal.

Supported USB IDs: `3553:b001` (tested), `1a86:e026`, `0c45:7403`, `0c45:7404`, `413d:2107`. Linux only (`linux/amd64`, `linux/arm64`). `viam-server` needs access to `/dev/input/event*` and `/dev/hidraw*`, which it has as root (the default).

> **Safety:** a USB pedal can drop events and the module can crash. Use it as a convenience control, not as an e-stop.

---

## Example: hold the pedal to run teleop

```json
{
  "components": [
    { "name": "footswitch", "api": "rdk:component:input_controller", "model": "clint:footswitch:pcsensor-1-pedal", "attributes": {} }
  ],
  "services": [
    {
      "name": "actions",
      "api": "rdk:service:generic",
      "model": "clint:footswitch:actions",
      "attributes": {
        "input_controller": "footswitch",
        "controls": {
          "pedal": {
            "on_press":   { "component": "teleop-lf", "method": "do_command", "args": { "command": "start" } },
            "on_release": { "component": "teleop-lf", "method": "do_command", "args": { "command": "stop" } }
          }
        }
      }
    }
  ]
}
```

---

## Model: `clint:footswitch:pcsensor-1-pedal`

**API:** `rdk:component:input_controller`

A single-pedal unit, reported as one button. The component finds the pedal by USB ID and grabs it, so it never types into other programs. It then reads the pedal's stored key, or programs one.

Over USB a single-pedal unit is identical to a 3-pedal unit, so pick the model that matches the pedals you can see.

### Configuration

```json
{
  "pedal": { "keycode": "KEY_F14" }
}
```

| Name          | Type        | Required | Description                                                                                         |
| ------------- | ----------- | -------- | --------------------------------------------------------------------------------------------------- |
| `pedal`       | pedal block | No       | The pedal. See [Pedal block](#pedal-block).                                                          |
| `slot`        | int         | No       | Firmware slot the switch is wired to: 1, 2 or 3. Defaults to 2. Change it only if the pedal never fires. |
| `device_path` | string      | No       | `/dev/input/by-path/…` link. Needed only when several pedals are plugged in.                        |
| `usb_id`      | string      | No       | Only use a pedal with this `vvvv:pppp` USB ID.                                                       |
| `debounce_ms` | int         | No       | Ignore changes closer together than this. Defaults to 25. Keep it below about 30.                   |

In `actions`, this pedal is named `pedal`.

---

## Model: `clint:footswitch:pcsensor-3-pedal`

**API:** `rdk:component:input_controller`

A 3-pedal unit, reported as three buttons. It works like `pcsensor-1-pedal`, with one block per pedal.

### Configuration

```json
{
  "left":   { "keycode": "KEY_F13" },
  "center": {},
  "right":  { "keycode": "KEY_F15" }
}
```

| Name                      | Type        | Required | Description                                                            |
| ------------------------- | ----------- | -------- | ---------------------------------------------------------------------- |
| `left`, `center`, `right` | pedal block | No       | One block per pedal (slots 1, 2, 3). See [Pedal block](#pedal-block).   |
| `device_path`             | string      | No       | `/dev/input/by-path/…` link. Needed only when several pedals are plugged in. |
| `usb_id`                  | string      | No       | Only use a pedal with this `vvvv:pppp` USB ID.                          |
| `debounce_ms`             | int         | No       | Ignore changes closer together than this. Defaults to 25.              |

In `actions`, these pedals are named `left`, `center` and `right`.

### Pedal block

| Name      | Type   | Required | Description                                                                                                                  |
| --------- | ------ | -------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `keycode` | string | No       | Evdev key name, for example `KEY_F13`. Written to the pedal on startup if it differs. If omitted, the stored key is read and kept. Required if the pedal is in mouse or macro mode. |
| `control` | string | No       | Viam button control the pedal reports as. Defaults to `ButtonSouth` (1-pedal), or `ButtonLT`, `ButtonSelect` and `ButtonRT` (3-pedal). |

Both models emit `ButtonPress`, `ButtonRelease`, `Connect` and `Disconnect`. A held pedal is released when it is unplugged or the component closes. An unplugged pedal reconnects automatically.

### DoCommand

`list_pedals` returns each pedal with its live state:

```json
{ "command": "list_pedals" }
```

```json
{
  "model": "pcsensor-1-pedal",
  "device": { "usb_id": "3553:b001", "path": "/dev/input/event5", "connected": true },
  "pedals": [
    { "name": "pedal", "slot": 2, "control": "ButtonSouth", "keycode": "KEY_B", "keycode_source": "device", "mode": "key", "pressed": false }
  ]
}
```

---

## Model: `clint:footswitch:actions`

**API:** `rdk:service:generic`

Calls `do_command` on another resource when a pedal is pressed or released. It works with any input controller, including gamepads.

### Configuration

```json
{
  "input_controller": "footswitch",
  "controls": {
    "pedal": {
      "on_press":   { "component": "<resource>", "method": "do_command", "args": { "command": "start" } },
      "on_release": { "component": "<resource>", "method": "do_command", "args": { "command": "stop" } }
    }
  }
}
```

| Name                | Type   | Required | Description                                                                                                  |
| ------------------- | ------ | -------- | ------------------------------------------------------------------------------------------------------------ |
| `input_controller`  | string | Yes      | Name of the input controller to listen to.                                                                   |
| `controls`          | object | No       | Actions per pedal. Keys are pedal names (`pedal`, or `left`, `center`, `right`) or Viam control names such as `ButtonSouth`. Use control names for gamepads. |
| `action_timeout_ms` | int    | No       | Cancel an action that runs longer than this. Defaults to 5000.                                               |

Each control takes `on_press` and `on_release`. Each action has `component` (any resource on the machine), `method` (`do_command` only) and optional `args` (the DoCommand payload).

Each control has its own queue of 16 actions, so a release never overtakes its press. When a queue is full, new actions are dropped. Every action is logged, and nothing is retried.

---

## Model: `clint:footswitch:discovery`

**API:** `rdk:service:discovery`

Suggests components for the pedals attached to the machine. It takes no attributes: `{}`.

Each pedal gets two suggestions, one for each pedal model, because the device can't report its pedal count. Add the one that matches your pedal. Both are named `footswitch`, and later pedals get `footswitch-1`, `footswitch-2` and so on. `device_path` is set only when more than one pedal is plugged in.

---

## Development

```sh
make test                    # unit tests, any OS
make module.tar.gz           # build; set VIAM_BUILD_OS/VIAM_BUILD_ARCH to cross-compile
go run ./cmd/cli -pedals 1   # Linux: log presses without viam-server (default -pedals 3)
```

Settings protocol reference: [rgerganov/footswitch](https://github.com/rgerganov/footswitch).
