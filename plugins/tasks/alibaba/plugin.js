export const meta = {
  apiVersion: 1,
  key: "alibaba",
  name: "Alibaba Bailian",
  icon: "Bailian.Color",
  description: {
    en: "Alibaba Cloud Bailian Wanxiang video generation (text-to-video and image-to-video)",
    zh: "阿里云百炼万相视频生成（文生视频、图生视频）",
  },
  version: "1.0.0",
  author: { name: "QuantumNous" },
  channelTypes: [17],
  models: [
    "wan3.0-video",
    "wan3.0-video-prime",
    "wan2.7-i2v",
    "wan2.7-t2v",
    "wan2.5-t2v-preview",
    "wan2.5-i2v-preview",
    "wan2.2-i2v-flash",
    "wan2.2-i2v-plus",
    "wanx2.1-i2v-plus",
    "wanx2.1-i2v-turbo",
  ],
  fetchMode: "per_task",
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Requested video duration in seconds.", zh: "请求的视频时长，单位为秒。" },
    },
    resolution: {
      enum: ["480P", "720P", "1080P"],
      description: { en: "Requested output video resolution.", zh: "请求的输出视频分辨率。" },
    },
  },
  routes: [
    { method: "POST", path: "/ali/api/v1/services/aigc/video-generation/video-synthesis", type: "submit", decode: "createVideoTask", render: "taskCreated" },
    { method: "GET", path: "/ali/api/v1/tasks/:task_id", type: "query", render: "taskStatus" },
  ],
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_video"],
};

function trimmed(value) {
  return String(value || "").trim();
}

function firstImage(req) {
  if (trimmed(req.image)) return trimmed(req.image);
  for (const image of req.images || []) if (trimmed(image)) return trimmed(image);
  return trimmed(req.input_reference);
}

function secondImage(req) {
  let count = 0;
  for (const image of req.images || []) {
    if (!trimmed(image)) continue;
    count++;
    if (count === 2) return trimmed(image);
  }
  return "";
}

function normalizeResolution(value) {
  let resolution = String(value || "").toUpperCase();
  if (!resolution.endsWith("P")) resolution += "P";
  return resolution;
}

function isWan3(model) {
  return model === "wan3.0-video" || model === "wan3.0-video-prime";
}

function validateNative(body, model, allowUnresolvedSmart) {
  if (!body || typeof body !== "object" || Array.isArray(body) || !trimmed(body.model)) throw new Error("model field is required");
  const input = body.input;
  if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("input must be an object");
  if (input.prompt !== undefined && typeof input.prompt !== "string") throw new Error("input.prompt must be a string");
  if (input.media !== undefined && !Array.isArray(input.media)) throw new Error("input.media must be an array");
  if (!trimmed(input.prompt) && !(input.media || []).length && !trimmed(input.img_url)) throw new Error("input.prompt and input.media cannot both be empty");
  for (const media of input.media || []) {
    if (!media || typeof media.type !== "string" || !media.type.trim() || typeof media.url !== "string" || !media.url.trim())
      throw new Error("each input.media item requires type and url");
  }
  const parameters = body.parameters;
  if (parameters === undefined || parameters === null) return;
  if (typeof parameters !== "object" || Array.isArray(parameters)) throw new Error("parameters must be an object");
  const duration = parameters.duration;
  if (duration !== undefined && duration !== null) {
    const smart = duration === -1 && (isWan3(model) || allowUnresolvedSmart);
    if (
      typeof duration !== "number" ||
      !Number.isInteger(duration) ||
      (!smart && (duration < (isWan3(model) ? 2 : 1) || duration > (isWan3(model) ? 30 : 3600)))
    )
      throw new Error(isWan3(model) ? "parameters.duration must be -1 or between 2 and 30" : "parameters.duration must be between 1 and 3600");
  }
  for (const key of ["prompt_extend", "watermark", "audio"])
    if (parameters[key] != null && typeof parameters[key] !== "boolean") throw new Error("parameters." + key + " must be a boolean");
  if (!isWan3(model)) return;
  if (parameters.resolution != null && !["480P", "720P", "1080P"].includes(parameters.resolution))
    throw new Error("parameters.resolution must be 480P, 720P, or 1080P");
  if (parameters.ratio != null && !["adaptive", "16:9", "4:3", "1:1", "3:4", "9:16"].includes(parameters.ratio)) throw new Error("parameters.ratio is invalid");
  if (parameters.seed != null && (!Number.isInteger(parameters.seed) || parameters.seed < -1 || parameters.seed > 2147483647))
    throw new Error("parameters.seed must be -1 or between 0 and 2147483647");
}

