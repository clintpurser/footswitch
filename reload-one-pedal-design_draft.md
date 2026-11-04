# Single-pedal support for the footswitch module

*   **Author:** Clint Purser
*   **Status:** Draft
*   **Date:** 2026-11-04

--------------------------------------------------------------------------------

## Overview

The `footswitch` module today supports PCsensor-family 3-pedal USB foot pedals
through one input controller model, `clint:footswitch:pcsensor-3-pedal`. PCsensor
also sells a single-pedal unit. It is the same circuit board in a smaller case
with only one switch wired up, and it is sold under the same brands (iKKEGOL,
PCsensor, unbranded).

This design adds support for that single-pedal unit so it works as a Viam input
controller with exactly one button, can have its key read and programmed like
the 3-pedal unit, and can be set up from the discovery service without the user
knowing anything about slots or USB IDs.

## The Problem

Hardware investigation on the author's single-pedal unit (USB ID `3553:b001`,
product string "PCsensor FootSwitch") found that it is indistinguishable from
the 3-pedal unit over USB:

| Property | Single-pedal unit | 3-pedal unit |
| --- | --- | --- |
| USB ID and product string | `3553:b001`, "PCsensor FootSwitch" | same |
| USB interfaces | interface 0 keyboard and mouse, interface 1 settings | same |
| HID report descriptors | identical bytes | same |
| Stored slot settings (factory) | slot 1 `a`, slot 2 `b`, slot 3 `c` | same |
| Pedal that is wired up | slot 2 only (presses type `b`) | all three |

The firmware does not know how many pedals are wired to it. It reports and
programs all three slots whether or not a switch is attached. The spec's earlier
guess that unused slots would read as "unconfigured" is wrong.

Because of this, the module as shipped mishandles a single-pedal unit in several
ways:

*   `pcsensor-3-pedal` starts without error and reports three controls. Two of
    them (`ButtonLT` for slot 1 and `ButtonRT` for slot 3) can never fire. Any
    consumer, including the module's own actions service, sees phantom buttons
    and `list_pedals` lies about the hardware.
*   A `left` or `right` block with a `keycode` writes a key into a slot that has
    no switch, which is a wasted flash write and a confusing config.
*   The discovery service suggests `pcsensor-3-pedal` for the unit, because it
    cannot tell the two apart. The suggested config is wrong and nothing tells
    the user so.
*   The documentation states that 1-pedal units are not supported.

The pedal count therefore has to come from the user's configuration, not from
the device. The design must make that choice easy to get right, keep existing
3-pedal configs working unchanged, and keep the discovery flow useful even
though discovery cannot detect the count.

## Goals

*   A single-pedal PCsensor unit works as an input controller that reports
    exactly one control, with press, release, connect and disconnect events,
    debounce, hotplug and `list_pedals` behaving as they do for the 3-pedal
    model.
*   The user declares the pedal count in the config. The module never guesses
    it from the device, because the device cannot tell.
*   The single pedal's key can be read from the device and programmed through
    the config, using slot 2. Slots 1 and 3 are read as part of the protocol but
    never written to, except where the protocol forces a full rewrite, in which
    case their stored values are written back unchanged.
*   A user with a single pedal can get to a working config from the discovery
    service's Test panel without reading about slots or USB IDs.
*   Existing `pcsensor-3-pedal` configs keep working with no changes and no new
    warnings.
*   The single-pedal path is covered by unit tests using the existing fake
    event source, fake slot driver and fake sysfs tree, so it runs in CI on any
    OS.
*   The change is verified on a Linux machine with the author's single-pedal
    unit before release.

## Non-Goals

*   2-pedal PCsensor units. There is no hardware to test, and the slot a 2-pedal
    unit uses is unknown. The design should not make adding one harder.
*   The `5131:2019` single-pedal unit, which uses a different protocol on a
    different USB interface (the upstream `footswitch1p` tool). It needs its own
    driver and is out of scope.
*   Non-PCsensor pedals.
*   Detecting the pedal count from the device. The investigation shows this is
    not possible for `3553:b001`.
*   Running the module on macOS or Windows. The module reads evdev and hidraw
    and stays Linux-only.
*   Pedals in mouse or macro mode. As today, a pedal in those modes needs a
    `keycode` so the module can program a key.
*   Changing the actions service. It listens to any input controller and needs
    no knowledge of pedal counts.

## Open Questions

*   **Config shape**: whether single-pedal support is a new model
    (`pcsensor-1-pedal`), a `pedal_count` attribute on `pcsensor-3-pedal`, or
    a single slot-keyed model. Decided in the Technical Plan.
*   **Default control for the single pedal**: whether it keeps slot 2's current
    default (`ButtonSelect`) or gets a more conventional single-button default
    such as `ButtonSouth`. Decided in the Technical Plan.
*   **Discovery output when the count is unknown**: whether discovery suggests
    one config, both, or one config with a note. Decided in the Technical Plan.
*   **Slot of other single-pedal units**: only one unit was tested. The
    upstream tool's default of pedal 2 "also works for single pedal devices",
    which suggests slot 2 is the norm, but a different unit could be wired to a
    different slot. The design should make the slot overridable in the config
    so a user with such a unit is not stuck.

## Critical User Journeys (CUJs)

### CUJ 1: Single pedal as a button

*   **Given**: A single-pedal PCsensor unit (`3553:b001`) is plugged into a
    Linux machine running `viam-server`, with factory settings.
