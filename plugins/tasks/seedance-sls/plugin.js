export const meta = {
  apiVersion: 1,
  key: "seedance-sls",
  name: "Seedance SLS",
  version: "1.0.0",
  author: { name: "QuantumNous" },
  channelTypes: [104],
  models: [
    "doubao-seedance-1-0-pro-250528",
    "doubao-seedance-1-0-lite-t2v",
    "doubao-seedance-1-0-lite-i2v",
    "doubao-seedance-1-5-pro-251215",
    "doubao-seedance-2-0-260128",
    "doubao-seedance-2-0-fast-260128",
    "doubao-seedance-2-0-mini-260615",
    "doubao-seedance-2-5-260628",
  ],
  fetchMode: "per_task",
  usageSchema: {
    tokens: {
      type: "number",
      unit: "token",
      description: { en: "Upstream billing tokens; estimated at submission, measured on completion.", zh: "上游计费 token：提交时预估，完成后使用实际值。" },
    },
    resolution: { enum: ["480p", "720p", "1080p", "4k"], description: { en: "Output video resolution.", zh: "输出视频分辨率。" } },
  },
  usageExamples: [
    { label: "480p · 5s", facts: { tokens: 48038, resolution: "480p" } },
    { label: "720p · 5s", facts: { tokens: 108000, resolution: "720p" } },
    { label: "1080p · 5s", facts: { tokens: 243000, resolution: "1080p" } },
  ],
  routes: [
    { method: "POST", path: "/seedance-sls/api/v3/contents/generations/tasks", type: "submit", decode: "createTask", render: "taskCreated" },
    { method: "GET", path: "/seedance-sls/api/v3/contents/generations/tasks/:task_id", type: "query", render: "taskStatus" },
  ],
  protocols: ["openai_video"],
};

function validatePayload(body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("request body must be an object");
  if (!Array.isArray(body.content)) throw new Error("content must be an array");
  if (!body.content.some((item) => item && item.type === "text" && typeof item.text === "string" && item.text.trim()))
    throw new Error("content must contain a non-empty text item");
  if (body.duration != null) {
    const value = body.duration;
    if (
      (typeof value !== "number" && typeof value !== "string") ||
      !String(value).trim() ||
      !Number.isInteger(Number(value)) ||
      Number(value) < -1 ||
      Number(value) > 3600
    )
      throw new Error("duration must be -1 or an integer between 0 and 3600");
  }
  if (
    body.frames != null &&
    ((typeof body.frames !== "number" && typeof body.frames !== "string") ||
      !String(body.frames).trim() ||
      !Number.isInteger(Number(body.frames)) ||
      Number(body.frames) < 0 ||
      Number(body.frames) > 86400)
  )
    throw new Error("frames must be an integer between 0 and 86400");
}

function decodeRequest(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("Seedance SLS requires application/json requests");
  const req = ctx.body.value;
  if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
  let body;
  if (req.content !== undefined) body = Object.assign({}, req);
  else {
    let metadata = req.metadata || {};
    if (typeof metadata === "string") metadata = JSON.parse(metadata);
    if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be an object");
    body = Object.assign({}, metadata);
    const images = Array.isArray(req.images) ? req.images.slice() : [];
    for (const image of [req.image, req.input_reference]) if (typeof image === "string" && image.trim() && !images.includes(image)) images.push(image);
    body.content = images.map((url) => ({ type: "image_url", image_url: { url: url } }));
    body.content.push({ type: "text", text: req.prompt || "" });
    if (req.duration != null) body.duration = req.duration;
    else if (req.seconds != null) body.duration = req.seconds;
    if (req.resolution != null) body.resolution = req.resolution;
    else if (req.size && !body.resolution) body.resolution = req.size;
    delete body.seconds;
  }
  body.model = ctx.model || req.model;
  if (typeof body.model !== "string" || !body.model.trim()) throw new Error("model is required");
  validatePayload(body);
  // Automatic duration is a provider mode, never a negative billing multiplier.
  const automaticDuration = Number(body.duration) === -1;
  if (automaticDuration) delete body.duration;
  return { kind: "submit", model: body.model, action: "generate", requestBody: { model: body.model, payload: body, automaticDuration: automaticDuration } };
}

function providerPayload(ctx) {
  if (ctx.preparedRequestBody) return ctx.preparedRequestBody;
  const req = ctx.requestBody || {};
  const body = Object.assign({}, req.payload || {});
  if (req.automaticDuration) body.duration = -1;
  body.model = ctx.upstreamModel || ctx.model || body.model;
  validatePayload(body);
  return body;
}