// Smart duration is a provider mode, not a negative billing quantity. Decoders
// normalize it before the host's generic multiplier checks; the wire builder
// restores the sentinel only after model-specific validation.
function normalizeSmartDuration(request) {
  const req = Object.assign({}, request);
  delete req.smartDuration;
  if (req.nativeRequest) {
    req.nativeRequest = Object.assign({}, req.nativeRequest);
    if (req.nativeRequest.parameters && req.nativeRequest.parameters.duration === -1) {
      req.nativeRequest.parameters = Object.assign({}, req.nativeRequest.parameters);
      delete req.nativeRequest.parameters.duration;
      req.smartDuration = true;
    }
    return req;
  }
  let metadata = req.metadata || {};
  if (typeof metadata === "string") metadata = JSON.parse(metadata);
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be an object");
  if (req.metadata !== undefined) req.metadata = Object.assign({}, metadata);
  if (metadata.parameters && metadata.parameters.duration === -1) {
    req.metadata.parameters = Object.assign({}, metadata.parameters);
    delete req.metadata.parameters.duration;
    req.smartDuration = true;
  } else if ((req.duration === -1 || req.seconds === -1 || req.seconds === "-1") && (!metadata.parameters || metadata.parameters.duration === undefined)) {
    req.smartDuration = true;
  }
  if (req.duration === -1) delete req.duration;
  if (req.seconds === -1 || req.seconds === "-1") delete req.seconds;
  return req;
}

function convert(ctx) {
  const req = ctx.requestBody;
  const upstreamModel = ctx.upstreamModel || req.model;
  const wan3 = isWan3(upstreamModel) || (isWan3(ctx.model) && !meta.models.includes(upstreamModel));
  const validationModel = wan3 ? "wan3.0-video" : upstreamModel;
  const allowUnresolvedSmart = !ctx.modelMappingResolved && !meta.models.includes(upstreamModel);
  if (req.nativeRequest) {
    const body = Object.assign({}, req.nativeRequest, { model: upstreamModel });
    if (req.smartDuration) body.parameters = Object.assign({}, body.parameters || {}, { duration: -1 });
    validateNative(body, validationModel, allowUnresolvedSmart);
    return body;
  }
  const input = { prompt: req.prompt || "" };
  const image = firstImage(req);
  if (image) input.img_url = image;
  const parameters = { prompt_extend: true, watermark: false, duration: 5 };
  if (wan3) Object.assign(parameters, { audio: true, ratio: "adaptive" });

  if (req.size) {
    if (String(upstreamModel).includes("t2v") && !String(req.size).includes("*")) throw new Error("invalid size: " + req.size + ", example: 1920*1080");
    if (String(req.size).includes("*")) parameters.size = req.size;
    else parameters.resolution = normalizeResolution(req.size);
  } else if (wan3) {
    parameters.resolution = "1080P";
  } else if (String(upstreamModel).includes("t2v")) {
    parameters.size = String(upstreamModel).startsWith("wan2.5") || String(upstreamModel).startsWith("wan2.2") ? "1920*1080" : "1280*720";
  } else if (String(upstreamModel).startsWith("wan2.6") || String(upstreamModel).startsWith("wan2.5") || String(upstreamModel).startsWith("wan2.2-i2v-plus")) {
    parameters.resolution = "1080P";
  } else {
    parameters.resolution = "720P";
  }

  if (Number(req.duration) > 0) parameters.duration = Number(req.duration);
  else if (req.seconds) {
    const seconds = Number(req.seconds);
    if (!Number.isInteger(seconds)) throw new Error("convert seconds to int failed");
    parameters.duration = seconds > 0 ? seconds : 5;
  }

  const metadata = req.metadata || {};
  Object.assign(input, metadata.input || {});
  Object.assign(parameters, metadata.parameters || {});
  if (req.smartDuration) parameters.duration = -1;
  const model = metadata.model === undefined ? upstreamModel : metadata.model;
  if (model !== upstreamModel) throw new Error("can't change model with metadata");
  const body = { model: model, input: input, parameters: parameters };

  if (wan3 || String(model).startsWith("wan2.7-i2v")) {
    if (!Array.isArray(input.media) || input.media.length === 0) {
      input.media = [];
      const first = trimmed(input.first_frame_url) || trimmed(input.img_url) || firstImage(req);
      const last = trimmed(input.last_frame_url) || secondImage(req);
      if (first) input.media.push({ type: "first_frame", url: first });
      if (last) input.media.push({ type: "last_frame", url: last });
      if (trimmed(input.audio_url)) input.media.push({ type: wan3 ? "reference_audio" : "driving_audio", url: input.audio_url });
    }
    if (!wan3 && input.media.length === 0) throw new Error("wan2.7-i2v requires image, images, input_reference, or input.media");
    delete input.img_url;
    delete input.first_frame_url;
    delete input.last_frame_url;
    delete input.audio_url;
  }
  validateNative(body, validationModel, allowUnresolvedSmart);
  for (const key of ["resolution", "size"]) if (!parameters[key]) delete parameters[key];
  return body;
}

