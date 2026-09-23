export const meta = {
  apiVersion: 1,
  key: "hailuo",
  name: "Hailuo Video",
  icon: "Hailuo.Color",
  description: {
    en: "MiniMax Hailuo video generation (text-to-video, image-to-video, and MiniMax-H3 multimodal reference)",
    zh: "MiniMax 海螺视频生成（文生视频、图生视频、MiniMax-H3 多模态参考生视频）",
  },
  version: "1.1.2",
  author: { name: "QuantumNous" },
  channelTypes: [35],
  models: [
    "MiniMax-H3",
    "MiniMax-Hailuo-2.3",
    "MiniMax-Hailuo-2.3-Fast",
    "MiniMax-Hailuo-02",
    "T2V-01-Director",
    "T2V-01",
    "I2V-01-Director",
    "I2V-01-live",
    "I2V-01",
    "S2V-01",
  ],
  fetchMode: "per_task",
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: {
        en: "Requested video duration in seconds. MiniMax-H3 allows 4 to 15; Hailuo 2.3/02/2.3-Fast allow 6 or 10; 01-series allow 6.",
        zh: "请求的视频时长，单位为秒。MiniMax-H3 允许 4 到 15；Hailuo 2.3/02/2.3-Fast 允许 6 或 10；01 系列允许 6。",
      },
    },
    resolution: {
      enum: ["512P", "768P", "720P", "1080P", "2K"],
      description: { en: "Requested output video resolution.", zh: "请求的输出视频分辨率。" },
    },
    input_images: {
      type: "number",
      unit: "count",
      description: {
        en: "H3 input image count (estimated at submit, actual on completion).",
        zh: "H3 输入图片数量（提交时预估，完成后按实际值）。",
      },
    },
    input_video_seconds: {
      type: "number",
      unit: "second",
      description: {
        en: "H3 input video duration in seconds (reserved at the request maximum, actual on completion).",
        zh: "H3 输入视频时长，单位为秒（提交时按请求上限预留，完成后按实际值）。",
      },
    },
  },
  usageExamples: [
    { label: "2.3/02 768P 6s", facts: { seconds: 6, resolution: "768P", input_images: 0, input_video_seconds: 0 } },
    { label: "2.3/02 768P 10s", facts: { seconds: 10, resolution: "768P", input_images: 0, input_video_seconds: 0 } },
    { label: "2.3/02 1080P 6s", facts: { seconds: 6, resolution: "1080P", input_images: 0, input_video_seconds: 0 } },
    { label: "02 512P 6s", facts: { seconds: 6, resolution: "512P", input_images: 0, input_video_seconds: 0 } },
    { label: "02 512P 10s", facts: { seconds: 10, resolution: "512P", input_images: 0, input_video_seconds: 0 } },
    { label: "01-series 720P 6s", facts: { seconds: 6, resolution: "720P", input_images: 0, input_video_seconds: 0 } },
    { label: "H3 768P 5s", facts: { seconds: 5, resolution: "768P", input_images: 0, input_video_seconds: 0 } },
    { label: "H3 2K 5s · 9 images", facts: { seconds: 5, resolution: "2K", input_images: 9, input_video_seconds: 0 } },
    { label: "H3 2K 5s · input video", facts: { seconds: 5, resolution: "2K", input_images: 0, input_video_seconds: 15 } },
  ],
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_video"],
  routes: [
    { method: "POST", path: "/hailuo/v2/video_generation", type: "submit", models: ["MiniMax-H3"], decode: "createH3", render: "taskCreated" },
    { method: "GET", path: "/hailuo/v2/query/video_generation/:task_id", type: "query", render: "taskStatus" },
  ],
};

function trimmed(value) {
  return String(value || "").trim();
}

function usesH3(ctx) {
  return ctx.model === "MiniMax-H3" || ctx.upstreamModel === "MiniMax-H3";
}