*   **When**: The user adds the single-pedal configuration and presses and
    releases the pedal.
*   **Then**: The component starts without error, `Controls()` returns one
    control, the Logs tab shows `pedal pressed (slot 2, KEY_B)` and `pedal
    released after 0.41s (slot 2, KEY_B)`, and `list_pedals` lists one pedal
    with `slot: 2`, `keycode: KEY_B`, `keycode_source: device`.

### CUJ 2: Programming the single pedal's key

*   **Given**: The same unit, configured with `keycode: KEY_F14` for the pedal.
*   **When**: The component starts.
*   **Then**: Slot 2 is programmed to F14 and the log says so. Slots 1 and 3
    read back with their previous values. Presses arrive as `KEY_F14` and the
    pedal no longer types `b` into anything, because the module grabs it.

### CUJ 3: Set up from discovery

*   **Given**: A single-pedal unit is plugged in and the user has added the
    `clint:footswitch:discovery` service.
*   **When**: The user opens the service's Test panel.
*   **Then**: The user can pick a suggestion that results in a working
    single-pedal component, and understands from what they see that the pedal
    count is their choice because the device cannot report it.

### CUJ 4: Existing 3-pedal users are unaffected

*   **Given**: A machine already running `pcsensor-3-pedal` with `left`,
    `center` and `right` blocks.
*   **When**: The module is upgraded to the version with single-pedal support.
*   **Then**: The component starts, reports the same three controls, programs
    the same keys and emits the same events and log lines as before. No config
    change is needed.

### CUJ 5: Hold-to-run action on a single pedal

*   **Given**: CUJ 1's component plus a `clint:footswitch:actions` service with
    `on_press` and `on_release` on the pedal's control.
*   **When**: The user holds the pedal, then unplugs it while held.
*   **Then**: `on_press` runs on the press, and `on_release` runs when the
    device disappears, exactly as it does for the 3-pedal unit.

## Technical Plan (Architecture)

### What already fits

The hardware-independent core (`core.go`) is already built around a list of
`pedalSpec` values, each with a name, a slot number and a control. Nothing in
the core assumes three pedals: it builds the key-to-pedal map from whatever
specs it is given, and `setup` only inspects the slots that have a spec. The
PCsensor driver (`pcsensor.go`) always reads and writes all three slots because
the protocol requires it, and `setup` already writes back untouched slots with
their stored values. So the single-pedal unit needs no core or driver change
for events, debounce, hotplug, programming or `list_pedals`.

What does assume three pedals is the model layer (`module.go`): the `Config`
struct has fixed `left`, `center` and `right` fields, `pedalLayout` is a fixed
three-entry table, and `pedalSpecs` always produces three specs. Discovery
(`discovery.go`) always suggests `pcsensor-3-pedal`.

### Decision 1: how the user declares the pedal count

The device cannot report its pedal count, so the configuration has to. Three
shapes were considered; the rejected two are in Alternatives Considered.

**Decision (author): Option A.** The slot number must stay hidden from the
user, which rules out B. Between A and C, an honest model name that someone
with a single-pedal unit can find in the registry outweighs the duplicated
model entry, docs and tests. 
#### Chosen: Option A, a second model, `clint:footswitch:pcsensor-1-pedal`

*   **Mechanism**: register a second input controller model that shares the
    core and driver. Its config has one pedal block, `pedal`, plus an optional
    `slot` (default 2) for units wired differently, and the same `device_path`,
    `usb_id` and `debounce_ms`. `module.go` grows a per-model layout table
    (names, default controls, slots) and a per-model `Validate`. `meta.json`,
    `cmd/module/main.go` and the docs get a new model entry.
*   **Trade-offs**: follows the existing spec's rule that the pedal count lives
    in the model name so `Validate` knows it without the device. Each model's
    docs and config are small and specific. `pcsensor-3-pedal` is untouched.
    Two models with the same USB ID means the registry lists two entries the
    user has to choose between, and every per-model file (docs, meta, tests)
    is duplicated. A 2-pedal unit would be a third model.
*   **Fails when**: the user picks the wrong model. Both start without error
    on either unit, so the mistake only shows as phantom or missing pedals.
    Discovery cannot pick for them; see Decision 2.


### Decision 2: what discovery suggests

Discovery scans sysfs for keyboard event devices with a supported USB ID. For
each device it finds, it now has two models it could suggest and no way to
tell which is right. Three behaviours were considered; the rejected two are in Alternatives Considered.

**Decision (author): Option D1.** The choice is made in the Test panel, where
the user is already looking, with no attribute to learn first. 
#### Chosen: Option D1, suggest both models for every device

*   **Mechanism**: `suggestConfigs` emits two configs per device, with
    `device_path` set on both: `footswitch-3553-b001-6` with model
    `pcsensor-3-pedal` (unchanged name, so existing users see nothing new) and
    `footswitch-1-pedal-3553-b001-6` with model `pcsensor-1-pedal`. The Test
    panel lists both; the user adds the one that matches their hardware.
*   **Trade-offs**: no new attributes, no docs to read first, and the choice is
    made where the user is already looking. The panel is noisier: two entries
    per pedal, and a user with three 3-pedal units sees six suggestions. The
    names are the only hint of which is which, so they must say "1-pedal".
