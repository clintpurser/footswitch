# Model `clint:footswitch:discovery`

A discovery service that finds attached PCsensor pedals and suggests a `clint:footswitch:pcsensor-3-pedal` component for each, with `device_path` set.

## Supported hardware

Finds the same pedals as [`pcsensor-3-pedal`](clint_footswitch_pcsensor-3-pedal.md#supported-hardware): USB IDs `3553:b001` (tested), `1a86:e026`, `0c45:7403`, `0c45:7404` and `413d:2107`. Linux only; on other platforms it returns no suggestions.

## Configuration

No attributes: `{}`.

## Usage

Add the service, open its **Test** panel and add a suggested component. Suggestions are named `footswitch-<vendor>-<product>-<usb port>`, for example `footswitch-3553-b001-1.2`, so identical pedals get different names. Pedals already in use are still listed.

Discovery only suggests input controllers; add the `clint:footswitch:actions` service by hand.