function taskMetadata(req) {
  let metadata = req.metadata || {};
  if (typeof metadata === "string") metadata = JSON.parse(metadata);
  if (typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be an object");
  return metadata;
}

function h3VideoRequest(req, model, official) {
  const metadata = taskMetadata(req);
  const fields = Object.assign({}, req, metadata);
  const prompt = trimmed(req.prompt);
  let content;
  if (fields.content !== undefined) {
    if (!Array.isArray(fields.content)) throw new Error("metadata.content must be an array");
    content = fields.content.slice();
    if (!content.some((item) => item && item.type === "text" && typeof item.text === "string" && trimmed(item.text)) && prompt) {
      content.unshift({ type: "text", text: prompt });
    }
  } else {
    content = prompt ? [{ type: "text", text: prompt }] : [];
    const frames = [];
    for (const pair of [
      [fields.first_frame_image, "first_frame"],
      [fields.last_frame_image, "last_frame"],
    ]) {
      if (pair[0]) frames.push({ type: "image_url", role: pair[1], image_url: { url: pair[0] } });
    }
    if (!frames.length) {
      const images = Array.isArray(req.images) ? req.images.slice() : [];
      const image = req.input_reference || req.image;
      if (image && !images.includes(image)) images.unshift(image);
      if (images.length > 2) throw new Error("MiniMax-H3 accepts at most 2 frame images");
      for (let i = 0; i < images.length; i++) frames.push({ type: "image_url", role: i === 0 ? "first_frame" : "last_frame", image_url: { url: images[i] } });
    }
    content.push(...frames);
    for (const media of ["video", "audio"]) {
      const raw = fields["reference_" + media];
      const values = (Array.isArray(raw) ? raw : raw ? [raw] : []).filter((value) => value && (typeof value !== "string" || trimmed(value)));
      if (values.length > 3) throw new Error("MiniMax-H3 accepts at most 3 reference " + media + "s");
      for (const value of values) content.push({ type: media + "_url", role: "reference_" + media, [media + "_url"]: { url: value } });
    }
  }
  let textCount = 0,
    firstFrames = 0,
    lastFrames = 0,
    images = 0,
    videos = 0,
    audios = 0;
  for (const item of content) {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("MiniMax-H3 content item must be an object");
    const role = item.role == null ? "" : item.role;
    if (typeof role !== "string") throw new Error("MiniMax-H3 content role must be a string");
    if (item.type === "text") {
      if (role || typeof item.text !== "string" || !trimmed(item.text)) throw new Error("MiniMax-H3 requires a non-empty text item without a role");
      if (Array.from(item.text).length > 7000) throw new Error("MiniMax-H3 text must not exceed 7000 characters");
      textCount++;
      continue;
    }
    if (!["image_url", "video_url", "audio_url"].includes(item.type)) throw new Error("MiniMax-H3 content has unsupported type");
    const value = item[item.type] && item[item.type].url;
    // Only host-issued file placeholders can stand in for a string URL.
    const file = value && typeof value === "object" && typeof value.__fileRef === "string";
    if ((!file && typeof value !== "string") || (!file && !trimmed(value))) throw new Error("MiniMax-H3 requires a non-empty " + item.type + ".url");
    if (item.type === "image_url") {
      if (role === "" || role === "first_frame") firstFrames++;
      else if (role === "last_frame") lastFrames++;
      else if (role === "reference_image") images++;
      else throw new Error("MiniMax-H3 image_url role must be first_frame, last_frame, or reference_image");
    } else if (item.type === "video_url") {
      if (role !== "reference_video") throw new Error("MiniMax-H3 video_url role must be reference_video");
      videos++;
    } else {
      if (role !== "reference_audio") throw new Error("MiniMax-H3 audio_url role must be reference_audio");
      audios++;
    }
  }
  if (textCount === 0) throw new Error("MiniMax-H3 requires a non-empty text item");
  if (textCount > 1) throw new Error("MiniMax-H3 requires exactly one text item");
  if (firstFrames > 1 || lastFrames > 1) throw new Error("MiniMax-H3 accepts at most one first_frame and one last_frame image");
  if (images + firstFrames + lastFrames > 9) throw new Error("MiniMax-H3 accepts at most 9 reference images");
  if (videos > 3 || audios > 3) throw new Error("MiniMax-H3 accepts at most 3 reference videos and 3 reference audios");
  const hasFrame = firstFrames + lastFrames > 0;
  const hasReference = images + videos + audios > 0;
  if (hasFrame && hasReference) throw new Error("MiniMax-H3 cannot mix frame images with reference media");

  let duration = fields.duration === undefined ? fields.seconds : fields.duration;
  if (duration == null || duration === "") {
    if (official) throw new Error("MiniMax-H3 duration is required");
    duration = 5;
  }
  if (
    !["number", "string"].includes(typeof duration) ||
    (!official && typeof duration === "string" && !/^[+-]?\d+$/.test(duration)) ||
    (official && typeof duration !== "number") ||
    !Number.isInteger(Number(duration)) ||
    Number(duration) < 4 ||
    Number(duration) > 15
  ) {
    throw new Error("MiniMax-H3 duration must be an integer between 4 and 15 seconds");
  }
  let resolution = fields.resolution;
  if (!official && (resolution == null || resolution === "")) resolution = fields.size;
  if (resolution == null || resolution === "") {
    if (official) throw new Error("MiniMax-H3 resolution is required");
    resolution = "768P";
  }
  if (typeof resolution !== "string") throw new Error("MiniMax-H3 resolution must be 768P or 2K");
  if (!official) {
    resolution = resolution.trim().toUpperCase();
    resolution = resolution.includes("2K") ? "2K" : resolution.includes("768P") ? "768P" : resolution;
  }
  if (!["768P", "2K"].includes(resolution)) throw new Error("MiniMax-H3 resolution must be 768P or 2K");
  let ratio = fields.ratio;
  const providedRatio = ratio != null;
  if (providedRatio) {
    if (typeof ratio !== "string") throw new Error("MiniMax-H3 ratio must be a string");
    if (!official) ratio = ratio.trim();
    if (!["adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"].includes(ratio))
      throw new Error("MiniMax-H3 ratio must be one of adaptive, 21:9, 16:9, 4:3, 1:1, 3:4, 9:16");
  }
  if (hasFrame) ratio = "adaptive";
  else if (!providedRatio) {
    if (official && !hasReference) throw new Error("MiniMax-H3 ratio is required for text-to-video");
    ratio = hasReference ? "adaptive" : "16:9";
  }
  if (!hasFrame && !hasReference && ratio === "adaptive") throw new Error("MiniMax-H3 ratio adaptive requires an image, video, or audio input");
  const body = { model: model, content: content, duration: Number(duration), resolution: resolution, ratio: ratio };
  for (const key of ["callback_url", "aigc_watermark"]) {
    if (fields[key] != null) {
      if (typeof fields[key] !== (key === "callback_url" ? "string" : "boolean")) throw new Error(key + " has an invalid type");
      body[key] = fields[key];
    }
  }
  return body;
}

export const native = {
  createH3: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json" || !ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
    if (ctx.body.value.model !== "MiniMax-H3") throw new Error("MiniMax video V2 currently supports model MiniMax-H3");
    const request = h3VideoRequest(Object.assign({}, ctx.body.value, { metadata: undefined }), "MiniMax-H3", true);
    return { kind: "submit", model: "MiniMax-H3", requestBody: request, action: request.content.length > 1 ? "image_to_video" : "text_to_video" };
  },
  taskCreated: function (_ctx, task) {
    return { task_id: task.task_id };
  },
  taskStatus: function (_ctx, task) {
    const data = task.data || {};
    const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "running", SUCCESS: "succeeded", FAILURE: "failed" };
    const body = Object.assign({}, data.task || {}, { id: task.task_id, model: task.model || "", status: statuses[task.status] || "queued" });
    if (!body.created_at) body.created_at = task.created_at;
    if (!body.updated_at) body.updated_at = task.updated_at || task.created_at;
    if (task.status === "SUCCESS" && !body.content && task.result_url) body.content = { url: task.result_url };
    if (task.status === "FAILURE" && !body.error) body.error = { code: "task_failed", message: task.fail_reason || "task failed" };
    return Object.assign({}, data, { task: body });
  },
  error: function (_ctx, error) {
    const types = {
      400: "bad_request_error",
      401: "authorized_error",
      403: "authorized_error",
      402: "insufficient_balance_error",
      422: "unprocessable_entity_error",
      429: "rate_limit_error",
      529: "overloaded_error",
    };
    const body = {
      type: "error",
      error: {
        type: types[error.httpStatus] || (error.httpStatus >= 500 ? "server_error" : "api_error"),
        message: error.message,
        http_code: String(error.httpStatus),
      },
    };
    if (error.requestId) body.request_id = error.requestId;
    return body;
  },
};

