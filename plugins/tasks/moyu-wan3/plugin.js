const RESOLUTIONS = ["480P", "720P", "1080P"];
const RATIOS = ["adaptive", "16:9", "4:3", "1:1", "3:4", "9:16"];

export const meta = {
  apiVersion: 1,
  key: "moyu-wan3",
  name: "Moyu Wan3",
  description: {
    en: "Translate the official Wan3 video API to Moyu's asynchronous video API",
    zh: "将官方 Wan3 视频接口转换为魔芋异步视频接口",
  },
  version: "1.0.1",
  author: { name: "QuantumNous" },
  baseUrl: "https://www.moyu.info",
  models: ["wan3.0-video", "wan3.0-video-prime"],
  fetchMode: "per_task",
  protocols: ["wan_video"],
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    resolution: {
      enum: RESOLUTIONS,
      description: { en: "Output video resolution", zh: "输出视频分辨率" },
    },
  },
};

function own(value, key) {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function objectValue(value, name) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(name + " must be an object");
  return value;
}

function stringValue(value, name, required) {
  if (value == null) {
    if (required) throw new Error(name + " is required");
    return undefined;
  }
  if (typeof value !== "string" || !value.trim()) throw new Error(name + " must be a non-empty string");
  return value.trim();
}

function numberValue(value, name, integer) {
  if (typeof value !== "number" || !Number.isFinite(value) || (integer && !Number.isInteger(value))) throw new Error(name + " must be a finite number");
  return value;
}

function normalizedResolution(value) {
  if (value == null) return "1080P";
  const resolution = stringValue(value, "parameters.resolution", true).toUpperCase();
  if (!RESOLUTIONS.includes(resolution)) throw new Error("parameters.resolution must be 480P, 720P, or 1080P");
  return resolution;
}

function normalizedRatio(value) {
  if (value == null) return "adaptive";
  const ratio = stringValue(value, "parameters.ratio", true);
  if (!RATIOS.includes(ratio)) throw new Error("parameters.ratio is not supported");
  return ratio;
}

function normalizedDuration(value) {
  if (value == null) return 5;
  numberValue(value, "parameters.duration", true);
  if (value !== -1 && (value < 2 || value > 30)) throw new Error("parameters.duration must be -1 or an integer between 2 and 30");
  return value;
}

function normalizedMedia(input) {
  if (input.media == null) return undefined;
  if (!Array.isArray(input.media) || input.media.length === 0) throw new Error("input.media must be a non-empty array");
  const media = input.media.map((item, index) => {
    const value = objectValue(item, "input.media[" + index + "]");
    const type = stringValue(value.type, "input.media[" + index + "].type", true);
    const url = stringValue(value.url, "input.media[" + index + "].url", true);
    const limits = { first_frame: 1, last_frame: 1, reference_image: 10, reference_video: 5, reference_audio: 5, file: 1, link: 1 };
    if (!own(limits, type)) throw new Error("unsupported input.media type: " + type);
    return { type: type, url: url };
  });
  const counts = {};
  for (const item of media) counts[item.type] = (counts[item.type] || 0) + 1;
  for (const type of Object.keys(counts)) if (counts[type] > ({ first_frame: 1, last_frame: 1, reference_image: 10, reference_video: 5, reference_audio: 5, file: 1, link: 1 }[type] || 0)) throw new Error("too many input.media entries for " + type);
  const frames = counts.first_frame || counts.last_frame;
  const references = counts.reference_image || counts.reference_video || counts.reference_audio;
  if ((frames && references) || (counts.file && counts.link) || (counts.file && media.length > 1) || (counts.link && media.length > 1)) throw new Error("unsupported input.media combination");
  if (counts.last_frame && !counts.first_frame) throw new Error("last_frame requires first_frame");
  return media;
}

