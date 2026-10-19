# Model `clint:footswitch:discovery`

A discovery service that finds attached PCsensor pedals and suggests a `clint:footswitch:pcsensor-3-pedal` component for each, with `device_path` set.

## Configuration

No attributes: `{}`.

## Usage

Add the service, open its **Test** panel and add a suggested component. Suggestions are named `footswitch-<vendor>-<product>-<usb port>`, for example `footswitch-3553-b001-1.2`, so identical pedals get different names. Pedals already in use are still listed.

Discovery only suggests input controllers; add the `clint:footswitch:actions` service by hand.