export function buildSubmitRequest(ctx) {
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + "/v1/video/generations",
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
    body: providerPayload(ctx),
    action: "generate",
  };
}

export function validatePreparedRequest(_ctx, body) {
  validatePayload(body);
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  const id = body.task_id || body.id;
  if (typeof id !== "string" || !id.trim()) throw new Error("task_id is empty");
  const data = Object.assign({}, body);
  if (data.task_id !== undefined) data.task_id = ctx.publicTaskId;
  if (data.id !== undefined) data.id = ctx.publicTaskId;
  return { taskId: id, taskData: data };
}

// SLS may relay another gateway and expose several distinct upstream IDs.
// Rewrite only structured identity fields; signed URLs and opaque text stay intact.
function publicTaskData(value, publicTaskId) {
  if (Array.isArray(value)) return value.map((item) => publicTaskData(item, publicTaskId));
  if (!value || typeof value !== "object") return value;
  const result = {};
  for (const key of Object.keys(value)) result[key] = key === "task_id" || key === "upstream_task_id" ? publicTaskId : publicTaskData(value[key], publicTaskId);
  return result;
}

export function sanitizeTaskData(body, publicTaskId) {
  const data = publicTaskData(body, publicTaskId);
  if (data && !Array.isArray(data) && typeof data === "object" && data.id !== undefined) data.id = publicTaskId;
  return data;
}

export function extractUsage(ctx) {
  const body = providerPayload(ctx);
  const model = ctx.model;
  const resolution = String(body.resolution || "720p")
    .trim()
    .toLowerCase();
  const video = body.content.some((item) => item && (item.type === "video_url" || item.video_url !== undefined));
  let ratio = 1;
  if (model === "doubao-seedance-2-0-260128") {
    if (resolution === "1080p") ratio = video ? 31 / 46 : 51 / 46;
    else if (resolution === "4k") ratio = video ? 16 / 46 : 26 / 46;
    else ratio = video ? 28 / 46 : 1;
  } else if (model === "doubao-seedance-2-5-260628") {
    if (resolution === "1080p") ratio = video ? 7 / 10.7 : 11.7 / 10.7;
    else if (resolution !== "4k") ratio = video ? 42 / 70 : 1;
  } else if (resolution !== "1080p" && resolution !== "4k") {
    if (model === "doubao-seedance-2-0-fast-260128") ratio = video ? 22 / 37 : 1;
    if (model === "doubao-seedance-2-0-mini-260615") ratio = video ? 14 / 23 : 1;
  }
  if (ctx.usagePurpose === "billing_ratios") return ratio === 1 ? null : { video_input: ratio };
  const seconds = Number(body.duration) > 0 ? Number(body.duration) : 15;
  const dimensions = { "480p": [854, 480], "720p": [1280, 720], "1080p": [1920, 1080], "4k": [3840, 2160] };
  const size = dimensions[resolution] || dimensions["1080p"];
  return { tokens: Math.min((seconds * size[0] * size[1] * 24) / 1024, 2147483647), resolution: dimensions[resolution] ? resolution : "1080p" };
}

export function buildQueryRequest(ctx) {
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + "/v1/video/generations/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
  };
}

function taskData(body, depth) {
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("invalid Seedance SLS task response");
  if (body.code && body.code !== "success") throw new Error("Seedance SLS task query failed: " + (body.message || body.code));
  const source = body.code === "success" ? body.data : body;
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("invalid Seedance SLS task data");
  const result = Object.assign({}, source);
  if (source.data && depth < 4) {
    const nested = taskData(source.data, depth + 1);
    for (const key of ["task_id", "status", "fail_reason", "result_url", "last_frame_url", "resolution", "ratio"]) if (!result[key]) result[key] = nested[key];
    for (const key of ["duration", "seed", "frames", "framespersecond", "generate_audio", "progress"])
      if (result[key] == null || result[key] === "") result[key] = nested[key];
    if (!result.total_tokens) result.total_tokens = nested.total_tokens;
  }
  return result;
}