function normalizeRequest(body) {
  objectValue(body, "request body");
  const input = objectValue(body.input, "input");
  const parameters = body.parameters == null ? {} : objectValue(body.parameters, "parameters");
  const model = stringValue(body.model, "model", true);
  const prompt = input.prompt == null ? undefined : stringValue(input.prompt, "input.prompt", false);
  const media = normalizedMedia(input);
  if (!prompt && !media) throw new Error("input.prompt or input.media is required");
  for (const key of ["prompt_extend", "audio"]) if (own(parameters, key)) throw new Error("parameters." + key + " is not supported by the Moyu Wan3 upstream");

  const normalized = {
    model: model,
    prompt: prompt,
    media: media,
    resolution: normalizedResolution(parameters.resolution),
    ratio: normalizedRatio(parameters.ratio),
    duration: normalizedDuration(parameters.duration),
  };
  if (own(parameters, "seed")) {
    numberValue(parameters.seed, "parameters.seed", true);
    if (parameters.seed < 0 || parameters.seed > 2147483647) throw new Error("parameters.seed must be between 0 and 2147483647");
    normalized.seed = parameters.seed;
  }
  if (own(parameters, "watermark")) {
    if (typeof parameters.watermark !== "boolean") throw new Error("parameters.watermark must be a boolean");
    normalized.watermark = parameters.watermark;
  }
  return normalized;
}

function upstreamPayload(ctx) {
  if (ctx.preparedRequestBody) return ctx.preparedRequestBody;
  const body = ctx.requestBody || {};
  if (body.input !== undefined || body.nativeRequest !== undefined) return normalizeRequest(body.nativeRequest || body);
  const input = {};
  const parameters = {};
  for (const key of ["prompt", "media"]) if (own(body, key)) input[key] = body[key];
  for (const key of ["resolution", "ratio", "duration", "seed", "watermark"]) if (own(body, key)) parameters[key] = body[key];
  return normalizeRequest({ model: body.model || ctx.model, input: input, parameters: parameters });
}

function apiBase(ctx) {
  return String(ctx.baseUrl || "").replace(/\/+$/, "");
}

export function createVideoTask(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("Moyu Wan3 requires application/json requests");
  const req = objectValue(ctx.body.value, "request body");
  const normalized = normalizeRequest(req);
  return {
    kind: "submit",
    model: normalized.model,
    action: normalized.media ? "image_to_video" : "text_to_video",
    requestBody: normalized,
  };
}

export function validatePreparedRequest(ctx, body) {
  upstreamPayload({ model: ctx.model, requestBody: body });
}

export function buildSubmitRequest(ctx) {
  const request = upstreamPayload(ctx);
  const body = { model: ctx.upstreamModel || request.model };
  for (const key of ["prompt", "media", "resolution", "ratio", "duration", "seed", "watermark"]) if (request[key] !== undefined) body[key] = request[key];
  return {
    url: apiBase(ctx) + "/v1/video/generations",
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
    body: body,
    action: "generate",
  };
}

function responseData(body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("invalid Moyu video response");
  if (body.code !== undefined && body.code !== "success") throw new Error("Moyu video request failed: " + (body.message || body.code));
  if (body.code === "success" && body.data && typeof body.data === "object") return body.data;
  return body;
}

export function parseSubmitResponse(_ctx, resp) {
  const body = responseData(resp.body || {});
  const id = body.task_id || body.id;
  if (typeof id !== "string" || !id.trim()) throw new Error("task_id is empty");
  return { taskId: id, taskData: resp.body || body };
}

export function buildQueryRequest(ctx) {
  if (!ctx.taskId) throw new Error("task_id is required");
  return {
    url: apiBase(ctx) + "/v1/video/generations/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
  };
}

function taskData(body) {
  return responseData(body);
}

