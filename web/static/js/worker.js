// Runs the Help agent's model off the main thread, so the page stays
// responsive while it downloads and loads. wllama starts its own worker for
// llama.cpp inside this one.
//
// Messages in:
//   { type: "load", model, revision, modelFile, contextSize }
//   { type: "generate", id, messages, tools, maxNewTokens }
//   { type: "stop" }
// Messages out:
//   { type: "progress", phase, loaded, total }  bytes; phase is "download"
//                                              or "save"
//   { type: "status", text }                  a loading step
//   { type: "ready", quant, threads }
//   { type: "update", id, view }              the turn so far; see model.js
//   { type: "result", id, result }
//   { type: "error", id?, message, name? }

import { downloadToCache, PARTS_CACHE } from "./download.js";
import { generateTurn, loadParams } from "./model.js";

// wllama is built from source with a llama.cpp new enough for LFM3; see
// README.md. The page's asset version is passed on so a deploy is picked up.
const version = new URL(import.meta.url).search;
const WLLAMA_URL = new URL(`./vendor/wllama/index.js${version}`, import.meta.url).href;
const WASM_URL = new URL(`./vendor/wllama/wllama.wasm${version}`, import.meta.url).href;

// The model file is kept in this Cache Storage cache, keyed by its URL on
// huggingface.co.
const CACHE_NAME = "help-agent-models";
// Where the Transformers.js version of this page kept the ONNX model.
const OLD_CACHE_NAME = "transformers-cache";

let wllama = null;
let controller = null;

function modelFileURL(model, revision, file) {
  return `https://huggingface.co/${model}/resolve/${encodeURIComponent(revision)}/${file}`;
}

/**
 * wllama's main build needs WebGPU, JSPI (WebAssembly.Suspending) and
 * 64-bit WebAssembly memory. Chrome and Edge have all three.
 */
async function checkBrowser() {
  if (typeof WebAssembly.Suspending !== "function") {
    throw new Error("This browser cannot run the model: it lacks WebAssembly JSPI. Try a recent version of Chrome or Edge.");
  }
  try {
    new WebAssembly.Memory({ address: "i64", initial: 1n });
  } catch {
    throw new Error("This browser cannot run the model: it lacks 64-bit WebAssembly memory. Try a recent version of Chrome or Edge.");
  }
  if (!self.navigator?.gpu) {
    throw new Error("This browser does not support WebGPU.");
  }
  if (!(await navigator.gpu.requestAdapter())) {
    throw new Error("WebGPU is available but no GPU adapter was found.");
  }
}

/** Removes model files this page no longer uses, which can be gigabytes. */
async function removeOldFiles(keepURL) {
  if (typeof caches === "undefined") return;
  await caches.delete(OLD_CACHE_NAME);
  const cache = await caches.open(CACHE_NAME);
  for (const request of await cache.keys()) {
    if (request.url !== keepURL) await cache.delete(request);
  }
  // Parts of a download that never finished.
  const parts = await caches.open(PARTS_CACHE);
  for (const request of await parts.keys()) {
    if (!request.url.startsWith(`${keepURL}?`)) await parts.delete(request);
  }
}

/** Downloads the model file into the cache, resuming a partial download; see download.js. */
async function downloadModel(url) {
  const cache = await caches.open(CACHE_NAME);
  try {
    await downloadToCache(cache, [url], (p) => self.postMessage({ type: "progress", ...p }));
  } catch (error) {
    // fetch reports a dropped connection as a TypeError.
    if (error instanceof TypeError) {
      throw new Error(`The download stopped (${error.message}). Try again to continue where it left off.`);
    }
    throw error;
  }
  const response = await cache.match(url);
  if (!response) throw new Error("The model was downloaded but the browser did not keep it. Try again.");
  // A blob from Cache Storage stays on disk; wllama reads it in pieces.
  return response.blob();
}

async function load({ model, revision, modelFile, contextSize }) {
  await checkBrowser();
  const url = modelFileURL(model, revision, modelFile);
  await removeOldFiles(url).catch(() => {});

  self.postMessage({ type: "status", text: "Downloading the model" });
  const blob = await downloadModel(url);

  self.postMessage({ type: "status", text: "Loading wllama" });
  const { Wllama, LoggerWithoutDebug } = await import(WLLAMA_URL);
  wllama = new Wllama({ default: WASM_URL }, { logger: LoggerWithoutDebug });
  // The compatibility build for browsers without JSPI is not bundled.
  wllama.setCompat(null);

  // llama.cpp warms up while loading, which also compiles the GPU shaders.
  self.postMessage({ type: "status", text: "Loading the model onto the GPU" });
  await wllama.loadModel([blob], loadParams(contextSize));

  self.postMessage({ type: "ready", quant: quantName(modelFile), threads: wllama.getNumThreads() });
}

/** "LFM3-1.7B-HelpAgent-Q4_K_M.gguf" → "Q4_K_M" */
function quantName(file) {
  return /-([A-Za-z0-9_]+)\.gguf$/.exec(file)?.[1] ?? "GGUF";
}

async function generate({ id, messages, tools, maxNewTokens }) {
  controller = new AbortController();
  try {
    const result = await generateTurn(wllama, {
      messages,
      tools,
      maxNewTokens,
      signal: controller.signal,
      onUpdate: (view) => self.postMessage({ type: "update", id, view }),
    });
    if (controller.signal.aborted) throw abortError();
    self.postMessage({ type: "result", id, result });
  } catch (error) {
    if (controller.signal.aborted) throw abortError();
    throw error;
  } finally {
    controller = null;
  }
}

function abortError() {
  const error = new Error("Generation stopped");
  error.name = "AbortError";
  return error;
}

self.addEventListener("message", async (event) => {
  const msg = event.data;
  try {
    if (msg.type === "load") {
      await load(msg);
    } else if (msg.type === "generate") {
      await generate(msg);
    } else if (msg.type === "stop") {
      controller?.abort();
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