function isModernHailuo(model) {
  return model === "MiniMax-Hailuo-2.3" || model === "MiniMax-Hailuo-2.3-Fast" || model === "MiniMax-Hailuo-02";
}

function defaultResolution(model) {
  if (isModernHailuo(model)) return "768P";
  return "720P";
}

function resolutionFor(size, model) {
  const value = String(size || "");
  if (value.includes("1080")) return "1080P";
  if (value.includes("768")) return "768P";
  if (value.includes("720")) return isModernHailuo(model) ? "768P" : "720P";
  if (value.includes("512")) return "512P";
  return defaultResolution(model);
}

function outboundDuration(req) {
  const raw = req && req.duration;
  if (raw === undefined || raw === null || raw === "") return 6;
  const n = Number(raw);
  if (!["number", "string"].includes(typeof raw) || !Number.isInteger(n) || n <= 0 || n > 3600)
    throw new Error("duration must be an integer between 1 and 3600 seconds");
  return n;
}

function outboundResolution(req, model) {
  if (req && req.resolution) return resolutionFor(req.resolution, model);
  const metadata = taskMetadata(req || {});
  if (metadata.resolution) return resolutionFor(metadata.resolution, model);
  if (req && req.size) return resolutionFor(req.size, model);
  return defaultResolution(model);
}

