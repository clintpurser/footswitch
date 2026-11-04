# Model `clint:footswitch:pcsensor-1-pedal`

An input controller for PCsensor-family single-pedal USB foot pedals. It finds the pedal, grabs it so its key never types into other programs, reads (or programs) which key the pedal sends, and reports the pedal as one button.

## Supported hardware

PCsensor-protocol single-pedal USB foot pedals, sold as iKKEGOL, PCsensor and unbranded. They show up as "PCsensor FootSwitch" in `lsusb`.

| USB ID | Status |
| --- | --- |
| `3553:b001` | Tested: iKKEGOL single-pedal |
| `1a86:e026` | Same protocol per [rgerganov/footswitch](https://github.com/rgerganov/footswitch); untested |
| `0c45:7403` | Same protocol; untested |
| `0c45:7404` | Same protocol; untested |
| `413d:2107` | Same protocol; untested |

**Pick this model by knowing your hardware.** A single-pedal unit is the 3-pedal unit's circuit board with one switch wired to slot 2. Over USB it is identical to a 3-pedal unit: same USB ID, same descriptors, and all three key slots stored and reported. The module cannot tell the two apart, so a single-pedal unit configured as [`pcsensor-3-pedal`](clint_footswitch_pcsensor-3-pedal.md) starts fine but reports two pedals that never fire, and a 3-pedal unit configured as this model reports only its center pedal.

Not supported: 2-pedal units, the `5131:2019` single-pedal unit (a different protocol), and non-PCsensor pedals.

Runs on Linux only (`linux/amd64`, `linux/arm64`), such as a Raspberry Pi, Jetson or x86 PC. `viam-server` needs access to `/dev/input/event*` and `/dev/hidraw*`, which it has when running as root (the default).

## Configuration

Every attribute is optional. With `{}`, the component finds the pedal, reads which key it sends and reports it as `ButtonSouth`.

```json
{
  "pedal": { "keycode": "KEY_F14" }
}
```

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `pedal` | pedal block | all defaults | The one pedal. |
| `slot` | integer 1, 2 or 3 | 2 | Which of the firmware's three key slots the switch is wired to. Only for a unit that turns out to be wired differently from the tested one: if the pedal never fires and `list_pedals` shows the wrong key, try another slot. |
| `device_path` | string | auto-detect | A `/dev/input/by-path/…` path. Picks one device when several pedals are plugged in. |
| `usb_id` | string `vvvv:pppp` | any supported ID | Restricts detection to one USB ID. |
| `debounce_ms` | integer | 25 | Changes closer together than this are filtered. Keep below about 30. |

### Pedal block

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `keycode` | evdev key name, e.g. `KEY_F14` | none | Written to the pedal's slot on startup if it differs. If omitted, the stored key is read and left unchanged. The other two slots are never changed. |
| `control` | Viam button control name | `ButtonSouth` | The control the pedal reports as. |

If the pedal is in mouse or macro mode, set a `keycode` for it; the component won't start otherwise.

The default control differs from the 3-pedal model's center pedal (`ButtonSelect`). An actions config written for a 3-pedal center pedal needs its control renamed to `ButtonSouth`, or set `control` here to match.

## Events

`ButtonPress` (value 1) and `ButtonRelease` (value 0), plus `Connect` and `Disconnect`. Autorepeat is ignored. If the pedal is unplugged while held, or the component closes, a `ButtonRelease` is sent. An unplugged pedal is polled for every second and picked up again automatically.

`TriggerEvent` is not supported.

## DoCommand

`{"command": "list_pedals"}` returns the pedal with its slot, control, key, where the key came from, mode and live state:

```json
{
  "model": "pcsensor-1-pedal",
  "device": { "usb_id": "3553:b001", "path": "/dev/input/by-path/pci-0000:00:14.0-usb-0:6:1.0-event-kbd", "connected": true },
  "pedals": [
    { "name": "pedal", "slot": 2, "control": "ButtonSouth", "keycode": "KEY_F14", "keycode_source": "config", "mode": "key", "pressed": false }
  ]
}
```
