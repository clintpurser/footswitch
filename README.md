# footswitch

Use PCsensor-family 3-pedal USB foot pedals (sold as iKKEGOL, PCsensor and unbranded) as Viam input controllers, and run DoCommands on other resources when a pedal is pressed or released.

Keywords: iKKEGOL, PCsensor, USB foot pedal, footswitch.

> **Safety:** a USB keyboard pedal can drop events, and the module can crash. This module is a convenience control and does not replace a hardware e-stop.

## Models

- [`clint:footswitch:pcsensor-3-pedal`](clint_footswitch_pcsensor-3-pedal.md): input controller for a 3-pedal unit.
- [`clint:footswitch:actions`](clint_footswitch_actions.md): generic service that runs DoCommands when any input controller's buttons are pressed or released.
- [`clint:footswitch:discovery`](clint_footswitch_discovery.md): discovery service that suggests a config for each attached pedal.

Supported USB IDs: `1a86:e026`, `3553:b001`, `0c45:7403`, `0c45:7404`, `413d:2107`.

Platforms: Linux only (`linux/amd64`, `linux/arm64`). The module reads the pedal through evdev and programs it through hidraw, so `viam-server` needs read access to `/dev/input/event*` and read/write access to `/dev/hidraw*`. It does when it runs as root, which is the default.

## Quick start

1. Plug in the pedal.
2. Add the `clint:footswitch:discovery` service, open its **Test** panel and add the suggested `pcsensor-3-pedal` component. Or add the component by hand with empty attributes `{}`.
3. Press the pedals and watch the machine's **Logs** tab:

   ```text
   pedal left pressed (slot 1, KEY_F13)
   pedal left released after 1.42s (slot 1, KEY_F13)
   ```

4. Add a `clint:footswitch:actions` service to make the pedals do something.

## Development

```sh
make test            # unit tests, run on any OS
make module.tar.gz   # build for the current OS/arch (set VIAM_BUILD_OS/VIAM_BUILD_ARCH to cross-compile)
go run ./cmd/cli     # on Linux: open the pedal and log presses without viam-server
```

Settings protocol reference: [rgerganov/footswitch](https://github.com/rgerganov/footswitch).
