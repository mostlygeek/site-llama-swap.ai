// Runs the Help agent's model off the main thread, so the page stays
// responsive while it downloads, prefills and generates.
//
// Messages in:
//   { type: "load", model, revision, transformersUrl }
//   { type: "generate", id, messages, tools, maxNewTokens }
//   { type: "stop" }
// Messages out:
//   { type: "progress", phase, loaded, total }  bytes; phase is "download",
//                                              "save" or "read"
//   { type: "status", text }                  a loading step
//   { type: "ready", dtype }
//   { type: "token", id, text, tokens }   tokens generated so far this turn
//   { type: "result", id, result }
//   { type: "error", id?, message, name? }

import { loadModel, generateTurn } from "./model.js";
import { downloadToCache } from "./download.js";

let T = null;
let loaded = null;
let stopping = null;
let stopRequested = false;

async function pickDtype() {
  if (!self.navigator?.gpu) {
    throw new Error("This browser does not support WebGPU.");
  }
  const adapter = await navigator.gpu.requestAdapter();
  if (!adapter) {
    throw new Error("WebGPU is available but no GPU adapter was found.");
  }
  // q4f16 loads without shader-f16 and then fails on the first generation,
  // so check the feature rather than guess from the GPU.
  return adapter.features.has("shader-f16") ? "q4f16" : "q4";
}

// Transformers.js keeps its downloads in this Cache Storage cache, keyed by
// the file's URL on huggingface.co.
const CACHE_NAME = "transformers-cache";

function modelFileURL(model, revision, file) {
  return `https://huggingface.co/${model}/resolve/${encodeURIComponent(revision)}/${file}`;
}

/** Downloads the model files into the cache Transformers.js reads; see download.js. */
async function precache(urls) {
  if (typeof caches === "undefined") return;
  const cache = await caches.open(CACHE_NAME);
  try {
    await downloadToCache(cache, urls, (p) => self.postMessage({ type: "progress", ...p }));
  } catch (error) {
    // fetch reports a dropped connection as a TypeError.
    if (error instanceof TypeError) {
      throw new Error(`The download stopped (${error.message}). Try again to continue where it left off.`);
    }
    throw error;
  }
}

async function load({ model, revision, transformersUrl }) {
  const dtype = await pickDtype();

  self.postMessage({ type: "status", text: `Downloading the model (${dtype})` });
  await precache([
    modelFileURL(model, revision, `onnx/model_${dtype}.onnx`),
    modelFileURL(model, revision, `onnx/model_${dtype}.onnx_data`),
  ]);

  self.postMessage({ type: "status", text: "Loading Transformers.js" });
  T = await import(transformersUrl);
  T.env.allowLocalModels = false;

  // Sum progress over every file, since the model loads several.
  const files = new Map();
  const onProgress = (info) => {
    if (info.status === "progress" && info.file && info.total) {
      files.set(info.file, { loaded: info.loaded, total: info.total });
      let done = 0;
      let total = 0;
      for (const f of files.values()) {
        done += f.loaded;
        total += f.total;
      }
      self.postMessage({ type: "progress", phase: "read", loaded: done, total });
    } else if (info.status === "done" && info.file) {
      const f = files.get(info.file);
      if (f) f.loaded = f.total;
    }
  };

  self.postMessage({ type: "status", text: "Loading the model" });
  loaded = await loadModel(T, { model, revision, device: "webgpu", dtype, onProgress });
  stopping = new T.InterruptableStoppingCriteria();

  // Compile the GPU shaders now, so the first question is not slower than the
  // rest.
  self.postMessage({ type: "status", text: "Preparing the GPU" });
  const warm = loaded.tokenizer("Hi", { add_special_tokens: false, return_tensors: "pt" });
  await loaded.model.generate({ ...warm, max_new_tokens: 1, do_sample: false });

  self.postMessage({ type: "ready", dtype });
}

async function generate({ id, messages, tools, maxNewTokens }) {
  stopRequested = false;
  stopping.reset();
  const result = await generateTurn(T, loaded, {
    messages,
    tools,
    maxNewTokens,
    stoppingCriteria: stopping,
    shouldStop: () => stopRequested,
    onText: (text, tokens) => self.postMessage({ type: "token", id, text, tokens }),
  });
  if (stopRequested) {
    const error = new Error("Generation stopped");
    error.name = "AbortError";
    throw error;
  }
  self.postMessage({ type: "result", id, result });
}

self.addEventListener("message", async (event) => {
  const msg = event.data;
  try {
    if (msg.type === "load") {
      await load(msg);
    } else if (msg.type === "generate") {
      await generate(msg);
    } else if (msg.type === "stop") {
      stopRequested = true;
      stopping?.interrupt();
    }
  } catch (error) {
    self.postMessage({
      type: "error",
      id: msg.id,
      name: error?.name,
      message: error?.message ?? String(error),
    });
  }
});