*   **Fails when**: the user adds both suggestions for the same device. The
    second one to start cannot grab the event device and fails with the
    existing "is another program using the pedal?" error, which does not say
    that the other suggestion is the cause.


### Decision 3: the single pedal's default control

The control name is the pedal's public interface: the actions service and any
other consumer key on it. `control` in the pedal block overrides it either
way; this decides the default when the block is omitted.

**Decision (author): Option E2, `ButtonSouth`.** Option E1 is in Alternatives
Considered.

#### Chosen: Option E2, `ButtonSouth`, the conventional primary button

*   **Mechanism**: the layout table entry uses `ButtonSouth`, the gamepad
    "A" button that Viam's own gamepad examples treat as the main action.
*   **Trade-offs**: reads as "the button" in configs and logs. Actions
    configs moved between the 3-pedal center pedal and the single pedal need
    one key renamed. The 3-pedal model keeps `ButtonLT` / `ButtonSelect` /
    `ButtonRT`, so the two models' defaults are unrelated, which has to be
    stated in the docs.
*   **Fails when**: a user assumes the single pedal is "the center pedal of a
    3-pedal unit" and reuses that config unchanged; the actions service warns
    that the controller doesn't report `ButtonSelect`, and nothing runs until
    they fix the name.

### Proposed design

#### Models

| | `pcsensor-3-pedal` (unchanged) | `pcsensor-1-pedal` (new) |
| --- | --- | --- |
| Pedal blocks | `left`, `center`, `right` | `pedal` |
| Slots | 1, 2, 3 | 2 by default; `slot` attribute overrides |
| Default controls | `ButtonLT`, `ButtonSelect`, `ButtonRT` | `ButtonSouth` |
| Finds the device | by `device_path`, `usb_id` or the supported ID list | same |
| Reads and programs keys | yes | yes, for its one slot |
| `list_pedals` `model` | `pcsensor-3-pedal` | `pcsensor-1-pedal` |

Both models register under `rdk:component:input_controller` and run on the
same core and driver. Neither can tell which unit is plugged in; the user's
choice of model is the declaration of pedal count.

#### Configuration of `pcsensor-1-pedal`

Every attribute is optional. With `{}` the component finds the pedal, reads
the key stored in slot 2 and reports the pedal as `ButtonSouth`.

```json
{
  "name": "pedal",
  "api": "rdk:component:input_controller",
  "model": "clint:footswitch:pcsensor-1-pedal",
  "attributes": {
    "pedal": { "keycode": "KEY_F14", "control": "ButtonSouth" },
    "slot": 2,
    "device_path": "/dev/input/by-path/pci-0000:00:14.0-usb-0:6:1.0-event-kbd",
    "usb_id": "3553:b001",
    "debounce_ms": 25
  }
}
```

| Attribute | Type | Default | Meaning |
| --- | --- | --- | --- |
| `pedal` | pedal block (`keycode`, `control`) | all defaults | The one pedal. Same block shape as the 3-pedal model. |
| `slot` | integer 1, 2 or 3 | 2 | Which of the firmware's three slots the switch is wired to. Only for units that turn out to be wired differently from the tested one. |
| `device_path`, `usb_id`, `debounce_ms` | as today | as today | Shared with the 3-pedal model. |

Unknown attributes are rejected, as today, with a message listing the allowed
ones (`pedal, slot, device_path, usb_id, debounce_ms`).

#### Component map

```text
                      module.go (model layer)
  +---------------------------------+   +---------------------------------+
  | pcsensor-3-pedal                |   | pcsensor-1-pedal (new)          |
  | Config{left,center,right,...}   |   | OnePedalConfig{pedal,slot,...}  |
  | layout: 3 specs, slots 1-3      |   | layout: 1 spec, slot = cfg.slot |
  +---------------+-----------------+   +----------------+----------------+
                  |  []pedalSpec {name,label,slot,control,keycode}         |
                  v                                                      v
            +-----------------------------------------------------------+
            | controllerCore (core.go)   key map, debounce, callbacks,  |
            |                            hotplug, list_pedals, logging  |
            +-----------+-----------------------------------+-----------+
                        | events                            | slot settings
                        v                                   v
            +------------------+                 +-----------------------+
            | evdev keyboard   |                 | pcsensorDriver        |
            | (grabbed)        |                 | readSlots/writeSlots  |
            +------------------+                 | always all 3 slots    |
                                                 +-----------------------+
            discovery.go: one scan -> two suggestions per device
                 footswitch-<id>-<port>          -> pcsensor-3-pedal
                 footswitch-1-pedal-<id>-<port>  -> pcsensor-1-pedal
```

#### What changes in each layer

*   **Model layer (`module.go`)**: the fixed `pedalLayout` table becomes a
    per-model layout, and `pedalSpecs` is generalised to take a layout and a
    list of pedal blocks in slot order. The shared `device_path`, `usb_id` and
    `debounce_ms` fields and their validation move into one embedded struct
    both configs use, so the two `Validate` functions only differ in their
    pedal blocks and, for the new model, `slot`. The new model's constructor
    calls `newControllerCore` with one spec whose slot is `cfg.slot`. The
    constructor and `Validate` for `pcsensor-3-pedal` keep their behaviour.