function normalizedStatus(value) {
  const status = String(value || "").toUpperCase();
  if (["QUEUED", "PENDING", "SUBMITTED"].includes(status)) return "QUEUED";
  if (["IN_PROGRESS", "RUNNING", "PROCESSING"].includes(status)) return "IN_PROGRESS";
  if (["SUCCESS", "SUCCEEDED", "COMPLETED"].includes(status)) return "SUCCESS";
  if (["FAILURE", "FAILED", "CANCELED", "CANCELLED"].includes(status)) return "FAILURE";
  return "UNKNOWN";
}

function resultUrl(data) {
  return data.result_url || data.video_url || (data.output && data.output.video_url) || "";
}

function progressValue(value) {
  if (value == null || value === "") return "";
  if (typeof value === "number" && Number.isFinite(value)) return value + "%";
  if (typeof value === "string") return value;
  throw new Error("invalid Moyu video task progress");
}

export function parseTaskResult(_ctx, body) {
  const data = taskData(body);
  const status = normalizedStatus(data.status || data.task_status);
  const url = resultUrl(data);
  let reason = data.fail_reason || data.error || data.message || "";
  if (reason && typeof reason !== "string") reason = JSON.stringify(reason);
  return {
    taskId: data.task_id || data.id || "",
    status: status,
    progress: progressValue(data.progress),
    reason: reason,
    url: status === "FAILURE" ? "" : url,
  };
}

function publicTaskData(value, publicTaskId) {
  if (Array.isArray(value)) return value.map((item) => publicTaskData(item, publicTaskId));
  if (!value || typeof value !== "object") return value;
  const result = {};
  for (const key of Object.keys(value)) {
    result[key] = ["id", "task_id", "upstream_task_id"].includes(key) ? publicTaskId : publicTaskData(value[key], publicTaskId);
  }
  return result;
}

export function sanitizeTaskData(body, publicTaskId) {
  return publicTaskData(body, publicTaskId);
}

function statusForWan(taskStatus) {
  return { QUEUED: "PENDING", IN_PROGRESS: "RUNNING", SUCCESS: "SUCCEEDED", FAILURE: "FAILED" }[taskStatus] || "PENDING";
}

export const native = {
  createVideoTask: createVideoTask,
  taskCreated: function (_ctx, task) {
    return { request_id: "", output: { task_id: task.task_id, task_status: "PENDING" } };
  },
  taskStatus: function (_ctx, task) {
    const data = task.data ? taskData(task.data) : {};
    const output = { task_id: task.task_id, task_status: statusForWan(task.status) };
    const url = resultUrl(data) || task.result_url || "";
    if (task.status === "SUCCESS" && url) output.video_url = url;
    if (task.status === "FAILURE") {
      output.code = "task_failed";
      output.message = task.fail_reason || data.fail_reason || data.message || "task failed";
    }
    return { request_id: "", output: output };
  },
  error: function (_ctx, error) {
    return { request_id: "", code: error.code, message: error.message };
  },
};

export const protocols = {
  wan_video: {
    decodeRequest: createVideoTask,
    renderSubmitted: native.taskCreated,
    render: native.taskStatus,
  },
};

export function extractUsage(ctx) {
  const request = upstreamPayload(ctx);
  if (ctx.usagePurpose === "billing_ratios") return null;
  return { seconds: request.duration === -1 ? 30 : request.duration, resolution: request.resolution };
}

export function extractUsageOnComplete(_task, _result, body) {
  const data = taskData(body);
  const usage = data.usage || {};
  const seconds = usage.output_duration ?? usage.duration ?? data.output_duration;
  if (typeof seconds === "number" && Number.isFinite(seconds) && seconds >= 0 && seconds <= 3600) return { seconds: seconds };
  return {};
}

export function listArtifacts(task) {
  if (task.status !== "SUCCESS") return [];
  const data = taskData(task.data || {});
  return resultUrl(data) || task.resultUrl ? [{ key: "video", type: "video" }] : [];
}

export function buildContentRequest(ctx) {
  const data = taskData(ctx.data || {});
  const url = ctx.artifactKey === "video" ? resultUrl(data) || ctx.resultUrl : "";
  if (!url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}
