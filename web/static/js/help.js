// The Help page: loads the model in a worker, runs the agent loop and draws
// the conversation.

import { listTools, callTool } from "./mcp.js";
import { partialView, runAgent, toolDefinitions } from "./agent.js";
import { addCopyButtons, renderMarkdown } from "./markdown.js";

const config = JSON.parse(document.getElementById("agent-config").textContent);

const $ = (id) => document.getElementById(id);
const els = {
  dot: $("model-dot"),
  label: $("model-label"),
  progress: $("progress"),
  progressFill: $("progress-fill"),
  messages: $("messages"),
  intro: $("intro"),
  notice: $("notice"),
  downloadNote: $("download-note"),
  gpuError: $("gpu-error"),
  load: $("load-model"),
  suggestions: $("suggestions"),
  thread: $("thread"),
  composer: $("composer"),
  input: $("input"),
  send: $("send"),
  stop: $("stop"),
  newChat: $("new-chat"),
};

// ---------------------------------------------------------------------------
// Model worker

let worker = null;
let modelState = "idle"; // idle | loading | ready | error
let loadPromise = null;
let gpuSupported = true;
let pending = null; // the in-flight generate request
let nextGenerateId = 1;

function setStatus(state, text) {
  els.dot.dataset.state = state;
  els.label.textContent = text;
}

function setProgress(fraction) {
  if (fraction == null) {
    els.progress.hidden = true;
    return;
  }
  els.progress.hidden = false;
  els.progressFill.style.width = `${Math.max(0, Math.min(1, fraction)) * 100}%`;
}

function formatBytes(n) {
  return n >= 1e9 ? `${(n / 1e9).toFixed(2)} GB` : `${Math.round(n / 1e6)} MB`;
}

function ensureModel() {
  if (modelState === "ready") return Promise.resolve();
  if (loadPromise) return loadPromise;

  modelState = "loading";
  els.load.disabled = true;
  els.load.textContent = "Loading…";
  setStatus("loading", "Starting");

  // The worker URL carries the page's asset version so a deploy is picked up.
  const version = new URL(import.meta.url).search;
  worker = new Worker(new URL(`./worker.js${version}`, import.meta.url), { type: "module" });

  loadPromise = new Promise((resolve, reject) => {
    worker.addEventListener("message", (event) => {
      const msg = event.data;
      switch (msg.type) {
        case "status":
          setStatus("loading", msg.text);
          break;
        case "progress":
          setProgress(msg.loaded / msg.total);
          setStatus("loading", `Downloading the model: ${formatBytes(msg.loaded)} of ${formatBytes(msg.total)}`);
          break;
        case "ready":
          modelState = "ready";
          setProgress(null);
          setStatus("ready", `LFM3-1.7B-HelpAgent (${msg.dtype}) ready`);
          els.notice.hidden = true;
          resolve();
          break;
        case "token":
          if (pending?.id === msg.id) pending.onText(msg.text);
          break;
        case "result":
          if (pending?.id === msg.id) {
            pending.resolve(msg.result);
            pending = null;
          }
          break;
        case "error":
          if (msg.id != null && pending?.id === msg.id) {
            const error = new Error(msg.message);
            error.name = msg.name ?? "Error";
            pending.reject(error);
            pending = null;
          } else if (modelState !== "ready") {
            modelState = "error";
            loadPromise = null;
            worker.terminate();
            worker = null;
            setProgress(null);
            setStatus("error", "The model failed to load");
            els.load.disabled = false;
            els.load.textContent = "Try again";
            showGpuError(msg.message);
            reject(new Error(msg.message));
          }
          break;
      }
    });
    // An uncaught error inside the worker, or a worker script that failed to
    // load. Either way the worker cannot be trusted any more.
    worker.addEventListener("error", (event) => {
      event.preventDefault();
      const message = event.message || "The model worker stopped unexpectedly.";
      worker?.terminate();
      worker = null;
      if (pending) {
        pending.reject(new Error(message));
        pending = null;
      }
      if (modelState === "ready") {
        modelState = "idle";
        loadPromise = null;
        setStatus("error", "The model stopped. Send a question to load it again.");
      } else {
        modelState = "error";
        loadPromise = null;
        setStatus("error", "The model failed to load");
        els.load.disabled = false;
        els.load.textContent = "Try again";
        showGpuError(message);
        reject(new Error(message));
      }
    });
    navigator.storage?.persist?.().catch(() => {});
    worker.postMessage({
      type: "load",
      model: config.model,
      revision: config.revision,
      transformersUrl: config.transformersUrl,
    });
  });
  return loadPromise;
}

function generate(onText) {
  return (messages, tools) =>
    new Promise((resolve, reject) => {
      const id = nextGenerateId++;
      pending = { id, resolve, reject, onText };
      worker.postMessage({ type: "generate", id, messages, tools, maxNewTokens: config.maxNewTokens });
    });
}

