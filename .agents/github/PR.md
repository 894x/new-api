<!--
Agent evidence supplement. Fill the main `.github/PULL_REQUEST_TEMPLATE.md`
first and append this checklist; do not replace the main template.

Keep every heading. If a section does not apply, write why; do not delete it.
Match the user's language in the filled answers. Do not invent facts.

User request: quote the request the user made to the agent as faithfully as
possible. Keep the original language and line breaks. Do not rewrite,
summarize, translate, or turn it into a PR description.

Keep the body short and factual. Do not paste unfiltered AI-generated text
in the PR body or in later comments. Repeated submissions of this kind may
result in a block. Verification must be commands actually run and what was
observed. Stating only that `go build` or tests passed is not proof.

Upstream new features: link an issue; if none exists, ask for authorization to
file one with `.agents/github/ISSUE.md`. Never publish just to satisfy a template.
Large or directional upstream changes require maintainer agreement on that issue.

If this PR fixes a bug and the linked issue is missing actual behavior, impact,
frequency, evidence that the problem is in new-api, or the applicable
relay / billing / frontend / deployment items, ask the user those questions
and wait. Ask for the facts. Do not tell the user to confirm a template.

Before opening an upstream PR, check the same out-of-scope list as `.agents/github/ISSUE.md`
(Coding Plan, reverse-engineered channels, third-party wrappers, Codex reverse-proxy
compatibility, pass-through-only forwarding, third-party hosts, usage questions).
Tell the user and do not open an upstream PR. This does not prohibit local
maintenance of existing fork features or an authorized PR to the fork.
Keep PR text concise and evidence-based. Record executed commands and observed results; do not assert verification or review that did not occur.

For an upstream contribution, search https://docs.newapi.ai/ , https://deepwiki.com/QuantumNous/new-api ,
the README, and the code. If this is a usage, configuration, or integration
question, answer the user and do not open an upstream PR.
Only claim actions actually performed. Mark unknown tool/model versions as
unavailable rather than guessing. Do not assert human review on the user's behalf.
-->

## Agent

- Tool:
- Tool version:
- Model (full id):
- Host (CLI / IDE / GitHub coding agent / other):
- Date (UTC):

## Links

- Closes #
- Related:

## User request

Quote the request the user made to the agent as faithfully as possible.
Keep the original language and line breaks. Do not rewrite, summarize,
translate, or turn it into a PR description.

- Verbatim:
- Later constraints or corrections from the user (quote, or none):

## Upstream contribution scope

For an upstream PR, if the change matches an item below, explain the upstream
acceptance policy and **do not open an upstream PR**. For a fork PR, state the
authorized fork scope; these restrictions do not remove existing fork features.

- Coding Plan
- Reverse-engineered channels
- Third-party API wrappers
- Codex channel-type changes, or compatibility from exposing Codex as a general-purpose API
- Codex API-specific protocol or behavior treated as standard OpenAI API behavior
- Pass-through-only forwarding (pass-through sends content as-is and does not go through new-api processing logic)
- Third-party hosting sites, relay services, or API services
- Usage, configuration, or integration (answer from docs and code instead)

- Matched: yes/no
- Target repository (upstream / fork):
- If matched for upstream, what was told to the user (stop here; do not open an upstream PR):

## Open gate — do not open unless all are satisfied

If any item fails, tell the user and **stop**. Do not open the PR.

- Out of scope (including pass-through-only forwarding): no
- Usage / configuration / integration (answered instead of opening): no
- Required issue facts present without invention: yes
- Verification is actual commands or steps and observed results, not only `go build` or tests passed: yes
- Body is short and factual; no unfiltered AI-generated text: yes
- Open: yes/no
- If no, what was told to the user (stop here):

## Kind

- [ ] Bug fix
- [ ] New feature
- [ ] Performance / refactor
- [ ] Docs
- [ ] Other:

## Issue facts

Take these from the linked issue. If a needed item is empty, ask the user that question.

- Actual behavior:
- Impact:
- Frequency:
- Evidence that the problem is in new-api rather than the client or upstream:
- Applicable types and their fields (relay / billing / frontend / deployment; write "not applicable" otherwise):

## Change

(what changed, why it works, grounded in the code actually touched.
Short and factual; do not paste unfiltered AI-generated text.)

## Research

### Duplicate / prior art

- Search queries (issues, PRs):
- What already existed and why this is not a duplicate:

### Docs and code

Open them. Do not write "already checked" without sources.

- https://docs.newapi.ai/ :
- https://deepwiki.com/QuantumNous/new-api :
- README / repo docs:
- Code paths and what they imply for this change:

### Alternatives considered

- Option A:
- Option B:
- Why this approach:

## Files

| Path | Why |
| --- | --- |
|  |  |

## Behavior

- Before:
- After:
- Explicit non-goals / leftover work:

## Verification

Only what was actually run. Do not state only that `go build` or tests passed.
Each item needs the command or steps and the observed result.

- Commands and results:
- Manual steps and observed result:
- UI: screenshot or recording (or why none):
- Tests added or updated, or why none:
- Databases / providers / platforms exercised:
- Not verified:

## Risks

- Failure modes:
- Billing / quota / auth impact:
- Follow-ups:

## Scope check

- Single focused change: yes/no (if no, why):
- Secrets included: no
- Upstream scope matched (or not applicable for an authorized fork PR):
