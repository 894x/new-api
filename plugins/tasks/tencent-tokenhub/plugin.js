export const meta = {
  apiVersion: 1,
  key: "tencent-tokenhub",
  name: "Tencent TokenHub Video",
  version: "1.0.0",
  author: { name: "QuantumNous" },
  channelTypes: [100],
  models: [
    "hy-video-1.5",
    "yt-video-2.0",
    "yt-video-fx",
    "yt-video-humanactor",
    "kl-video-v3",
    "kl-video-v2-6",
    "kl-video-v2-5-turbo",
    "kl-video-v2-1-master",
    "kl-video-v2-1",
    "vd-video-q3-pro",
    "vd-video-q3-turbo",
  ],
  fetchMode: "per_task",
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }],
  usageSchema: {
    seconds: { type: "number", unit: "second", description: { en: "Requested or declared billing duration.", zh: "请求或声明的计费时长。" } },
    outputResolution: {
      enum: ["360p", "480p", "540p", "720p", "1080p", "4k", "other"],
      description: { en: "Normalized video resolution.", zh: "标准化视频分辨率。" },
    },
    audioEnabled: { type: "boolean", description: { en: "Whether audio generation is enabled.", zh: "是否开启音频生成。" } },
    specifiedVoice: { type: "boolean", description: { en: "Whether a voice is specified.", zh: "是否指定音色。" } },
  },
};

const durationKeys = ["duration", "seconds", "duration_seconds", "billing_duration_seconds"];

function trimmed(value) {
  return String(value || "").trim();
}

function metadataOf(req) {
  let value = req.metadata;
  if (typeof value === "string") value = value.trim() ? JSON.parse(value) : null;
  if (value == null) return {};
  if (typeof value !== "object" || Array.isArray(value)) throw new Error("metadata must be a JSON object");
  return value;
}

function durationValue(value) {
  if (typeof value !== "number" && (typeof value !== "string" || !/^[+]?\d+$/.test(value))) return 0;
  const number = Number(value);
  return Number.isSafeInteger(number) && number >= 1 && number <= 3600 ? number : 0;
}

function firstValue(req, metadata, keys, accept) {
  for (const values of [req, metadata]) {
    for (const key of keys) if (accept(values[key])) return values[key];
  }
  return undefined;
}

function durationOf(req, metadata) {
  return Number(firstValue(req, metadata, durationKeys, (value) => durationValue(value) > 0) || 0);
}

function validateRequest(req, model) {
  if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
  const metadata = metadataOf(req);
  for (const values of [req, metadata]) {
    for (const key of durationKeys) {
      if (values[key] != null && !durationValue(values[key])) throw new Error(key + " must be between 1 and 3600");
    }
  }
  if (model === "yt-video-humanactor" && !durationOf(req, metadata))
    throw new Error("billing_duration_seconds is required for yt-video-humanactor because billing follows the input audio duration");
  return metadata;
}

function hasImage(req, metadata) {
  return [req, metadata].some((values) =>
    ["image", "images", "image_url", "image_base64", "input_reference"].some((key) => {
      const value = values[key];
      return value != null && value !== "" && (!Array.isArray(value) || value.length > 0);
    })
  );
}

function stringValue(req, metadata, keys) {
  return firstValue(req, metadata, keys, (value) => typeof value === "string" && value.trim() !== "") || "";
}

function boolValue(req, metadata, keys) {
  const value = firstValue(req, metadata, keys, (candidate) => typeof candidate === "boolean" || typeof candidate === "string");
  return value === true || (typeof value === "string" && ["true", "1", "yes"].includes(value.toLowerCase()));
}

function resolutionOf(req, metadata, model) {
  const value = stringValue(req, metadata, ["resolution", "size"]).toLowerCase().trim().replace(/ /g, "");
  const aliases = {
    "3840x2160": "4k",
    "2160x3840": "4k",
    "2160p": "4k",
    "1920x1080": "1080p",
    "1080x1920": "1080p",
    "1280x720": "720p",
    "720x1280": "720p",
    "960x540": "540p",
    "540x960": "540p",
    "640x360": "360p",
    "360x640": "360p",
  };
  return aliases[value] || value || { "yt-video-2.0": "480p", "yt-video-fx": "360p", "yt-video-humanactor": "1080p" }[model] || "720p";
}

