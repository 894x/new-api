# Built-in task plugins

Each provider lives in `<plugin-key>/plugin.js`. The embedded loader registers
those directories as factory plugins in the shared JavaScript runtime. Files at
this directory's root are documentation, not providers.

Keep provider request/response and usage contracts with that provider's tests.
Host routing, credentials, asset ownership, quota conversion, and settlement
remain gateway responsibilities. Adding an embedded implementation does not
by itself replace a legacy channel route; host activation and removal of the
corresponding legacy driver must be verified separately.