export function parseTaskResult(_ctx, body) {
  const data = taskData(body, 0);
  const states = {
    SUBMITTED: "SUBMITTED",
    PENDING: "SUBMITTED",
    QUEUED: "QUEUED",
    IN_PROGRESS: "IN_PROGRESS",
    PROCESSING: "IN_PROGRESS",
    RUNNING: "IN_PROGRESS",
    SUCCESS: "SUCCESS",
    SUCCEEDED: "SUCCESS",
    COMPLETED: "SUCCESS",
    FAILURE: "FAILURE",
    FAILED: "FAILURE",
  };
  const status = states[String(data.status || "").toUpperCase()] || data.status || "";
  let reason = data.fail_reason || "",
    url = data.result_url || "";
  if (status === "FAILURE") {
    if (!reason) {
      reason = url;
      url = "";
    } else if (url && !/^https?:\/\/[^\s/?#]+(?:[/?#]|$)/.test(String(url).trim())) {
      if (String(url).trim() !== String(reason).trim()) reason += "\n" + String(url).trim();
      url = "";
    }
  }
  let progress = data.progress;
  if (progress != null && typeof progress !== "string" && typeof progress !== "number") throw new Error("invalid Seedance SLS task progress");
  if (typeof progress === "number" && Number.isFinite(progress)) progress = progress + "%";
  if (typeof progress !== "string" || !progress)
    progress = { SUCCESS: "100%", FAILURE: "100%", IN_PROGRESS: "30%", SUBMITTED: "10%", QUEUED: "20%" }[status] || "";
  const tokens = Number(data.total_tokens);
  return {
    taskId: data.task_id || "",
    status: status,
    progress: progress,
    reason: reason,
    url: url,
    totalTokens: Number.isFinite(tokens) && tokens > 0 ? Math.min(Math.floor(tokens), 2147483647) : 0,
  };
}

export function extractUsageOnComplete(_task, _result, body) {
  const result = parseTaskResult({}, body);
  return result.totalTokens > 0 ? { tokens: result.totalTokens } : {};
}

export const native = {
  createTask: decodeRequest,
  taskCreated: function (_ctx, task) {
    return { id: task.task_id };
  },
  taskStatus: function (_ctx, task) {
    const data = task.data ? taskData(task.data, 0) : {};
    const statuses = { SUCCESS: "succeeded", FAILURE: "failed", IN_PROGRESS: "running" };
    const result = {
      id: task.task_id,
      model: task.model || "",
      status: statuses[task.status] || "queued",
      created_at: task.created_at,
      updated_at: task.updated_at,
    };
    for (const key of ["seed", "resolution", "ratio", "duration", "frames", "framespersecond", "generate_audio"])
      if (data[key] != null && data[key] !== "") result[key] = data[key];
    const tokens = Number(data.total_tokens);
    if (Number.isFinite(tokens) && tokens > 0) result.usage = { completion_tokens: Math.min(tokens, 2147483647), total_tokens: Math.min(tokens, 2147483647) };
    if (task.status === "SUCCESS") {
      result.content = { video_url: data.result_url || task.result_url || "" };
      if (data.last_frame_url) result.content.last_frame_url = data.last_frame_url;
    }
    if (task.status === "FAILURE") result.error = { code: "", message: task.fail_reason || "task failed" };
    return result;
  },
  error: function (_ctx, error) {
    return { error: { code: error.code, message: error.message } };
  },
};

export const protocols = {
  openai_video: {
    decodeRequest: decodeRequest,
    render: function (_ctx, task) {
      const states = { SUCCESS: "completed", FAILURE: "failed", IN_PROGRESS: "in_progress" };
      const result = {
        id: task.task_id,
        task_id: task.task_id,
        object: "video",
        model: task.model || "",
        status: states[task.status] || "queued",
        progress: Number(String(task.progress || "0").replace("%", "")),
        created_at: task.created_at,
      };
      if (task.status === "FAILURE") result.error = { message: task.fail_reason || "task failed" };
      return result;
    },
  },
};

export function listArtifacts(task) {
  if (task.status !== "SUCCESS") return [];
  const data = taskData(task.data || {}, 0);
  const result = [];
  if (data.result_url || task.resultUrl) result.push({ key: "video", type: "video" });
  if (data.last_frame_url) result.push({ key: "last_frame", type: "image" });
  return result;
}

export function buildContentRequest(ctx) {
  const data = taskData(ctx.data || {}, 0);
  const url = ctx.artifactKey === "video" ? data.result_url || ctx.resultUrl : ctx.artifactKey === "last_frame" ? data.last_frame_url : "";
  if (!url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}