function resolutionRatio(body) {
  const parameters = body.parameters || {};
  let resolution = parameters.size
    ? {
        "832*480": "480P",
        "480*832": "480P",
        "624*624": "480P",
        "1280*720": "720P",
        "720*1280": "720P",
        "960*960": "720P",
        "1088*832": "720P",
        "832*1088": "720P",
        "1920*1080": "1080P",
        "1080*1920": "1080P",
        "1440*1440": "1080P",
        "1632*1248": "1080P",
        "1248*1632": "1080P",
      }[parameters.size]
    : normalizeResolution(parameters.resolution || (isWan3(body.model) ? "1080P" : ""));
  const ratios = {
    "wan3.0-video": { "480P": 1, "720P": 2, "1080P": 4 },
    "wan3.0-video-prime": { "480P": 1, "720P": 2, "1080P": 4 },
    "wan2.6-i2v": { "720P": 1, "1080P": 1 / 0.6 },
    "wan2.5-t2v-preview": { "480P": 1, "720P": 2, "1080P": 1 / 0.3 },
    "wan2.2-t2v-plus": { "480P": 1, "1080P": 5 },
    "wan2.5-i2v-preview": { "480P": 1, "720P": 2, "1080P": 1 / 0.3 },
    "wan2.2-i2v-plus": { "480P": 1, "1080P": 5 },
    "wan2.2-kf2v-flash": { "480P": 1, "720P": 2, "1080P": 4.8 },
    "wan2.2-i2v-flash": { "480P": 1, "720P": 2 },
    "wan2.2-s2v": { "480P": 1, "720P": 1.8 },
  };
  return ratios[body.model] ? { key: "resolution-" + resolution, value: ratios[body.model][resolution] } : null;
}

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

export function buildSubmitRequest(ctx) {
  const body = convert(ctx);
  return {
    url: ctx.baseUrl + "/api/v1/services/aigc/video-generation/video-synthesis",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "X-DashScope-Async": "enable" },
    body: body,
    action: body.input.img_url || (body.input.media || []).length ? "image_to_video" : "text_to_video",
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  if (body.code) return { error: { code: "ali_api_error", message: body.code + ": " + (body.message || ""), httpStatus: 502 } };
  if (!body.output || !body.output.task_id) throw new Error("task_id is empty");
  return {
    taskId: body.output.task_id,
    taskData: Object.assign({}, body, { output: Object.assign({}, body.output, { task_id: ctx.publicTaskId || body.output.task_id }) }),
  };
}

export function extractUsage(ctx) {
  const body = convert(ctx);
  const parameters = body.parameters || {};
  const wan3 = isWan3(ctx.model) || isWan3(ctx.upstreamModel || body.model);
  const duration = parameters.duration == null ? 5 : parameters.duration;
  const seconds = wan3 && (duration === -1 || (body.input.media || []).some((media) => media.type === "reference_video")) ? 30 : duration;
  if (ctx.usagePurpose === "billing_ratios") {
    const ratios = { seconds: seconds };
    const resolution = resolutionRatio(wan3 ? Object.assign({}, body, { model: "wan3.0-video" }) : body);
    if (resolution && resolution.value !== undefined) ratios[resolution.key] = resolution.value;
    return ratios;
  }
  let resolution = parameters.size
    ? {
        "832*480": "480P",
        "480*832": "480P",
        "624*624": "480P",
        "1280*720": "720P",
        "720*1280": "720P",
        "960*960": "720P",
        "1088*832": "720P",
        "832*1088": "720P",
        "1920*1080": "1080P",
        "1080*1920": "1080P",
        "1440*1440": "1080P",
        "1632*1248": "1080P",
        "1248*1632": "1080P",
      }[parameters.size]
    : normalizeResolution(parameters.resolution || (wan3 ? "1080P" : ""));
  if (!["480P", "720P", "1080P"].includes(resolution)) resolution = "720P";
  return { seconds: seconds, resolution: resolution };
}