*   **Core (`core.go`)**: one cosmetic change. `pedalSpec` gains a `label`
    used in log lines and setup errors instead of `"pedal " + name`. For the
    3-pedal model the label is `pedal left` and so on, so its output is
    byte-for-byte what it is today; for the new model it is `pedal`, so logs
    read `pedal pressed (slot 2, KEY_F14)` rather than `pedal pedal pressed`.
    Nothing else in the core changes: the key map, setup, debounce, hotplug,
    `list_pedals` and the release-on-disconnect path already work on any list
    of specs.
*   **Driver (`pcsensor.go`)**: no change. On startup `setup` reads all three
    slots as before. If the configured `keycode` differs from the stored key
    in the pedal's slot, it writes all three slots, passing slots 1 and 3
    back with their stored values, which is what the protocol requires and
    what the 3-pedal model already does for unconfigured slots.
*   **Discovery (`discovery.go`)**: `suggestConfigs` emits two configs per
    device. The 3-pedal suggestion keeps its current name and shape. The new
    one is named `footswitch-1-pedal-<vendor>-<product>-<port>` with model
    `pcsensor-1-pedal` and the same `device_path`. The ordering is 3-pedal
    first, then 1-pedal, per device.
*   **Module entry point (`cmd/module/main.go`)** and **`meta.json`**: register
    and describe the new model.
*   **Local CLI (`cmd/cli/main.go`)**: gains a `-pedals` flag (`1` or `3`,
    default `3`) so the single-pedal path can be exercised on a Linux box
    without `viam-server`.

#### Startup sequence for `pcsensor-1-pedal`

1.  Find the device by `device_path`, `usb_id` or the supported ID list, open
    its keyboard event device and grab it (unchanged).
2.  Read the three stored slot settings through hidraw (unchanged).
3.  Look only at slot `cfg.slot` (default 2). If `pedal.keycode` is set and the
    stored key differs, write all three slots with only that slot changed and
    log `programmed pedal (slot 2) to send KEY_F14`. If no keycode is set,
    use the stored key; fail with the existing messages if the slot is in
    mouse or macro mode, is unconfigured, or holds a key the module does not
    know.
4.  Build the key map with one entry and emit `Connect` on the one control.
5.  Keys from other slots cannot arrive (no switch is wired to them). If they
    do, the existing "no pedal is mapped to it" warning fires once per key.

#### Validation rules for `pcsensor-1-pedal`

`Validate` rejects the config when:

*   a top-level key is not `pedal`, `slot`, `device_path`, `usb_id` or
    `debounce_ms`;
*   `slot` is present and not 1, 2 or 3;
*   `pedal.keycode` is not a known evdev key name with a USB HID equivalent, or
    is a modifier key;
*   `pedal.control` is not a Viam button control name;
*   `usb_id` or `debounce_ms` fail the existing checks.

It declares no dependencies.

#### `list_pedals` for the new model

```json
{
  "model": "pcsensor-1-pedal",
  "device": { "usb_id": "3553:b001", "path": "/dev/input/by-path/...-event-kbd", "connected": true },
  "pedals": [
    { "name": "pedal", "slot": 2, "control": "ButtonSouth", "keycode": "KEY_F14", "keycode_source": "config", "mode": "key", "pressed": false }
  ]
}
```

#### Adding a 2-pedal model later

A 2-pedal unit becomes one more layout table entry and config struct once
someone learns which two slots it uses. Nothing in this design has to be
undone for that; the per-model layout and the shared device config are the
seams it would use.

## Alternatives Considered

### Alternative A: detect the pedal count from the device

*   **Pros**: no user choice; discovery would suggest the right model and
    the 3-pedal model could refuse to start on a single-pedal unit.
*   **Cons**: there is nothing to detect. The single-pedal unit has the same
    USB ID, product string, interface layout and HID report descriptors as
    the 3-pedal unit, and its three slots hold the same factory keys. The
    spec's guess that unused slots would read as unconfigured was tested and
    found false.
*   **Reason for Rejection**: ruled out by the hardware investigation on the
    author's unit (`3553:b001`, read through hidapi on macOS). Revisit only if
    another USB ID or firmware turns out to expose the count.

### Alternative B: a slot-keyed model, `clint:footswitch:pcsensor`

*   **Pros**: one model covers 1, 2 and 3 pedals and any wiring; no new model
    per pedal count.
*   **Cons**: the user has to know which firmware slot their switch is wired
    to, which is exactly what a single-pedal owner cannot be expected to
    know. Two models with overlapping capability need explaining. A guessed
    slot that never fires looks like a hardware fault.
*   **Reason for Rejection**: Rejected by author: the slot number must stay
    hidden behind the model; a user with a one-switch unit has no way to learn
    that it sends slot 2.

### Alternative C: a `pedal_count` attribute on `pcsensor-3-pedal`

*   **Pros**: smallest change: one attribute and a layout table, no new
    model, existing configs untouched because the default is 3. Hides the
    slot as well as the chosen design does.
*   **Cons**: the model name would no longer describe the hardware. Someone
    searching the registry for a single-pedal driver sees only a model called
    "3-pedal". The registry's one-line description would have to cover both.
    Renaming the model would break existing configs.
*   **Reason for Rejection**: Rejected by author: an honest model name that a
    single-pedal owner can find outweighs the duplicated model entry, docs and
    tests.

### Alternative D2: discovery keeps suggesting only `pcsensor-3-pedal`