function hasHailuoImage(req, hasInputReferenceFile) {
  if (hasInputReferenceFile) return true;
  const metadata = taskMetadata(req || {});
  return Boolean(
    trimmed(req && req.input_reference) ||
    trimmed(req && req.image) ||
    (Array.isArray(req && req.images) && req.images.length) ||
    metadata.first_frame_image ||
    metadata.last_frame_image ||
    metadata.subject_reference
  );
}

const H3_MODEL = "MiniMax-H3";
const H3_MIN_DURATION = 4;
const H3_MAX_DURATION = 15;
const H3_MAX_REFERENCE_IMAGES = 9;
const H3_MAX_INPUT_VIDEO_SECONDS = 15;

// MiniMax-H3 speaks the /v2 video generation contract: a multimodal `content`
// array instead of flat frame fields, an explicit `ratio`, 768P/2K resolutions,
// a task id path parameter on query, and a `{task: {...}}` query envelope.
function isH3(model) {
  return model === H3_MODEL;
}

function h3HasVisualContent(content) {
  return content.some(function (item) {
    return item && (item.type === "image_url" || item.type === "video_url");
  });
}

function h3QueryTask(body) {
  const task = body && typeof body === "object" && !Array.isArray(body) ? body.task : null;
  return task && typeof task === "object" && !Array.isArray(task) ? task : null;
}

function h3APIError(body) {
  const error = body && typeof body === "object" && !Array.isArray(body) ? body.error : null;
  if (!error || typeof error !== "object" || Array.isArray(error)) return null;
  const message = trimmed(error.message);
  if (!message) return null;
  const statusCode = Number(error.http_code || error.code || 0);
  return { message: message, statusCode: Number.isInteger(statusCode) ? statusCode : 0 };
}