function variantRatio(model, resolution, audio, voice) {
  switch (model) {
    case "yt-video-2.0":
      return ["720p", "1080p"].includes(resolution) ? 2.5 : 1;
    case "yt-video-fx":
      return resolution === "720p" ? 2 : 1;
    case "yt-video-humanactor":
      return resolution === "1080p" ? 2 : 1;
    case "kl-video-v3":
      if (resolution === "4k") return 5;
      if (resolution === "1080p") return audio ? 2 : 4 / 3;
      return audio ? 1.5 : 1;
    case "kl-video-v2-6":
      if (resolution === "4k") return 10;
      if (resolution === "1080p") return audio ? (voice ? 4 : 10 / 3) : 5 / 3;
      return 1;
    case "kl-video-v2-5-turbo":
      return resolution === "1080p" ? 5 / 3 : 1;
    case "kl-video-v2-1":
      return resolution === "1080p" ? 1.75 : 1;
    case "vd-video-q3-pro":
      return resolution === "720p" ? 20 / 9 : resolution === "1080p" ? 8 / 3 : 1;
    case "vd-video-q3-turbo":
      return resolution === "720p" ? 12 / 7 : resolution === "1080p" ? 13 / 7 : 1;
    default:
      return 1;
  }
}

function normalizeImage(value) {
  if (typeof value !== "string") return value;
  const image = value.trim();
  if (image.startsWith("data:") && image.includes(",")) return { base64: image.slice(image.indexOf(",") + 1) };
  return { url: image };
}

export function buildSubmitRequest(ctx) {
  const req = ctx.requestBody || {};
  const model = ctx.upstreamModel || ctx.model;
  const metadata = validateRequest(req, model);
  const body = Object.assign({}, metadata, req, { model });
  delete body.metadata;
  delete body.id;
  if (!Object.prototype.hasOwnProperty.call(body, "resolution") && Object.prototype.hasOwnProperty.call(body, "size")) body.resolution = body.size;
  delete body.size;
  if (!Object.prototype.hasOwnProperty.call(body, "duration") && Object.prototype.hasOwnProperty.call(body, "seconds"))
    body.duration = durationValue(body.seconds);
  delete body.seconds;
  delete body.billing_duration_seconds;
  if (!Object.prototype.hasOwnProperty.call(body, "image") && Object.prototype.hasOwnProperty.call(body, "input_reference")) body.image = body.input_reference;
  if (Object.prototype.hasOwnProperty.call(body, "image")) body.image = normalizeImage(body.image);
  if (Array.isArray(body.images)) body.images = body.images.map(normalizeImage);
  delete body.input_reference;
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + "/v1/api/video/submit",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json" },
    body,
    action: hasImage(req, metadata) ? "image_to_video" : "text_to_video",
  };
}

export function extractUsage(ctx) {
  const req = ctx.requestBody || {};
  const model = ctx.upstreamModel || ctx.model;
  if (!meta.models.includes(model)) throw new Error("unsupported Tencent TokenHub video model: " + model);
  const metadata = validateRequest(req, model);
  const resolution = resolutionOf(req, metadata, model);
  const audio = boolValue(req, metadata, ["sound", "audio", "generate_audio", "with_audio"]);
  const voice = stringValue(req, metadata, ["voice_id", "voice", "voice_type"]) !== "";
  const seconds = durationOf(req, metadata) || 5;
  if (ctx.usagePurpose !== "billing_ratios")
    return {
      seconds,
      outputResolution: meta.usageSchema.outputResolution.enum.includes(resolution) ? resolution : "other",
      audioEnabled: audio,
      specifiedVoice: voice,
    };
  if (model === "hy-video-1.5") return null;
  const ratio = variantRatio(model, resolution, audio, voice);
  if (model === "yt-video-2.0" || model === "yt-video-fx") return { resolution: ratio };
  return { seconds, resolution: ratio };
}

export function extractUsageOnComplete() {
  return null;
}

export function parseSubmitResponse(_ctx, resp) {
  const body = resp.body || {};
  if (typeof body.id !== "string" || !body.id) throw new Error("Tencent TokenHub response is missing id");
  return { taskId: body.id, taskData: body };
}

export function buildQueryRequest(ctx) {
  const model = (ctx.requestBody && ctx.requestBody.model) || ctx.upstreamModel;
  if (!ctx.taskId || !model) throw new Error("task_id and model are required");
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + "/v1/api/video/query",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json" },
    body: { model, id: ctx.taskId },
  };
}

export function parseTaskResult(_ctx, body) {
  const statuses = {
    queued: "QUEUED",
    pending: "QUEUED",
    processing: "IN_PROGRESS",
    in_progress: "IN_PROGRESS",
    completed: "SUCCESS",
    failed: "FAILURE",
    cancelled: "FAILURE",
  };
  const status = statuses[body.status];
  if (!status) throw new Error("unknown Tencent TokenHub task status: " + body.status);
  const result = { status };
  if (Number.isFinite(body.progress) && body.progress > 0) result.progress = Math.min(Math.trunc(body.progress), 100) + "%";
  if (status === "SUCCESS") result.url = (body.data && body.data.url) || "";
  if (status === "FAILURE") result.reason = body.message || (body.error && (body.error.message || body.error.code)) || "Tencent TokenHub task failed";
  return result;
}