function wanUsage(usage) {
  if (!usage || typeof usage !== "object") return null;
  const number = (value) =>
    (typeof value === "number" || typeof value === "string") && Number.isFinite(Number(value)) ? Math.min(Math.max(Number(value), 0), 3600) : 0;
  const input = number(usage.input_video_duration);
  const output = number(usage.output_video_duration) || number(usage.duration);
  return input + output > 0 ? { kind: "video_duration", unit: "second", input: input, output: output, total: input + output } : null;
}

export function extractBillingOnComplete(task, _result, context) {
  if (!isWan3(task.model) && !isWan3(context.upstreamModel)) return null;
  const usage = wanUsage(task.data && task.data.usage);
  return usage ? { modelUnits: Math.min(usage.total, 30), consumedRatios: ["seconds"] } : null;
}

export function extractUsageOnComplete(task, taskResult, body) {
  const output = (body && body.output) || {};
  const facts = {};
  const seconds = Number(output.duration || output.duration_seconds || 0);
  if (Number.isFinite(seconds) && seconds > 0) facts.seconds = Math.min(seconds, 3600);
  const resolution = normalizeResolution(output.resolution || "");
  if (["480P", "720P", "1080P"].includes(resolution)) facts.resolution = resolution;
  const usage = wanUsage(body && body.usage);
  if (usage) facts.seconds = Math.min(usage.total, 30);
  return facts;
}

export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/api/v1/tasks/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

export function parseTaskResult(ctx, body) {
  const output = body.output || {};
  if (output.task_status === "PENDING") return { status: "QUEUED" };
  if (output.task_status === "RUNNING") return { status: "IN_PROGRESS" };
  if (output.task_status === "SUCCEEDED") return { status: "SUCCESS", url: output.video_url || "", usage: wanUsage(body.usage) };
  if (["FAILED", "CANCELED", "UNKNOWN"].includes(output.task_status)) {
    let reason = body.message || "";
    if (!reason && output.message) reason = "task failed, code: " + (output.code || "") + " , message: " + output.message;
    if (!reason) reason = "task failed";
    return { status: "FAILURE", reason: reason, usage: wanUsage(body.usage) };
  }
  return { status: "QUEUED" };
}

function artifactData(ctx) {
  const data = (ctx && ctx.data) || {};
  if (data.data && typeof data.data === "object" && data.data.task_id && Object.prototype.hasOwnProperty.call(data.data, "data")) return data.data.data || {};
  return data;
}

