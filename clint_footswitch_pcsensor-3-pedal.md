# Model `clint:footswitch:pcsensor-3-pedal`

An input controller for PCsensor-family 3-pedal USB foot pedals. It finds the pedal, grabs it so its keys never type into other programs, reads (or programs) which key each pedal sends, and reports each pedal as a button.

## Supported hardware

PCsensor-protocol 3-pedal USB foot pedals, sold as iKKEGOL, PCsensor and unbranded. They show up as "PCsensor FootSwitch" in `lsusb`.

| USB ID | Status |
| --- | --- |
| `3553:b001` | Tested: iKKEGOL 3-pedal, x86_64 Pop!_OS 24.04 |
| `1a86:e026` | Same protocol per [rgerganov/footswitch](https://github.com/rgerganov/footswitch); untested |
| `0c45:7403` | Same protocol; untested |
| `0c45:7404` | Same protocol; untested |
| `413d:2107` | Same protocol; untested |

Not supported yet: 1- and 2-pedal units (including `5131:2019`), and non-PCsensor pedals.

Runs on Linux only (`linux/amd64`, `linux/arm64`), such as a Raspberry Pi, Jetson or x86 PC. `viam-server` needs access to `/dev/input/event*` and `/dev/hidraw*`, which it has when running as root (the default).

## Configuration

Every attribute is optional. With `{}`, the component finds the pedal, reads which key each pedal sends and reports them as `ButtonLT`, `ButtonSelect` and `ButtonRT`.

```json
{
  "left":   { "keycode": "KEY_F13" },
  "center": {},
  "right":  { "keycode": "KEY_F15", "control": "ButtonRT" }
}
```

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `left`, `center`, `right` | pedal block | all defaults | One block per pedal (slots 1, 2, 3). |
| `device_path` | string | auto-detect | A `/dev/input/by-path/…` path. Picks one device when several pedals are plugged in. |
| `usb_id` | string `vvvv:pppp` | any supported ID | Restricts detection to one USB ID. |
| `debounce_ms` | integer | 25 | Changes on the same pedal closer together than this are filtered. Keep below about 30. |

### Pedal block

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `keycode` | evdev key name, e.g. `KEY_F13` | none | Written to the pedal's slot on startup if it differs. If omitted, the stored key is read and left unchanged. |
| `control` | Viam button control name | `ButtonLT` / `ButtonSelect` / `ButtonRT` | The control the pedal reports as. |

If a pedal is in mouse or macro mode, set a `keycode` for it; the component won't start otherwise.

## Events

`ButtonPress` (value 1) and `ButtonRelease` (value 0) for each pedal, plus `Connect` and `Disconnect`. Autorepeat is ignored. If the pedal is unplugged while held, or the component closes, a `ButtonRelease` is sent for each held pedal. Unplugged pedals are polled for every second and picked up again automatically.

`TriggerEvent` is not supported.

## DoCommand

`{"command": "list_pedals"}` returns every pedal with its slot, control, key, where the key came from, mode and live state:

```json
{
  "model": "pcsensor-3-pedal",
  "device": { "usb_id": "3553:b001", "path": "/dev/input/by-path/pci-0000:00:14.0-usb-0:6:1.0-event-kbd", "connected": true },
  "pedals": [
    { "name": "left", "slot": 1, "control": "ButtonLT", "keycode": "KEY_F13", "keycode_source": "config", "mode": "key", "pressed": false }
  ]
}
```