// Older T2V-01*/I2V-01*/S2V-01 official tables disagree on 1080P support (research: 未验证).
// Keep those models permissive: duration 6 only, resolution optional.
function validateHailuoCombo(model, duration, resolution, hasImage) {
  if (isH3(model)) return;
  if (model === "MiniMax-Hailuo-2.3-Fast" && !hasImage) {
    throw new Error("MiniMax-Hailuo-2.3-Fast supports image-to-video only");
  }
  if (!isModernHailuo(model)) {
    if (duration !== undefined && Number(duration) !== 6) throw new Error(model + " duration must be 6");
    return;
  }
  const n = duration === undefined ? 6 : Number(duration);
  if (n !== 6 && n !== 10) throw new Error(model + " duration must be 6 or 10");
  if (n === 10) {
    if (model === "MiniMax-Hailuo-02" && hasImage) {
      if (resolution !== "768P" && resolution !== "512P") throw new Error("MiniMax-Hailuo-02 duration 10 only allows resolution 768P or 512P");
      return;
    }
    if (resolution !== "768P") throw new Error(model + " duration 10 only allows resolution 768P");
    return;
  }
  const allowed = model === "MiniMax-Hailuo-02" && hasImage ? ["512P", "768P", "1080P"] : ["768P", "1080P"];
  if (allowed.indexOf(resolution) < 0) throw new Error(model + " duration 6 only allows resolution " + allowed.join(" or "));
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
  const req = ctx.requestBody || {};
  const model = ctx.upstreamModel;
  if (usesH3(ctx)) {
    const body = h3VideoRequest(req, model, false);
    return {
      url: ctx.baseUrl.replace(/\/+$/, "") + "/v2/video_generation",
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
      body: body,
      action: h3HasVisualContent(body.content) ? "image_to_video" : "text_to_video",
    };
  }
  const metadata = taskMetadata(req);
  if (meta.models.includes(model)) validateHailuoCombo(model, outboundDuration(req), outboundResolution(req, model), hasHailuoImage(req, false));
  const body = {
    model: model,
    prompt: req.prompt || undefined,
    duration: outboundDuration(req),
    resolution: outboundResolution(req, model),
  };
  ["prompt_optimizer", "fast_pretreatment", "callback_url", "aigc_watermark", "first_frame_image", "last_frame_image", "subject_reference"].forEach(
    function (key) {
      if (metadata[key] !== undefined && metadata[key] !== null) body[key] = metadata[key];
    }
  );
  return {
    url: ctx.baseUrl + "/v1/video_generation",
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
    body: body,
    action: hasHailuoImage(req, false) ? "image_to_video" : "text_to_video",
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  const error = parseSubmitError(ctx, resp);
  if (error) return { error: error };
  if (!body.base_resp && !usesH3(ctx)) throw new Error("hailuo submit failed");
  if (!body.task_id) throw new Error("missing task_id");
  return { taskId: body.task_id, taskData: Object.assign({}, body, { task_id: ctx.publicTaskId || body.task_id }) };
}

export function parseSubmitError(_ctx, resp) {
  const error = resp.body && resp.body.error;
  if (error && typeof error.message === "string" && error.message.trim()) {
    const status = Number(error.http_code || error.code);
    return {
      code: typeof error.type === "string" ? error.type : "upstream_error",
      message: error.message,
      httpStatus: Number.isInteger(status) && status >= 400 && status <= 599 ? status : 502,
    };
  }
  const base = resp.body && resp.body.base_resp;
  if (base && base.status_code !== undefined && base.status_code !== 0)
    return { code: String(base.status_code), message: "hailuo api error: " + (base.status_msg || "submit failed"), httpStatus: 400 };
  return null;
}

export function extractUsage(ctx) {
  if (usesH3(ctx)) {
    const body = h3VideoRequest(ctx.requestBody || {}, ctx.upstreamModel || ctx.model, false);
    if (ctx.usagePurpose === "billing_ratios") {
      const resolution = body.resolution === "2K" ? 1.6 : 1;
      const images = body.content.filter((item) => item.type === "image_url").length;
      return { seconds: body.duration, resolution_multiplier: resolution, image_surcharge: 1 + (Math.max(images - 5, 0) * 0.4) / (body.duration * resolution) };
    }
    return {
      seconds: body.duration,
      resolution: body.resolution,
      input_images: body.content.filter((item) => item.type === "image_url").length,
      input_video_seconds: body.content.some((item) => item.type === "video_url") ? H3_MAX_INPUT_VIDEO_SECONDS : 0,
    };
  }
  if (ctx.usagePurpose === "billing_ratios") return null;
  const req = ctx.requestBody || {};
  const model = ctx.upstreamModel || req.model;
  return { seconds: outboundDuration(req), resolution: outboundResolution(req, model), input_images: 0, input_video_seconds: 0 };
}

export function buildQueryRequest(ctx) {
  // Polling carries no relay info; the host fills these identities from the
  // persisted task properties.
  const path = usesH3(ctx)
    ? "/v2/query/video_generation/" + encodeURIComponent(ctx.taskId)
    : "/v1/query/video_generation?task_id=" + encodeURIComponent(ctx.taskId);
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + path,
    method: "GET",
    headers: { Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
  };
}

export function parseTaskResult(_ctx, body) {
  const apiError = h3APIError(body);
  if (apiError) {
    if (apiError.statusCode === 408 || apiError.statusCode === 429 || apiError.statusCode >= 500) throw new Error(apiError.message);
    return { code: apiError.statusCode, status: "FAILURE", progress: "100%", reason: apiError.message };
  }
  const h3Task = h3QueryTask(body);
  if (h3Task) {
    const h3Statuses = { queued: "QUEUED", running: "IN_PROGRESS", succeeded: "SUCCESS", failed: "FAILURE", cancelled: "FAILURE" };
    const h3Status = h3Statuses[h3Task.status];
    if (!h3Status) {
      return { status: "UNKNOWN", reason: "unrecognized status: " + String(h3Task.status || "") };
    }
    const h3Result = {
      taskId: h3Task.id,
      usage: h3Usage(h3Task.usage),
      code: 0,
      status: h3Status,
      progress: h3Status === "QUEUED" ? "30%" : h3Status === "IN_PROGRESS" ? "50%" : "100%",
    };
    if (h3Status === "SUCCESS") {
      const url = trimmed(h3Task.content && h3Task.content.url);
      if (url) h3Result.url = url;
    }
    if (h3Status === "FAILURE") {
      h3Result.reason = trimmed(h3Task.error && h3Task.error.message) || "task " + trimmed(h3Task.status);
    }
    return h3Result;
  }
  if (body.base_resp && body.base_resp.status_code !== 0) {
    return { code: body.base_resp.status_code || 0, status: "FAILURE", progress: "100%", reason: body.base_resp.status_msg || "" };
  }
  const base = body.base_resp || {};
  const statuses = { Preparing: "IN_PROGRESS", Queueing: "IN_PROGRESS", Processing: "IN_PROGRESS", Success: "SUCCESS", Fail: "FAILURE" };
  const status = statuses[body.status];
  if (!status) {
    return { status: "UNKNOWN", reason: "unrecognized status: " + String(body.status || "") };
  }
  const progress = status === "SUCCESS" || status === "FAILURE" ? "100%" : body.status === "Processing" ? "50%" : "30%";
  const reason = status === "FAILURE" ? "task failed" : "";
  return { code: base.status_code || 0, status: status, progress: progress, reason: reason };
}

function artifactData(ctx) {
  const data = (ctx && ctx.data) || {};
  if (data.data && typeof data.data === "object" && data.data.task_id && Object.prototype.hasOwnProperty.call(data.data, "data")) return data.data.data || {};
  return data;
}

function artifactFileID(ctx) {
  return trimmed(artifactData(ctx).file_id);
}

// /v2 tasks expose a public CDN URL instead of a downloadable file id.
function h3ArtifactURL(ctx) {
  const task = h3QueryTask(artifactData(ctx));
  return task ? trimmed(task.content && task.content.url) : "";
}

export function listArtifacts(task) {
  if (task.status !== "SUCCESS") return [];
  return artifactFileID(task) || h3ArtifactURL(task) || trimmed(task.resultUrl) ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const task = artifactData(ctx).task;
  if (task && task.content && trimmed(task.content.url)) return { url: task.content.url, method: ctx.clientRequest.method, credentialless: true };
  const fileID = artifactFileID(ctx);
  if (!fileID) {
    const url = h3ArtifactURL(ctx) || trimmed(ctx.resultUrl);
    if (!url) throw new Error("artifact_not_found");
    return { url: url, method: ctx.clientRequest.method, credentialless: true };
  }
  return {
    url: ctx.baseUrl + "/v1/files/download?file_id=" + encodeURIComponent(fileID),
    method: ctx.clientRequest.method,
    headers: { Accept: "video/*", Authorization: "Bearer " + ctx.apiKey },
  };
}

function h3Usage(usage) {
  if (!usage || typeof usage !== "object") return null;
  const bounded = (value, max) => (typeof value === "number" && Number.isFinite(value) ? Math.min(Math.max(value, 0), max) : 0);
  let input = bounded(usage.input_seconds, 3600);
  let output = bounded(usage.output_seconds, 15);
  let total = input + output;
  if (total <= 0) {
    total = bounded(usage.total_seconds, 3615);
    output = Math.min(total, 15);
    input = total - output;
  }
  const result = { kind: "video_duration", unit: "second", input: input, output: output, total: total };
  if (usage.input_image_count !== undefined && usage.input_image_count !== null) result.input_images = Math.trunc(bounded(usage.input_image_count, 9));
  return total > 0 || result.input_images > 0 ? result : null;
}

export function extractBillingOnComplete(task, _result, context) {
  const providerTask = task.data && task.data.task;
  if (!providerTask) return null;
  const usage = h3Usage(providerTask.usage);
  if (!usage) return null;
  const ratios = context.otherRatios || {};
  const frozenResolution = ratios.resolution_multiplier || ratios.resolution || 1;
  const resolution = providerTask.resolution ? (String(providerTask.resolution).trim().toUpperCase() === "2K" ? 1.6 : 1) : frozenResolution;
  const imageUnits =
    usage.input_images === undefined
      ? Math.max((ratios.image_surcharge || 1) - 1, 0) * (ratios.seconds || 0) * frozenResolution
      : Math.max(usage.input_images - 5, 0) * 0.4;
  const units = usage.total * resolution + imageUnits;
  return units > 0 ? { modelUnits: units, consumedRatios: ["seconds", "resolution", "resolution_multiplier", "image_surcharge"] } : null;
}

export function extractUsageOnComplete(_task, _taskResult, body) {
  const h3Task = h3QueryTask(body);
  if (h3Task) {
    const resolution = trimmed(h3Task.resolution).toUpperCase();
    const facts = {};
    if (resolution === "2K" || resolution === "768P") facts.resolution = resolution;
    const usage = h3Task.usage && typeof h3Task.usage === "object" && !Array.isArray(h3Task.usage) ? h3Task.usage : {};
    const fields = [
      { key: "seconds", value: usage.output_seconds, minimum: H3_MIN_DURATION, maximum: H3_MAX_DURATION, integer: false },
      { key: "input_images", value: usage.input_image_count, minimum: 0, maximum: H3_MAX_REFERENCE_IMAGES, integer: true },
      { key: "input_video_seconds", value: usage.input_seconds, minimum: 0, maximum: H3_MAX_INPUT_VIDEO_SECONDS, integer: false },
    ];
    // Omit malformed or out-of-contract upstream values so settlement keeps
    // the bounded submission estimate instead of accepting a new multiplier.
    for (const field of fields) {
      if (field.value === undefined || field.value === null || field.value === "") continue;
      const value = Number(field.value);
      if (!Number.isFinite(value) || value < field.minimum || value > field.maximum || (field.integer && !Number.isInteger(value))) continue;
      facts[field.key] = value;
    }
    return Object.keys(facts).length ? facts : null;
  }
  const width = Number((body || {}).video_width || 0);
  const height = Number((body || {}).video_height || 0);
  if (!(width > 0) || !(height > 0)) return null;
  return { resolution: resolutionFor(width + "x" + height, "") };
}

export const protocols = {
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const req = ctx.body.value;
      if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
      const model = trimmed(req.model);
      if (!model) throw new Error("model is required");
      if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
      if (req.images !== undefined && !Array.isArray(req.images)) throw new Error("images must be an array");
      if (req.metadata !== undefined && (!req.metadata || typeof req.metadata !== "object" || Array.isArray(req.metadata)))
        throw new Error("metadata must be an object");
      const input = responsesInput(req);
      const prompt = input.prompt || trimmed(req.prompt);
      const images = [];
      for (const image of [req.image, req.input_reference].concat(req.images || [], input.images)) {
        if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
      }
      if (!prompt && images.length === 0) throw new Error("input is required");
      const metadata = Object.assign({}, req.metadata || {});
      if (images.length && !metadata.first_frame_image) metadata.first_frame_image = images[0];
      if (images.length > 1 && !metadata.last_frame_image) metadata.last_frame_image = images[1];
      const requestBody = { model: model, prompt: prompt, metadata: metadata };
      if (images.length) requestBody.images = images;
      if (Object.prototype.hasOwnProperty.call(req, "seconds")) requestBody.duration = req.seconds;
      else if (Object.prototype.hasOwnProperty.call(req, "duration")) requestBody.duration = req.duration;
      if (Object.prototype.hasOwnProperty.call(req, "size")) requestBody.size = req.size;
      else if (Object.prototype.hasOwnProperty.call(req, "resolution")) requestBody.size = req.resolution;
      return { kind: "submit", model: model, action: images.length ? "image_to_video" : "text_to_video", requestBody: requestBody };
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
      if (status === "FAILURE")
        return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "task failed" }], state: state, done: true };
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
        metadata: { vendor: "hailuo" },
      };
    },
  },
};