*   **Pros**: no code change and no extra entries in the Test panel.
*   **Cons**: the suggestion shown to a single-pedal user is wrong and nothing
    in the panel says so. A trusted suggestion starts with two phantom pedals,
    visible only in `list_pedals` or the actions service's "doesn't report
    control" warning.
*   **Reason for Rejection**: Rejected by author: it fails the goal that a
    single-pedal user reaches a working config from the Test panel.

### Alternative D3: a `pedal_count` attribute on the discovery service

*   **Pros**: one correctly modelled suggestion per device once the attribute
    is set; the user states their pedal count once in an obvious place.
*   **Cons**: the default of 3 still misleads on first look. The discovery
    service stops being attribute-free. A machine with one 3-pedal and one
    1-pedal unit cannot be described by a single attribute.
*   **Reason for Rejection**: Rejected by author: making the choice where the
    user is already looking, in the Test panel, needs no attribute to learn
    first and handles mixed units.

### Alternative E1: `ButtonSelect` as the single pedal's default control

*   **Pros**: actions configs written for a 3-pedal unit's center pedal port
    to the single pedal unchanged, since both are slot 2.
*   **Cons**: "Select" is a gamepad name for a secondary button and means
    nothing for a lone pedal; users wiring it as the primary action have to
    override it and the docs have to explain the odd default.
*   **Reason for Rejection**: Rejected by author: `ButtonSouth` reads as "the
    button". Porting a center-pedal config costs one renamed key, and the
    actions service's existing warning names the mismatch at startup.

### Alternative F: support the `5131:2019` single-pedal unit in the same change

*   **Pros**: covers the other single-pedal product sold under the same
    brands.
*   **Cons**: it speaks a different settings protocol on a different USB
    interface (the upstream `footswitch1p` tool), so it needs its own driver
    behind the `slotIO` seam, and there is no hardware to test it on.
*   **Reason for Rejection**: Rejected by author: out of scope until there is
    a unit to test. The new model's USB ID list can grow once a driver exists.

--------------------------------------------------------------------------------

## Non-Functional Requirements

*   **Rollout & Rollback**: the change is additive. `pcsensor-3-pedal` keeps
    its config, behaviour and log output, so upgrading needs no config edits.
    Rolling back means pinning the previous module version in the machine
    config; a `pcsensor-1-pedal` component then fails to start with "model not
    found", which is the expected rollback signal. Release follows the
    existing tag-triggered deploy workflow.
*   **Flash wear**: the pedal's settings live in flash and the protocol
    rewrites all three slots on every write. As today, the module writes only
    when a configured `keycode` differs from the stored key, so a stable config
    writes once and never again.
*   **Security & Privacy**: nothing new. The module still needs read access
    to `/dev/input/event*` and read/write to `/dev/hidraw*`.
*   **Performance**: no change to the event path. One extra config per device
    in discovery output.

--------------------------------------------------------------------------------

### Additional Resources

*   `iKKEGOL 3-Pedal Footswitch — Viam Input Controller Module Spec.md` (local,
    gitignored): the original module spec, including the "Later: 1- and
    2-pedal units" section this design replaces.