function videoURL(task) {
  let data = task.data || {};
  if (data.data && data.data.task_id && Object.prototype.hasOwnProperty.call(data.data, "data")) data = data.data.data || {};
  return (data.data && data.data.url) || task.resultUrl || "";
}

export function listArtifacts(task) {
  return task.status === "SUCCESS" && videoURL(task) ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  const url = videoURL(ctx);
  if (ctx.artifactKey !== "video" || !url) throw new Error("artifact_not_found");
  return { url, method: ctx.clientRequest.method, credentialless: true };
}

function decodeVideo(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const req = ctx.body.value;
  const metadata = validateRequest(req, ctx.model || req.model);
  return { kind: "submit", model: ctx.model || req.model, action: hasImage(req, metadata) ? "image_to_video" : "text_to_video", requestBody: req };
}

export const protocols = {
  openai_video: {
    decodeRequest: decodeVideo,
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      const output = {
        id: task.task_id,
        task_id: task.task_id,
        object: "video",
        model: (task.properties && task.properties.origin_model_name) || "",
        status: statuses[task.status] || "unknown",
        progress: Number(String(task.progress || "0").replace("%", "")),
        created_at: task.created_at,
        completed_at: task.finish_time || 0,
      };
      if (task.status === "FAILURE") output.error = { code: "task_failed", message: task.fail_reason || "Tencent TokenHub task failed" };
      return output;
    },
  },
};
function responsesInput(req) {
  const texts = [],
    images = [];
  const input = req.input;
  if (typeof input === "string") texts.push(input);
  else if (Array.isArray(input)) {
    for (const item of input) {
      if (typeof item === "string") {
        texts.push(item);
        continue;
      }
      if (!item || typeof item !== "object" || Array.isArray(item)) continue;
      const content = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
      for (const part of content) {
        if (typeof part === "string") {
          texts.push(part);
          continue;
        }
        if (!part || typeof part !== "object" || Array.isArray(part)) continue;
        if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
        if (["input_image", "image_url"].includes(part.type)) {
          let image = part.image_url;
          if (image && typeof image === "object") image = image.url;
          if (trimmed(image)) images.push(trimmed(image));
        }
      }
    }
  }
  return {
    prompt: texts
      .filter(function (text) {
        return trimmed(text);
      })
      .join("\n"),
    images: images,
  };
}

function responsesVideoText(ctx) {
  const artifact = ctx && ctx.artifacts && ctx.artifacts.video;
  const url = trimmed(artifact && artifact.url);
  if (!url) throw new Error("video artifact is unavailable");
  const escaped = url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

protocols.openai_responses = {
  decodeRequest: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
    const req = ctx.body.value;
    if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
    if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
    const input = responsesInput(req);
    const request = Object.assign({}, req);
    for (const key of ["input", "stream", "background", "store", "instructions"]) delete request[key];
    if (input.prompt) request.prompt = input.prompt;
    if (input.images.length) request.images = (Array.isArray(request.images) ? request.images : []).concat(input.images);
    return decodeVideo({ model: ctx.model || req.model, body: { kind: "json", value: request } });
  },
  renderEvents: function (ctx, task, previousState) {
    const status = String(task.status || "UNKNOWN").toUpperCase();
    const value = Number(String(task.progress || "").replace("%", ""));
    const progress = Number.isFinite(value) && value >= 0 && value <= 100 ? value : null;
    const state = { status: status, progress: progress };
    if (status === "SUCCESS") {
      const text = responsesVideoText(ctx);
      const events = previousState && previousState.status === status ? [] : [{ type: "output", data: text }];
      return { events: events, state: state, done: true };
    }
    if (status === "FAILURE") return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "task failed" }], state: state, done: true };
    if (previousState && previousState.status === status && previousState.progress === progress) return { events: [], state: state, done: false };
    const event = { type: "progress", message: status.toLowerCase() };
    if (progress !== null) event.progress = progress;
    return { events: [event], state: state, done: false };
  },
  renderFinal: function (ctx, _task) {
    return {
      output: [
        {
          type: "message",
          status: "completed",
          role: "assistant",
          content: [{ type: "output_text", text: responsesVideoText(ctx), annotations: [], logprobs: [] }],
        },
      ],
      metadata: { vendor: "tencent-tokenhub" },
    };
  },
};
