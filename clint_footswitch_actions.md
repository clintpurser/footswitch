# Model `clint:footswitch:actions`

A generic service that listens to any input controller (the pedals, or a gamepad) and calls `do_command` on another resource when a control is pressed or released.

## Configuration

```json
{
  "input_controller": "pedals",
  "controls": {
    "ButtonLT":     { "on_press": { "component": "arm", "method": "do_command", "args": { "cmd": "stop" } } },
    "ButtonSelect": { "on_press":   { "component": "gripper", "method": "do_command", "args": { "cmd": "close" } },
                      "on_release": { "component": "gripper", "method": "do_command", "args": { "cmd": "open" } } }
  }
}
```

| Attribute | Type | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `input_controller` | string | Yes | | Name of the input controller to listen to. |
| `controls` | map of control name to `{on_press, on_release}` | No | `{}` | Actions per Viam control name. |
| `action_timeout_ms` | integer | No | 5000 | Longer actions are cancelled and logged. |

Each action has `component` (any resource on the machine), `method` (`do_command` only) and optional `args` (the DoCommand payload).

## Behavior

- Each control has its own queue (16 actions) and worker, so a slow resource never delays another control, and a release never overtakes its press. When a queue is full, new actions are dropped and logged.
- Every action is logged with its control, event, resource and duration. Failures, timeouts and drops are logged as errors. Nothing is retried.
- Releases sent when the pedal is unplugged or closed run `on_release` as usual, so hold-to-run actions still stop.
