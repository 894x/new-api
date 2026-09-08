# Seedance parameter capability templates

Channel creation and editing share the **Model Parameter Capabilities → Built-in templates** tab. Select a Seedance version, enter the exact upstream model name **after model mapping**, review the resulting configuration, and apply it. Save the capability editor and then the channel form to persist it.

Templates are copied into ordinary exact-model rules. Existing effective constraints are preserved by default, including constraints inherited from channel defaults or model patterns. The replacement checkbox replaces the template parameters in the exact rule; unrelated parameters remain intact. Review inherited constraints in the effective preview, especially if unusual numeric bounds were configured on enum parameters. Reapplying to the same exact model updates its last matching rule instead of adding duplicates. Saved channels do not track future template updates.

## Included rules

| Template | Duration | Resolution |
| --- | --- | --- |
| Seedance 2.0 | -1, or integer 4–15 | 480p, 720p, 1080p, 4k |
| Seedance 2.0 Fast / Mini | -1, or integer 4–15 | 480p, 720p |
| Seedance 2.5 | -1, or integer 4–30 | 480p, 720p, 1080p |

All templates include the documented basic ratio enum and `service_tier=default`. Violations are rejected. Channel-selection participation defaults to false for newly copied constraints and can be edited separately. Existing participation settings are retained when preserving a parameter.

`duration` combines numeric bounds with an explicit allowed-value list. To allow only automatic duration, keep `-1` in allowed values and ensure the minimum does not exclude it. Missing optional parameters remain absent. This does not validate mode-specific combinations (for example, Seedance 2.5 editing requires automatic duration and adaptive ratio), media properties, or aggregate media limits.

## Seedance SLS enforcement

For SLS, the mapped-request validator builds the canonical upstream payload, applies parameter overrides followed by parameter capabilities, and validates the duration safety bound before quota reservation or asset synchronization. Billing inspection and upstream serialization use that prepared payload, so overrides are applied once per routing attempt. Model changes must use channel model mapping.

Native Seedance JSON requests support the `duration=-1` sentinel; channel capabilities can permit or reject it. The generic video request validator retains its existing duration contract. SLS pre-consumption retains the existing base-price/video-input pricing; duration is not multiplied into the pre-charge, and reported completion tokens continue through existing task settlement.

Validation failures return local HTTP 400 task errors with the offending parameter in the message. This change does not submit a task to the provider on validation failure.

Template revision: **2026-09-08**, based on the user-provided copy of the [official Seedance guide](https://ark.volcengine.com/region:cn-beijing/docs/82379/2298881?lang=zh). Provider aliases and model support must match the selected channel.
