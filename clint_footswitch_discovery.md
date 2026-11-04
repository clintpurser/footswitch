# Model `clint:footswitch:discovery`

A discovery service that finds attached PCsensor pedals and suggests a [`clint:footswitch:pcsensor-3-pedal`](clint_footswitch_pcsensor-3-pedal.md) and a [`clint:footswitch:pcsensor-1-pedal`](clint_footswitch_pcsensor-1-pedal.md) component for each, with `device_path` set.

## Supported hardware

Finds the same pedals as [`pcsensor-3-pedal`](clint_footswitch_pcsensor-3-pedal.md#supported-hardware): USB IDs `3553:b001` (tested), `1a86:e026`, `0c45:7403`, `0c45:7404` and `413d:2107`. Linux only; on other platforms it returns no suggestions.

## Configuration

No attributes: `{}`.

## Usage

Add the service, open its **Test** panel and add the suggested component that matches your hardware. Each attached pedal produces two suggestions, because a single-pedal unit is indistinguishable from a 3-pedal unit over USB:

| Suggestion | Model | Add it if |
| --- | --- | --- |
| `footswitch-<vendor>-<product>-<usb port>` | `pcsensor-3-pedal` | the unit has three pedals |
| `footswitch-1-pedal-<vendor>-<product>-<usb port>` | `pcsensor-1-pedal` | the unit has one pedal |

For example `footswitch-3553-b001-1.2` and `footswitch-1-pedal-3553-b001-1.2`; the USB port in the name keeps identical pedals apart. Pedals already in use are still listed.

Add only one of the two for a given pedal. If both are added, the second one to start can't grab the pedal and fails with "is another program using the pedal?".

Discovery only suggests input controllers; add the `clint:footswitch:actions` service by hand.