function showGpuError(message) {
  els.gpuError.textContent = message;
  els.gpuError.hidden = false;
}

/** Turns off everything that would need the model. */
function disableAgent() {
  gpuSupported = false;
  els.load.disabled = true;
  els.input.disabled = true;
  els.send.disabled = true;
  els.input.placeholder = "The help agent needs WebGPU";
  for (const button of els.suggestions.querySelectorAll("button")) button.disabled = true;
}

async function checkWebGPU() {
  if (!navigator.gpu) {
    showGpuError(
      "This browser does not support WebGPU, which the help agent needs. Try a recent version of Chrome or Edge on a computer with a GPU.",
    );
    disableAgent();
    setStatus("error", "WebGPU is not available");
    return;
  }
  try {
    const adapter = await navigator.gpu.requestAdapter();
    if (!adapter) throw new Error("no adapter");
    const f16 = adapter.features.has("shader-f16");
    els.downloadNote.textContent = `The first load downloads about ${f16 ? "1.4" : "1.6"} GB. Your browser keeps it for next time.`;
  } catch {
    showGpuError("WebGPU is turned on in this browser, but it could not find a usable GPU.");
    disableAgent();
    setStatus("error", "No usable GPU");
  }
}

// ---------------------------------------------------------------------------
// Tools

let tools = [];
const toolsReady = listTools()
  .then((list) => {
    tools = toolDefinitions(list);
  })
  .catch((error) => {
    console.error("listing tools", error);
  });

function quote(s) {
  return `“${String(s ?? "").slice(0, 80)}”`;
}

/** A short description of a tool call, for its step in the work list. */
function describeCall(name, args = {}) {
  switch (name) {
    case "docs__search_docs":
      return `Searched the docs for ${quote(args.query)}`;
    case "docs__get_doc":
      return `Read ${args.id ?? "a document"}`;
    case "docs__list_docs":
      return args.category ? `Listed the ${args.category} docs` : "Listed the docs";
    case "docs__get_config_schema":
      return `Looked up ${args.path ? `the ${args.path} config key` : "the config schema"}`;
    case "code__grep":
      return `Searched the code for ${quote(args.pattern)}`;
    case "code__read":
      return `Read ${args.path ?? "a source file"}`;
    case "code__find":
      return `Found files matching ${quote(args.pattern)}`;
    case "code__ls":
      return `Listed ${args.path || "the repository"}`;
    default:
      return `Called ${name}`;
  }
}

// ---------------------------------------------------------------------------
// Conversation

const history = [];
let running = null; // AbortController while the agent runs

function scrollToBottom(force = false) {
  const el = els.messages;
  const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
  if (force || nearBottom) el.scrollTop = el.scrollHeight;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text != null) node.textContent = text;
  return node;
}

function addUserMessage(text) {
  const msg = el("article", "msg user");
  msg.append(el("div", "bubble", text));
  els.thread.append(msg);
}

/** One agent response: its work steps, its answer and a stats line. */
function createAgentCard() {
  const card = el("article", "msg agent");
  const work = el("div", "work");
  const answer = el("div", "answer markdown");
  const meta = el("div", "meta");
  card.append(work, answer, meta);
  els.thread.append(card);

  const contents = []; // finished answer text from each model turn
  let live = ""; // the streaming turn's answer text
  let thinking = null;
  let renderQueued = false;

  function renderAnswer() {
    const text = [...contents, live].filter((s) => s.trim()).join("\n\n");
    answer.innerHTML = renderMarkdown(text);
    addCopyButtons(answer);
  }

  function queueRender() {
    if (renderQueued) return;
    renderQueued = true;
    requestAnimationFrame(() => {
      renderQueued = false;
      renderAnswer();
      scrollToBottom();
    });
  }

  function step(className, label) {
    const details = el("details", `step ${className}`);
    const summary = el("summary");
    const status = el("span", "status");
    const text = el("span", "label", label);
    const time = el("span", "time");
    summary.append(status, text, time);
    const body = el("pre", "step-body");
    details.append(summary, body);
    work.append(details);
    return { details, status, text, time, body };
  }

  return {
    status(text) {
      meta.textContent = text;
    },
    startTurn() {
      thinking = null;
      live = "";
      card.classList.add("running");
    },
    stream(raw) {
      const view = partialView(raw);
      if (view.reasoning.trim()) {
        if (!thinking) {
          thinking = step("thinking running", "Thinking");
        }
        thinking.body.textContent = view.reasoning;
      }
      if (!view.thinking && thinking) {
        thinking.details.classList.remove("running");
        thinking.text.textContent = "Thought";
      }
      live = view.content;
      meta.textContent = view.callingTool ? "Calling a tool…" : view.thinking ? "Thinking…" : "Writing…";
      queueRender();
    },
    endTurn(message) {
      if (thinking) {
        thinking.details.classList.remove("running");
        thinking.text.textContent = "Thought";
      }
      live = "";
      if (message.content) contents.push(message.content);
      renderAnswer();
    },
    toolStart(call) {
      const s = step("tool running", describeCall(call.function.name, call.function.arguments));
      s.body.textContent = JSON.stringify(call.function.arguments, null, 2);
      meta.textContent = "Calling a tool…";
      scrollToBottom();
      return s;
    },
    toolEnd(s, ok, text, durationMs) {
      s.details.classList.remove("running");
      s.details.classList.add(ok ? "ok" : "failed");
      s.body.textContent = text;
      s.time.textContent = `${durationMs} ms`;
    },
    finish(reason, stats) {
      card.classList.remove("running");
      const parts = [];
      if (stats.tools) parts.push(`${stats.tools} tool call${stats.tools === 1 ? "" : "s"}`);
      if (stats.tokens && stats.ms) parts.push(`${(stats.tokens / (stats.ms / 1000)).toFixed(1)} tokens/s`);
      if (reason === "aborted") parts.push("stopped");
      if (reason === "length") parts.push("ran out of tokens while thinking");
      if (reason === "max_iterations") parts.push(`stopped after ${config.maxIterations} model turns`);
      meta.textContent = parts.join(" · ");
      if (!contents.length && reason === "stop") {
        answer.append(el("p", "muted", "The model finished without writing an answer. Try asking again."));
      }
    },
    error(message) {
      card.classList.remove("running");
      card.append(el("p", "error", message));
      meta.textContent = "";
    },
  };
}