*   [rgerganov/footswitch](https://github.com/rgerganov/footswitch): the
    settings protocol reference. Its README notes that programming "the second
    pedal (default) also works for single pedal devices", which matches the
    slot 2 finding.
*   Hardware probe, 2026-11-04: a read-only hidapi query of the author's
    single-pedal unit (`3553:b001`) on macOS returned slot settings
    `08 01 00 04`, `08 01 00 05`, `08 01 00 06` (keys `a`, `b`, `c`), with HID
    report descriptors identical to the 3-pedal unit's. Pressing the pedal
    types `b`.

### Future Improvements

*   **2-pedal model**: one more layout entry and config struct once a unit
    is available and its slots are known.
*   **`5131:2019` driver**: a second `slotIO` implementation for the other
    single-pedal protocol, behind the existing seam.
*   **Re-test detection on other USB IDs**: `1a86:e026`, `0c45:7403`,
    `0c45:7404` and `413d:2107` were not probed. If any of them exposes the
    pedal count, discovery could use it for that ID only.
*   **Start-up cross-check**: the 3-pedal model could warn at startup if a
    slot other than the pressed one never fires over a long period. It cannot
    be an error, since an unused pedal is normal.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

Naming decisions made here so the implementer does not have to: the new
model's Go identifiers are `Pcsensor1Pedal` (model var), `OnePedalConfig`
(config struct) and `NewPcsensor1Pedal` (constructor); the new model's code
lives in `onepedal.go` with tests in `onepedal_test.go`; the shared device
attributes live in an embedded `DeviceConfig` struct; the per-model layout
type is `pedalLayoutEntry` with fields `name`, `label`, `slot` and `control`.

### Target Files to Create/Modify

#### 1. `./module.go` (Modify)

*   **Type / Scope**: Go, model layer for `pcsensor-3-pedal` and shared config
    helpers.
*   **Proposed Changes**:
    *   Extract `DevicePath`, `USBID` and `DebounceMS` from `Config` into a new
        exported `DeviceConfig` struct, embedded anonymously in `Config` (and
        in `OnePedalConfig`) so the JSON shape is unchanged. Move the `usb_id`
        and `debounce_ms` checks from `Config.Validate` into
        `(*DeviceConfig).validateDevice(path string) error`, and add a
        `(*DeviceConfig).debounce() time.Duration` helper that applies the
        default of 25 ms. `Config.Validate` and `NewPcsensor3Pedal` call these.
    *   Replace the anonymous `pedalLayout` table with
        `type pedalLayoutEntry struct { name, label string; slot int; control input.Control }`
        and `var threePedalLayout = []pedalLayoutEntry{{"left", "pedal left", 1, ButtonLT}, {"center", "pedal center", 2, ButtonSelect}, {"right", "pedal right", 3, ButtonRT}}`.
    *   Generalise `pedalSpecs` into a package-level
        `resolveSpecs(path string, layout []pedalLayoutEntry, blocks []*PedalConfig) ([]pedalSpec, error)`
        that walks the layout in order, takes `blocks[i]` for the block,
        and sets `name`, `label`, `slot` and `control` from the entry. Keep the
        existing duplicate-keycode and duplicate-control checks and their
        error messages. `(*Config).pedalSpecs(path)` becomes a one-line call
        with `threePedalLayout` and `cfg.pedalBlocks()`, so every existing
        error string is unchanged.
    *   Keep `convertConfig`'s allowed-key message as it is; add a sibling
        `convertOnePedalConfig` in `onepedal.go` (see file 2) rather than
        generalising the message.
*   **Why**: the 3-pedal model hard-codes its layout and owns the shared
    attributes. The new model needs both without copying them.

#### 2. `./onepedal.go` (Create)

*   **Type / Scope**: Go, model layer for `pcsensor-1-pedal`.
*   **Proposed Changes**:
    *   `var Pcsensor1Pedal = resource.NewModel("clint", "footswitch", "pcsensor-1-pedal")`
        and an `init()` that registers it under `input.API` with
        `Constructor: newFootswitchPcsensor1Pedal` and
        `AttributeMapConverter: convertOnePedalConfig`.
    *   `type OnePedalConfig struct { DeviceConfig; Pedal *PedalConfig \`json:"pedal,omitempty"\`; Slot *int \`json:"slot,omitempty"\` }`.
    *   `convertOnePedalConfig`: same strict decode as `convertConfig`, with
        the message "allowed: pedal, slot, device_path, usb_id, debounce_ms;
        the pedal block allows keycode and control".
    *   `const defaultOnePedalSlot = 2`. `(*OnePedalConfig).slotNumber() int`
        returns `*Slot` or the default.
    *   `(*OnePedalConfig).layout() []pedalLayoutEntry` returns one entry
        `{"pedal", "pedal", cfg.slotNumber(), input.ButtonSouth}`.
    *   `(*OnePedalConfig).Validate(path)`: reject `slot` outside 1..3 with
        `"%s: slot must be 1, 2 or 3, got %d"`; then
        `resolveSpecs(path, cfg.layout(), []*PedalConfig{cfg.Pedal})`; then
        `validateDevice`. Returns no dependencies.
    *   `newFootswitchPcsensor1Pedal` (native-config unwrap) and exported
        `NewPcsensor1Pedal(ctx, deps, name, conf, logger) (input.Controller, error)`
        that builds `coreOptions{model: Pcsensor1Pedal.Name, pedals: specs, debounce: conf.debounce(), open: openPedal with conf.DevicePath and conf.USBID}`.
    *   The resource type `footswitchPcsensor1Pedal` is the same shape as
        `footswitchPcsensor3Pedal` (name, logger, core, `AlwaysRebuild`) and
        the same method bodies. To avoid copying them, rename the existing
        struct to `pedalController` in `module.go` and have both constructors
        return it; it carries no model-specific state.
*   **Why**: the new model is a thin wrapper, like the existing one, and its
    own file keeps the two models' configs apart.

#### 3. `./core.go` (Modify)

*   **Type / Scope**: Go, hardware-independent controller core.
*   **Proposed Changes**:
    *   Add `label string` to `pedalSpec`.
    *   In `setup`, `change`, `handleEvent` (debounce debug line) and the
        programmed / read-back log lines, replace the literal `"pedal %s"`
        prefix plus `p.name` with `"%s"` plus `p.label`. Affected lines:
        the four `setup` errors and the modifier warning that start with
        `pedal %s (slot %d)`, the `programmed pedal %s (slot %d)` info line,
        the `pedal %s (slot %d) reads back as` warning, the debounce debug
        line, and the `pedal %s pressed` / `pedal %s released` info lines.
    *   Leave the `pedals %s and %s both send` error and `list_pedals`'s
        `"name"` field on `p.name`.
*   **Why**: with a pedal named `pedal`, the current format prints "pedal
    pedal pressed". Labels keep the 3-pedal output identical and make the
    single pedal's read naturally.

#### 4. `./discovery.go` (Modify)

*   **Type / Scope**: Go, discovery service.
*   **Proposed Changes**: `suggestConfigs` appends two configs per device, in
    this order: the existing `footswitch-<vendor>-<product>-<port>` with
    `Pcsensor3Pedal`, then `footswitch-1-pedal-<vendor>-<product>-<port>`
    with `Pcsensor1Pedal`, both with `device_path` set to `dev.stablePath()`
    and no pedal blocks. Update the function comment and the
    `DiscoverResources` comment to say both models are suggested because the
    device cannot report its pedal count.
*   **Why**: Decision D1.

#### 5. `./cmd/module/main.go` (Modify)

*   **Type / Scope**: Go, module entry point.
*   **Proposed Changes**: add
    `resource.APIModel{API: input.API, Model: footswitch.Pcsensor1Pedal}`
    to `module.ModularMain`.
*   **Why**: `viam-server` only serves registered models.

#### 6. `./cmd/cli/main.go` (Modify)

*   **Type / Scope**: Go, local test CLI (Linux only).
*   **Proposed Changes**: add a `-pedals` integer flag (default 3). With 3,
    behave as today. With 1, call `NewPcsensor1Pedal` with
    `&footswitch.OnePedalConfig{}`. Any other value exits with a usage error.
    Update the file comment and the README's development section to show
    `go run ./cmd/cli -pedals 1`.
*   **Why**: the single-pedal path must be exercisable on a Linux box without
    `viam-server` for hardware verification.

#### 7. `./meta.json` (Modify)

*   **Type / Scope**: Viam module manifest.
*   **Proposed Changes**: add a model entry
    `{"api": "rdk:component:input_controller", "model": "clint:footswitch:pcsensor-1-pedal", "short_description": "iKKEGOL / PCsensor single-pedal USB footswitch as a button; reads and programs its key", "markdown_link": "clint_footswitch_pcsensor-1-pedal.md"}`
    after the 3-pedal entry. Change the module `description` to say "1- and
    3-pedal" instead of implying 3-pedal only, and the discovery model's
    `short_description` to "suggests pcsensor-3-pedal and pcsensor-1-pedal
    configs".
*   **Why**: the registry lists models from this file.

#### 8. `./clint_footswitch_pcsensor-1-pedal.md` (Create)

*   **Type / Scope**: Registry documentation for the new model.
*   **Proposed Changes**: mirror the 3-pedal doc's structure: supported
    hardware (same USB IDs; `3553:b001` tested with the author's unit; a note
    that the device is identical to the 3-pedal unit over USB, so the user
    must pick this model by knowing their hardware), configuration table
    (`pedal`, `slot`, `device_path`, `usb_id`, `debounce_ms`), events,
    `DoCommand` with the `list_pedals` example from the Technical Plan, and
    a line that `5131:2019` single-pedal units are not supported.
*   **Why**: `meta.json` links to it and it is the only place the `slot`
    attribute is explained.

#### 9. `./clint_footswitch_pcsensor-3-pedal.md` (Modify)

*   **Type / Scope**: Registry documentation.
*   **Proposed Changes**: replace "Not supported yet: 1- and 2-pedal units
    (including `5131:2019`)" with a pointer to the 1-pedal doc and a warning
    that a single-pedal unit looks identical and will start under this model
    with two phantom pedals, so single-pedal owners should use
    `pcsensor-1-pedal`. Keep the 2-pedal and `5131:2019` exclusions.
*   **Why**: this is the doc a single-pedal owner will land on first.

#### 10. `./clint_footswitch_discovery.md` (Modify)

*   **Type / Scope**: Registry documentation.
*   **Proposed Changes**: say that each attached pedal produces two
    suggestions, `footswitch-…` for 3-pedal units and `footswitch-1-pedal-…`
    for single-pedal units, that the user adds the one matching their
    hardware, and that adding both for one device makes the second fail to
    grab the pedal.
*   **Why**: Decision D1 changes what the Test panel shows.

#### 11. `./README.md` (Modify)

*   **Type / Scope**: Module README.
*   **Proposed Changes**: first paragraph and keywords mention 1- and 3-pedal
    units; add the `pcsensor-1-pedal` model to the Models list; quick start
    step 2 says to pick the suggestion matching the pedal count and that
    single-pedal users adding by hand should use `pcsensor-1-pedal`; the
    example log lines gain a single-pedal example `pedal pressed (slot 2,
    KEY_B)`; the development section shows `go run ./cmd/cli -pedals 1`.
*   **Why**: the README is the registry landing page.

#### 12. `./config_test.go` (Modify)

*   **Type / Scope**: Unit tests.
*   **Proposed Changes**:
    *   `TestConvertOnePedalConfigRejectsUnknownKeys`: accepts
        `{"pedal": {"keycode": "KEY_F14"}, "slot": 2, "debounce_ms": 20}` and
        `nil`; rejects `{"left": {}}`, `{"pedal": {"key": "x"}}` and
        `{"slot": "two"}`.
    *   `TestValidateOnePedal`: table like `TestValidate` with cases: `{}` ok;
        slot 1 and 3 ok; slot 0 and 4 contain "slot"; bad keycode contains
        "pedal.keycode"; modifier contains "modifier"; bad control contains
        "pedal.control"; bad `usb_id` contains "usb_id"; zero `debounce_ms`
        contains "debounce_ms". Asserts no dependencies.
    *   `TestLayouts`: `threePedalLayout` resolves to labels `pedal left`,
        `pedal center`, `pedal right` on slots 1-3 with the existing default
        controls; `OnePedalConfig{}` resolves to one spec with label `pedal`,
        slot 2, `ButtonSouth`; with `Slot: 3` the spec's slot is 3.
*   **Why**: validation and layout are the new model's whole surface.

#### 13. `./core_test.go` (Modify)

*   **Type / Scope**: Unit tests.
*   **Proposed Changes**:
    *   Split `newHarness` so the core is built from `model string` and
        `specs []pedalSpec`: add `newHarnessSpecs(t, model, specs, slots, debounce)`
        and make the existing `newHarness` call it with
        `"pcsensor-3-pedal"` and `defaultSpecs(t, cfg)`, so existing tests are
        untouched.
    *   `TestOnePedalReadsSlot2`: factory slots `{a, b, c}`; `OnePedalConfig{}`;
        expect one `Connect` on `ButtonSouth`; `controls()` has length 1; key
        code 48 (`KEY_B`) down and up produce `ButtonPress` then
        `ButtonRelease` on `ButtonSouth`; key code 30 (`KEY_A`) produces no
        event (`h.none()`); `listPedals()` reports `model` `pcsensor-1-pedal`
        and one pedal with `slot` 2, `keycode` `KEY_B`, `keycode_source`
        `device`; `fakeSlots.writes` is empty.
    *   `TestOnePedalProgramsOnlyItsSlot`: factory slots; `Pedal.Keycode`
        `KEY_F14`; expect exactly one `writeSlots` call whose slots are
        `{keySetting(a), keySetting(f14), keySetting(c)}`; presses on `KEY_F14`
        (code 184) are reported and `KEY_B` is ignored.
    *   `TestOnePedalSlotOverride`: `Slot: 3`; presses on `KEY_C` (code 46)
        are reported, `KEY_B` is ignored, `list_pedals` shows `slot` 3.
    *   `TestOnePedalSetupErrors`: slot 2 in mouse mode with no keycode fails
        with a message containing "pedal (slot 2) is in mouse mode"; slot 1
        in mouse mode with slot 2 a plain key starts fine (unused slots never
        block).
*   **Why**: proves the core handles a one-spec list, that unused slots are
    neither consulted nor rewritten with new values, and that the label
    appears in errors.

#### 14. `./sysfs_test.go` (Modify)

*   **Type / Scope**: Unit tests.
*   **Proposed Changes**: in `TestScanFindsKeyboardInterface`, assert
    `suggestConfigs(devs)` returns two configs: index 0 unchanged
    (`footswitch-3553-b001-6`, `Pcsensor3Pedal`), index 1 named
    `footswitch-1-pedal-3553-b001-6` with `Pcsensor1Pedal` and the same
    `device_path`.
*   **Why**: Decision D1's only code change.

#### 15. `./onepedal_test.go` (Create)

*   **Type / Scope**: Unit tests.
*   **Proposed Changes**: `TestNewPcsensor1PedalOffLinux`-style construction
    test is not possible without a device, so this file holds only a
    `TestPcsensor1PedalRegistered` check that `resource.LookupRegistration(input.API, Pcsensor1Pedal)`
    finds the model with a non-nil constructor and converter, matching the
    pattern that would catch a missing `init`.
*   **Why**: the registration is the one line that, if forgotten, fails only
    at runtime inside `viam-server`.

### Files deliberately not touched

*   `./pcsensor.go`, `./device.go`, `./sysfs.go`, `./keys.go`, `./grab_*.go`,
    `./actions.go`: no change needed; the Technical Plan explains why.
*   `./Makefile`, `./.github/workflows/*.yml`, `./go.mod`: no new
    dependencies, build steps or CI changes.
*   `iKKEGOL 3-Pedal Footswitch — Viam Input Controller Module Spec.md`: a
    local, gitignored file; its "Later: 1- and 2-pedal units" section is
    superseded by this design but the file is not edited.

### Verification Plan

*   **Compilation/Build**: `make module.tar.gz` with
    `VIAM_BUILD_OS=linux VIAM_BUILD_ARCH=arm64` and again with `amd64`; both
    must build on the author's Mac. `go build ./...` on macOS must still pass
    (the `grab_other.go` stub keeps it compiling).
*   **Unit Tests**: `make test` (`go test ./...`) on macOS and in CI. All
    existing tests pass unchanged; the new tests in files 12-15 pass.
*   **Static Analysis & Linting**: `gofmt -l .` prints nothing;
    `go vet ./...` is clean.
*   **Test Coverage Requirement**: every new branch in `onepedal.go` and
    `resolveSpecs` is reached by a test in files 12-13; the `-cover` output
    for `onepedal.go` must not show an unreached `Validate` branch.
*   **Hardware (Linux, author's single-pedal unit)**, in this order:
    1.  `go run ./cmd/cli -pedals 1` with factory settings: logs
        `pedal pressed (slot 2, KEY_B)` on a press and nothing for other keys.
    2.  With `viam-server`: add the discovery service; the Test panel lists
        two suggestions for the device; add the `footswitch-1-pedal-…` one;
        `list_pedals` shows one pedal, slot 2, `KEY_B`, source `device`.
    3.  Set `"pedal": {"keycode": "KEY_F14"}`; the log shows
        `programmed pedal (slot 2) to send KEY_F14`; presses report; the
        upstream `footswitch -r` on the Linux box shows slots `a`, `f14`,
        `c`.
    4.  Add the actions service with `on_press` and `on_release` on
        `ButtonSouth`; hold the pedal, unplug it; `on_release` runs.
    5.  Add both suggestions for the same device; the second fails with the
        existing grab error. Remove one.
*   **Hardware (3-pedal unit, if available)**: the existing configs start,
    log and program exactly as before the change (CUJ 4). If no unit is at
    hand, the unchanged `core_test.go` tests and the byte-identical label
    strings stand in for it, and the release notes say so.

--------------------------------------------------------------------------------

<!-- Provenance: Indicates initial structural drafting via EGM. -->

*Drafted with EGM*
