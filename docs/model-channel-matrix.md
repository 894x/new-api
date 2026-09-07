# Model-channel matrix

Open **Admin → Model-channel matrix** to compare configured models across
channels. Each public model name is a row and each channel is a column. The
matrix has its own sidebar entry and its own switch under sidebar modules.
Channel read permission is required to view the page; channel write permission
is required to edit routing overrides.

Cells show effective RPM, TPM, priority and weight, with inheritance or override
marked for each field. An unconfigured pair is shown as **Not configured**.
Click a configured cell to inspect its upstream model and edit the four values:

- Leave a field empty to inherit that channel's default.
- RPM/TPM `0` explicitly means unlimited. Priority/weight `0` is an explicit
  routing value, not inheritance. Weight `0` still participates in selection.
- **Reset to channel defaults** clears all four overrides after saving.
- Saving affects only the selected model on the selected channel and uses the
  existing routing override API, validation, transaction, audit and cache refresh.

Enabled channels are shown by default. **All** includes disabled channels, whose
configuration remains inspectable and editable. Search by model, channel name or
channel ID and optionally filter an exact group. Rows come from actual channel
model configuration; a metadata entry or explicit override is not required.
Different public aliases remain separate rows even when mapped to one upstream
model. Group filtering does not create separate limits for that group.

RPM/TPM use the existing [channel-model capacity policy](channel-model-capacity.md).
Counters are shared across users and groups for each channel and public model;
they do not apply to async task relays or Realtime. The matrix displays configured
limits, not live usage or remaining capacity.

`GET /api/channel/model-matrix` provides the aggregate read model. It accepts
`model`, `channel`, `group`, `status=enabled|all`, `model_page`,
`model_page_size` (default 25, maximum 100), `channel_page` and
`channel_page_size` (default 10, maximum 50). Model and channel axes are paginated
independently, with stable model-name and channel-ID ordering. Only configured
cells in the visible cross-section are returned, together with filtered totals
and group options. Channel credentials and other private configuration are not
included. No new database tables or relay behavior are introduced.