const legacyRenderers = {
  openai_video: function (task) {
    const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
    const output = {
      id: task.task_id,
      object: "video",
      model: task.properties && task.properties.origin_model_name ? task.properties.origin_model_name : "",
      status: statuses[task.status] || "unknown",
      progress: Number(String(task.progress || "0").replace("%", "")),
      created_at: task.created_at,
    };
    if (task.updated_at) output.completed_at = task.updated_at;
    if (task.data && task.data.base_resp && task.data.base_resp.status_code !== 0) {
      output.error = { message: task.data.base_resp.status_msg, code: String(task.data.base_resp.status_code) };
    }
    return output;
  },
};

protocols.openai_video = {
  decodeRequest: function (ctx) {
    if (!ctx.body || (ctx.body.kind !== "json" && ctx.body.kind !== "multipart")) throw new Error("JSON or multipart body required");
    let req;
    let hasInputReferenceFile = false;
    if (ctx.body.kind === "json") {
      if (!ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
      req = Object.assign({}, ctx.body.value);
    } else {
      const first = function (name) {
        const values = (ctx.body.fields || {})[name] || [];
        if (values.length > 1) throw new Error(name + " must be provided once");
        return values[0];
      };
      req = {};
      const fields = ctx.body.fields || {};
      for (const name of Object.keys(fields)) {
        req[name] = first(name);
      }
      for (const file of ctx.body.files || []) {
        if (file.field !== "input_reference") throw new Error("unexpected file field: " + file.field);
        if (hasInputReferenceFile) throw new Error("input_reference must be provided once");
        hasInputReferenceFile = true;
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
    }
    const seconds = req.seconds === undefined ? req.duration : req.seconds;
    if (seconds !== undefined) req.duration = seconds;
    if (hasInputReferenceFile) {
      req.metadata = Object.assign({}, req.metadata || {}, {
        first_frame_image: { __fileRef: "request_file:input_reference", encoding: "dataUrl", maxBytes: 20971520 },
      });
    } else {
      const image = trimmed(req.input_reference || req.image);
      if (image) {
        req.metadata = Object.assign({}, req.metadata || {});
        if (!req.metadata.first_frame_image) req.metadata.first_frame_image = image;
      }
    }
    const hasImage = hasHailuoImage(req, hasInputReferenceFile);
    const comboModel = ctx.upstreamModel || ctx.model;
    if (comboModel === "MiniMax-H3") h3VideoRequest(req, comboModel, false);
    else if (meta.models.includes(comboModel)) validateHailuoCombo(comboModel, outboundDuration(req), outboundResolution(req, comboModel), hasImage);
    return {
      kind: "submit",
      model: ctx.model,
      action: hasImage ? "image_to_video" : "text_to_video",
      requestBody: Object.assign({}, req, { model: ctx.model }),
    };
  },
  render: function (ctx, task) {
    return legacyRenderers.openai_video(task);
  },
};