function setRunning(isRunning) {
  els.send.hidden = isRunning;
  els.stop.hidden = !isRunning;
  els.newChat.hidden = isRunning || history.length === 0;
  els.input.disabled = false;
}

async function ask(question) {
  if (running || !gpuSupported) return;
  question = question.trim();
  if (!question) return;

  els.suggestions.hidden = true;
  els.intro.classList.add("compact");
  addUserMessage(question);
  history.push({ role: "user", content: question });
  els.input.value = "";
  autoGrow();

  const card = createAgentCard();
  running = new AbortController();
  const signal = running.signal;
  setRunning(true);
  scrollToBottom(true);

  try {
    if (modelState !== "ready") card.status("Loading the model…");
    await ensureModel();
    await toolsReady;
    if (!tools.length) throw new Error("The documentation tools are not available. Reload the page to try again.");
    if (signal.aborted) {
      card.finish("aborted", {});
      return;
    }

    let raw = "";
    const stats = { tools: 0, tokens: 0, ms: 0 };
    const events = runAgent({
      history,
      systemPrompt: config.systemPrompt,
      tools,
      maxIterations: config.maxIterations,
      signal,
      generate: generate((text) => {
        raw += text;
        card.stream(raw);
      }),
      callTool: (name, args) => callTool(name, args, signal),
    });

    const steps = new Map();
    for await (const event of events) {
      switch (event.type) {
        case "turn_start":
          raw = "";
          card.startTurn();
          break;
        case "turn_end":
          stats.tokens += event.stats.completionTokens ?? 0;
          stats.ms += event.stats.generateMs ?? 0;
          card.endTurn(event.message);
          break;
        case "tool_start":
          steps.set(event.call.id, card.toolStart(event.call));
          break;
        case "tool_end":
          stats.tools++;
          card.toolEnd(steps.get(event.call.id), event.ok, event.message.content, event.durationMs);
          break;
        case "error":
          card.error(`Something went wrong: ${event.message}`);
          break;
        case "done":
          if (event.reason !== "error") card.finish(event.reason, stats);
          break;
      }
    }
  } catch (error) {
    card.error(error.message);
  } finally {
    running = null;
    setRunning(false);
    els.input.focus();
  }
}

function stop() {
  if (!running) return;
  running.abort();
  worker?.postMessage({ type: "stop" });
}

function newChat() {
  if (running) return;
  history.length = 0;
  els.thread.replaceChildren();
  els.suggestions.hidden = false;
  els.intro.classList.remove("compact");
  els.newChat.hidden = true;
  els.input.focus();
}

// ---------------------------------------------------------------------------
// Page wiring

function autoGrow() {
  els.input.style.height = "auto";
  els.input.style.height = `${Math.min(els.input.scrollHeight, 200)}px`;
}

for (const question of config.suggestions ?? []) {
  const button = el("button", "suggestion", question);
  button.type = "button";
  button.addEventListener("click", () => ask(question));
  els.suggestions.append(button);
}

els.load.addEventListener("click", () => ensureModel().catch(() => {}));
els.composer.addEventListener("submit", (event) => {
  event.preventDefault();
  ask(els.input.value);
});
els.input.addEventListener("input", autoGrow);
els.input.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    ask(els.input.value);
  }
});
els.stop.addEventListener("click", stop);
els.newChat.addEventListener("click", newChat);

checkWebGPU();
