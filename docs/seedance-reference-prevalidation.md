# Seedance reference media prevalidation

Seedance SLS validates the final, mapped and parameter-overridden request before billing or asset synchronization. Built-in rules apply to the `doubao-seedance-2-0` and `doubao-seedance-2-5` families (including dated suffixes and dot/underscore aliases). Other models retain their existing behavior.

| Rule | Seedance 2.0 | Seedance 2.5 |
| --- | --- | --- |
| Reference audio, each | 2–15 seconds | 2–30 seconds |
| Reference audio count / total duration | 3 / 15 seconds | 10 / 30 seconds |
| Reference video, each | 2–15 seconds | 2–30 seconds |
| Reference video count / total duration | 3 / 15 seconds | 10 / 30 seconds |
| Reference image count | 9 | 30 |
| Audio without image/video | Rejected | Accepted |

Audio and video totals are independent. Repeated references count each occurrence toward both count and duration limits. Structural/count checks run before any media downloads. A first-frame request accepts one `first_frame` image and an optional single `last_frame` image; it cannot mix those with reference media. Seedance 2.5 first/last-frame requests accept only `adaptive` when `ratio` is supplied.

Audio URLs and Base64 data URIs are inspected as WAV/MP3, up to 15 MiB. Video references accept HTTP(S) URLs or account asset IDs, with MP4/MOV containers, up to 200 MiB, dimensions 300–6000, aspect ratio 0.4–2.5, pixels 407696–8295044 and FPS 24–60. Downloads reuse the existing SSRF-protected, bounded media reader. Errors identify the original content index, for example `content[2].audio_url: total reference audio duration ...`.

Logical assets must belong to the requesting account and match the reference type. Complete stored metadata is reused; legacy records without complete metadata return an explicit re-import-required error, because their current source URL cannot prove the duration of an existing replica. Direct media is inspected regardless of whether the channel asset library is enabled. Verified metadata is cached only in the request context, reused across routing retries and automatic import, and model limits are rechecked on each validation. If a URL now describes different metadata from an older imported asset, import creates a new asset rather than submitting the stale clip.

After successful model validation, automatic import can accept the model's combined reference count (15 for 2.0, 50 for 2.5), replacing the unrelated default limit of eight for that request. Asset-library admission still enforces its own existing media limits, including its narrower video pixel range; cached metadata does not bypass library admission.

Scope: this change implements reference duration, count, documented frame combinations and the existing media inspection capabilities. It does not inspect video codecs, detect moderation/person-face issues, or add image-content inspection to generation. The supplied general guide does not define the wire-level discriminator for 2.5 editing/extending; therefore edit-specific minimum input duration (4 seconds), output duration (-1), and edit/extend ratio constraints require the task-type API schema before implementation. Do not infer editing from prompt text.

Source: the official Seedance general guide pasted by the user, sections “图片要求”, “视频要求”, “音频要求” and the Seedance 2.5 parameter notes. Existing asset-library admission rules remain a separate contract.