export function listArtifacts(task) {
  const output = artifactData(task).output || {};
  return task.status === "SUCCESS" && trimmed(output.video_url) ? [{ key: "video", type: "video" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const url = trimmed((artifactData(ctx).output || {}).video_url);
  if (!url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}

export const native = {
  createVideoTask: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json" || !ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
    const req = ctx.body.value,
      input = req.input || {};
    validateNative(req, req.model, !meta.models.includes(req.model));
    return {
      kind: "submit",
      model: req.model,
      action: input.img_url || (input.media || []).length ? "image_to_video" : "text_to_video",
      requestBody: normalizeSmartDuration({ model: req.model, nativeRequest: req }),
    };
  },
  taskCreated: function (ctx, task) {
    const data = task.data || {};
    return Object.assign({}, data, {
      request_id: data.request_id || "",
      output: Object.assign({ task_status: "PENDING" }, data.output || {}, { task_id: task.task_id }),
    });
  },
  taskStatus: function (ctx, task) {
    if (!task.data && task.status === "FAILURE") {
      return { output: { task_id: task.task_id, task_status: "FAILED", code: "task_failed", message: task.fail_reason || "task failed" } };
    }
    const data = task.data || {},
      output = Object.assign({}, data.output || {}, { task_id: task.task_id });
    const statuses = { NOT_START: "PENDING", SUBMITTED: "PENDING", QUEUED: "PENDING", IN_PROGRESS: "RUNNING", SUCCESS: "SUCCEEDED", FAILURE: "FAILED" };
    output.task_status = statuses[task.status] || "PENDING";
    if (task.status === "SUCCESS" && !output.video_url && task.result_url) output.video_url = task.result_url;
    if (task.status === "FAILURE" && !output.message) output.message = task.fail_reason || "task failed";
    return Object.assign({}, data, { output: output });
  },
  error: function (ctx, error) {
    return { code: error.code, message: error.message, request_id: "" };
  },
};

export const protocols = {
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const req = ctx.body.value;
      if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
      const model = trimmed(ctx.model);
      if (!model) throw new Error("model is required");
      if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
      const input = responsesInput(req);
      const prompt = input.prompt || trimmed(req.prompt);
      const requestBody = { model: model, prompt: prompt };
      if (trimmed(req.image)) requestBody.image = trimmed(req.image);
      if (req.images !== undefined && !Array.isArray(req.images)) throw new Error("images must be an array");
      const images = [];
      for (const image of req.images || []) if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
      for (const image of input.images) if (!images.includes(image)) images.push(image);
      if (images.length) requestBody.images = images;
      if (trimmed(req.input_reference)) requestBody.input_reference = trimmed(req.input_reference);
      for (const key of ["size", "duration", "seconds"]) {
        if (Object.prototype.hasOwnProperty.call(req, key)) requestBody[key] = req[key];
      }
      if (Object.prototype.hasOwnProperty.call(req, "metadata")) requestBody.metadata = req.metadata;
      if (
        !prompt &&
        (!model.includes("i2v") || !firstImage(requestBody)) &&
        !(isWan3(model) && ((requestBody.metadata && requestBody.metadata.input && requestBody.metadata.input.media) || firstImage(requestBody)))
      )
        throw new Error("input is required");
      return {
        kind: "submit",
        model: model,
        action: firstImage(requestBody) ? "image_to_video" : "text_to_video",
        requestBody: normalizeSmartDuration(requestBody),
      };
    },
    renderEvents: function (ctx, task, previousState) {
      const status = String(task.status || "UNKNOWN").toUpperCase();
      const value = Number(String(task.progress || "").replace("%", ""));
      const progress = Number.isFinite(value) && value >= 0 && value <= 100 ? value : null;
      const state = { status: status, progress: progress };
      if (status === "SUCCESS") {
        const text = responsesVideoText(ctx);
        const events = previousState && previousState.status === status ? [] : text ? [{ type: "output", data: text }] : [];
        return { events: events, state: state, done: true };
      }
      if (status === "FAILURE") {
        return { events: [{ type: "error", code: "task_failed", message: "task failed" }], state: state, done: true };
      }
      if (previousState && previousState.status === status && previousState.progress === progress) {
        return { events: [], state: state, done: false };
      }
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
            content: [
              {
                type: "output_text",
                text: responsesVideoText(ctx),
                annotations: [],
                logprobs: [],
              },
            ],
          },
        ],
        metadata: { vendor: "ali" },
      };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) {
      let req;
      if (ctx.body && ctx.body.kind === "json") req = ctx.body.value;
      else if (ctx.body && ctx.body.kind === "multipart") {
        if ((ctx.body.files || []).length) throw new Error("Alibaba requires image references to be URLs");
        const first = function (name) {
          const values = (ctx.body.fields || {})[name] || [];
          if (values.length > 1) throw new Error(name + " must be provided once");
          return values[0];
        };
        req = {};
        const fields = ctx.body.fields || {};
        for (const name of Object.keys(fields)) {
          if (name === "images") req.images = fields[name] || [];
          else req[name] = first(name);
        }
        if (req.metadata !== undefined) {
          let parsed;
          try {
            parsed = JSON.parse(req.metadata);
          } catch (e) {
            throw new Error("metadata must be a JSON object string");
          }
          if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("metadata must be a JSON object string");
          req.metadata = parsed;
        }
        if (req.seconds !== undefined) req.seconds = Number(req.seconds);
        else if (req.duration !== undefined) req.seconds = Number(req.duration);
        if (req.duration !== undefined) req.duration = Number(req.duration);
      } else throw new Error("JSON or multipart body required");
      return {
        kind: "submit",
        model: ctx.model,
        action: firstImage(req) ? "image_to_video" : "text_to_video",
        requestBody: normalizeSmartDuration(Object.assign({}, req, { model: ctx.model })),
      };
    },
    render: function (ctx, task) {
      const data = task.data || {},
        outputData = data.output || {};
      const statuses = {
        PENDING: "queued",
        RUNNING: "in_progress",
        SUCCEEDED: "completed",
        FAILED: "failed",
        CANCELED: "failed",
        UNKNOWN: "failed",
      };
      const output = {
        id: task.task_id,
        object: "video",
        model: task.properties ? task.properties.origin_model_name || "" : "",
        status: statuses[outputData.task_status] || "unknown",
        progress: Number(String(task.progress || "0").replace("%", "")),
        created_at: task.created_at,
        completed_at: task.updated_at,
      };
      if (data.code) output.error = { code: data.code, message: data.message || "" };
      else if (outputData.code) output.error = { code: outputData.code, message: outputData.message || "" };
      return output;
    },
  },
};
