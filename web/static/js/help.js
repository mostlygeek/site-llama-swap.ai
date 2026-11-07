// The Help page: loads the model in a worker, runs the agent loop and draws
// the conversation.

import { listTools, callTool } from "./mcp.js";
import { partialView, runAgent, toolDefinitions } from "./agent.js";
import { addCopyButtons, renderMarkdown } from "./markdown.js";
import { linkDocs } from "./doclinks.js";

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
  markLoading(true);
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
          setStatus(
            "loading",
            `${{ read: "Loading the model into memory", save: "Saving the model" }[msg.phase] ?? "Downloading the model"}: ` +
              `${formatBytes(msg.loaded)} of ${formatBytes(msg.total)}`,
          );
          break;
        case "ready":
          markLoading(false);
          modelState = "ready";
          setProgress(null);
          setStatus("ready", `LFM3-1.7B-HelpAgent (${msg.dtype}) ready`);
          els.notice.hidden = true;
          resolve();
          break;
        case "token":
          if (pending?.id === msg.id) pending.onText(msg.text, msg.tokens);
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
            markLoading(false);
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

// A browser that runs out of memory while loading the model can close and
// reload the page; iOS Safari does this with a brief flash. To tell the
// reader why instead of starting over, note when a load starts and clear it
// when it ends. Finding the note on a fresh page means the last load never
// ended.
const LOADING_KEY = "help-agent-loading";
const LOADING_STALE_MS = 15 * 60 * 1000;

function markLoading(on) {
  try {
    if (on) localStorage.setItem(LOADING_KEY, String(Date.now()));
    else localStorage.removeItem(LOADING_KEY);
  } catch {
    // Storage can be off; the warning is a nicety.
  }
}

// Leaving the page normally is not a crash.
addEventListener("pagehide", () => {
  if (modelState === "loading") markLoading(false);
});

function loadWasInterrupted() {
  try {
    const started = Number(localStorage.getItem(LOADING_KEY));
    localStorage.removeItem(LOADING_KEY);
    return started > 0 && Date.now() - started < LOADING_STALE_MS;
  } catch {
    return false;
  }
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
  // Browsers only offer WebGPU to secure pages: HTTPS, or http://localhost.
  // On plain HTTP to any other host navigator.gpu is missing even in a
  // browser that supports it, so say that instead of blaming the browser.
  if (!window.isSecureContext) {
    showGpuError(
      `WebGPU only works over HTTPS or on http://localhost, so it is turned off for ${location.origin}. ` +
        "Open this page over HTTPS, or through localhost (for example with an SSH tunnel).",
    );
    disableAgent();
    setStatus("error", "WebGPU needs HTTPS");
    return;
  }
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

// Inline icons, so the page needs no icon library.
const ICONS = {
  chevron: '<svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="m9 6 6 6-6 6"/></svg>',
  brain: '<svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" d="M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Zm0 0a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18Zm0 0v13"/></svg>',
  wrench: '<svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>',
  check: '<svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" d="M20 6 9 17l-5-5"/></svg>',
  alert: '<svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3ZM12 9v4m0 4h.01"/></svg>',
  copy: '<svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2" fill="none" stroke="currentColor" stroke-width="2"/><path fill="none" stroke="currentColor" stroke-width="2" d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  refresh: '<svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8M21 3v5h-5M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16m5 0H3v5"/></svg>',
};

function icon(name, className = "icon") {
  const span = el("span", className);
  span.innerHTML = ICONS[name];
  return span;
}

function formatDuration(ms) {
  if (ms == null) return "";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  const m = Math.floor(ms / 60_000);
  return `${m}m ${Math.round((ms % 60_000) / 1000)}s`;
}

function formatSpeed(tokens, ms) {
  return ms > 0 ? `${(tokens / (ms / 1000)).toFixed(1)} tokens/s` : "";
}

/** A header button that shows and hides body. Collapsed to start. */
function collapsible(head, body) {
  head.type = "button";
  head.setAttribute("aria-expanded", "false");
  body.hidden = true;
  head.addEventListener("click", () => {
    const open = head.getAttribute("aria-expanded") !== "true";
    head.setAttribute("aria-expanded", String(open));
    body.hidden = !open;
  });
}

/** The argument values of a tool call on one line, since the keys are implied by the tool. */
function argSummary(args) {
  if (!args || typeof args !== "object") return "";
  return Object.values(args)
    .map((v) => (typeof v === "string" ? v : JSON.stringify(v)))
    .join(", ");
}

/** A button that copies text() and briefly says so. */
function copyButton(text, title = "Copy response") {
  const button = el("button", "icon-button");
  button.type = "button";
  button.title = title;
  button.setAttribute("aria-label", title);
  button.append(icon("copy"));
  button.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(text());
      button.replaceChildren(icon("check"));
      button.classList.add("done");
      button.title = "Copied";
    } catch {
      button.title = "Copy failed";
    }
    setTimeout(() => {
      button.replaceChildren(icon("copy"));
      button.classList.remove("done");
      button.title = title;
    }, 1500);
  });
  return button;
}

/**
 * One agent response, laid out like the Help page in llama-swap's UI: a
 * collapsible Work box holding the reasoning and tool calls, the answer, then
 * an action row (regenerate, copy) and a status line with token counts and
 * speed that expands into a breakdown.
 */
function createAgentCard({ onRegenerate }) {
  const card = el("article", "msg agent running");

  // Work box: one header, then each reasoning block and tool call.
  const work = el("div", "work");
  const workHead = el("button", "work-head");
  const workSummary = el("span", "work-summary");
  const workSpin = el("span", "spinner");
  workHead.append(icon("chevron", "icon chevron"), el("span", "work-title", "Work"), workSummary, workSpin);
  const workBody = el("div", "work-body");
  work.append(workHead, workBody);
  collapsible(workHead, workBody);
  work.hidden = true;

  const answer = el("div", "answer markdown");
  const errorBox = el("p", "error");
  errorBox.hidden = true;

  // Footer: actions on the left, the status line on the right.
  const footer = el("div", "msg-footer");
  const actions = el("div", "msg-actions");
  const regen = el("button", "icon-button");
  regen.type = "button";
  regen.title = "Regenerate response";
  regen.setAttribute("aria-label", "Regenerate response");
  regen.append(icon("refresh"));
  regen.addEventListener("click", () => onRegenerate());
  const note = el("span", "note");
  const statsToggle = el("button", "stats-line");
  const statsText = el("span");
  statsToggle.append(icon("chevron", "icon chevron"), statsText);
  const statsDetail = el("dl", "stats-detail");
  collapsible(statsToggle, statsDetail);
  statsToggle.hidden = true;

  const contents = []; // finished answer text from each model turn
  const copy = copyButton(() => contents.join("\n\n"));
  actions.append(regen, copy, note);
  footer.append(actions, statsToggle);
  card.append(work, answer, errorBox, footer, statsDetail);
  els.thread.append(card);

  const totals = { turns: 0, promptTokens: 0, lastPrefillMs: 0, prefillMs: 0, tokens: 0, reasoningTokens: 0, generateMs: 0, tools: 0, toolMs: 0 };
  let reasoningMs = 0;
  let live = ""; // the streaming turn's answer text
  let turn = null; // the streaming turn: { started, firstToken, tokens, reasoning, reasoningDone }
  let renderQueued = false;
  let finished = false;
  let latest = true;
  let tick = null;
  // Actions appear once the response is complete.
  regen.hidden = true;
  copy.hidden = true;

  function renderAnswer() {
    const text = [...contents, live].filter((s) => s.trim()).join("\n\n");
    answer.innerHTML = renderMarkdown(text);
    addCopyButtons(answer);
    linkDocs(answer, config.docLinks);
  }

  function queueRender() {
    if (renderQueued) return;
    renderQueued = true;
    requestAnimationFrame(() => {
      renderQueued = false;
      renderAnswer();
      renderStatus();
      scrollToBottom();
    });
  }

  function renderWorkSummary() {
    const parts = [];
    let reasoningTokens = totals.reasoningTokens;
    if (turn?.reasoning && !turn.reasoningDone) reasoningTokens += turn.tokens;
    // Counted as tokens stream in, so it can be off by a token or two.
    parts.push(`~${reasoningTokens.toLocaleString()} reasoning tokens`);
    let ms = reasoningMs + totals.toolMs;
    if (turn?.reasoning && !turn.reasoningDone && turn.firstToken) ms += performance.now() - turn.firstToken;
    parts.push(formatDuration(ms));
    parts.push(`${totals.tools} tool call${totals.tools === 1 ? "" : "s"}`);
    workSummary.textContent = parts.join(" · ");
  }

  function renderStatus() {
    if (finished) return;
    renderWorkSummary();
    const tokens = totals.tokens + (turn?.tokens ?? 0);
    let ms = totals.generateMs;
    if (turn?.firstToken) ms += performance.now() - turn.firstToken;
    let phase = "";
    if (!turn) phase = "";
    else if (!turn.firstToken) phase = "processing prompt…";
    else if (turn.callingTool) phase = "calling a tool…";
    else if (!turn.reasoningDone) phase = "reasoning…";
    else phase = "writing…";
    statsText.textContent = [phase, tokens ? `${tokens.toLocaleString()} tokens` : "", formatSpeed(tokens, ms)]
      .filter(Boolean)
      .join(" · ");
    statsToggle.hidden = !statsText.textContent;
  }

  function reasoningBlock() {
    const block = el("div", "work-item reasoning");
    const head = el("button", "item-head");
    const label = el("span", "item-label", "Reasoning");
    const time = el("span", "item-meta", "reasoning…");
    head.append(icon("chevron", "icon chevron"), icon("brain"), label, time);
    const body = el("pre", "item-body");
    block.append(head, body);
    collapsible(head, body);
    workBody.append(block);
    work.hidden = false;
    return { block, time, body };
  }

  function closeReasoning() {
    if (!turn?.reasoning || turn.reasoningDone) return;
    turn.reasoningDone = true;
    const ms = turn.firstToken ? performance.now() - turn.firstToken : 0;
    reasoningMs += ms;
    turn.reasoning.time.textContent = formatDuration(ms);
  }

  return {
    get element() {
      return card;
    },
    status(text) {
      statsText.textContent = text;
      statsToggle.hidden = false;
    },
    startTurn() {
      turn = { started: performance.now(), firstToken: 0, tokens: 0, reasoning: null, reasoningDone: false, callingTool: false };
      live = "";
      // Keep the elapsed time and speed moving between tokens.
      clearInterval(tick);
      tick = setInterval(renderStatus, 250);
      renderStatus();
    },
    stream(raw, tokens) {
      if (!turn.firstToken) turn.firstToken = performance.now();
      turn.tokens = tokens ?? turn.tokens;
      const view = partialView(raw);
      if (view.reasoning.trim()) {
        if (!turn.reasoning) turn.reasoning = reasoningBlock();
        turn.reasoning.body.textContent = view.reasoning;
      }
      if (!view.thinking) closeReasoning();
      turn.callingTool = view.callingTool;
      live = view.content;
      work.classList.add("active");
      queueRender();
    },
    endTurn(message, stats) {
      closeReasoning();
      clearInterval(tick);
      totals.turns++;
      totals.promptTokens = stats.promptTokens ?? totals.promptTokens;
      totals.lastPrefillMs = stats.prefillMs ?? 0;
      totals.prefillMs += stats.prefillMs ?? 0;
      totals.tokens += stats.completionTokens ?? 0;
      totals.reasoningTokens += stats.reasoningTokens ?? 0;
      totals.generateMs += stats.generateMs ?? 0;
      turn = null;
      live = "";
      if (message.content) contents.push(message.content);
      renderAnswer();
      renderStatus();
    },
    toolStart(call) {
      const { name, arguments: args } = call.function;
      const block = el("div", "work-item tool running");
      const head = el("button", "item-head");
      const summary = el("span", "item-args", argSummary(args));
      const status = el("span", "item-status");
      status.append(el("span", "spinner"));
      const time = el("span", "item-meta");
      head.append(icon("chevron", "icon chevron"), icon("wrench"), el("span", "item-label", describeCall(name, args)), el("code", "item-name", name), summary, status, time);
      const body = el("div", "item-body tool-body");
      body.append(el("div", "item-heading", "Arguments"), el("pre", "", JSON.stringify(args, null, 2)));
      block.append(head, body);
      collapsible(head, body);
      workBody.append(block);
      work.hidden = false;
      totals.tools++;
      renderStatus();
      scrollToBottom();
      return { block, status, time, body };
    },
    toolEnd(item, ok, text, durationMs) {
      item.block.classList.remove("running");
      item.block.classList.add(ok ? "ok" : "failed");
      item.status.replaceChildren(icon(ok ? "check" : "alert"));
      item.time.textContent = formatDuration(durationMs);
      item.body.append(el("div", "item-heading", ok ? "Result" : "Error"), el("pre", "", text));
      totals.toolMs += durationMs;
      renderStatus();
    },
    finish(reason) {
      finished = true;
      clearInterval(tick);
      card.classList.remove("running");
      work.classList.remove("active");
      renderWorkSummary();

      const notes = {
        aborted: "Stopped",
        length: "Ran out of tokens while reasoning",
        max_iterations: `Stopped after ${config.maxIterations} model turns`,
        error: "Failed",
      };
      note.textContent = notes[reason] ?? "";
      regen.title = reason === "stop" ? "Regenerate response" : "Retry";
      regen.setAttribute("aria-label", regen.title);
      regen.hidden = !latest;
      copy.hidden = contents.length === 0;
      if (!contents.length && reason === "stop") {
        answer.append(el("p", "muted", "The model finished without writing an answer. Try regenerating."));
      }

      if (totals.tokens) {
        statsText.textContent = [
          `${totals.tokens.toLocaleString()} tokens`,
          formatDuration(totals.generateMs),
          formatSpeed(totals.tokens, totals.generateMs),
        ].join(" · ");
        const rows = [
          ["Model turns", totals.turns],
          ["Prompt, last turn", `${totals.promptTokens.toLocaleString()} tokens · ${formatDuration(totals.lastPrefillMs)} · ${formatSpeed(totals.promptTokens, totals.lastPrefillMs)}`],
          ["Prompt processing", `${formatDuration(totals.prefillMs)} over all turns`],
          ["Generated", `${totals.tokens.toLocaleString()} tokens, ~${totals.reasoningTokens.toLocaleString()} reasoning`],
          ["Generation", `${formatDuration(totals.generateMs)} · ${formatSpeed(totals.tokens, totals.generateMs)}`],
          ["Tool calls", `${totals.tools} · ${formatDuration(totals.toolMs)}`],
        ];
        statsDetail.replaceChildren(...rows.flatMap(([k, v]) => [el("dt", "", k), el("dd", "", String(v))]));
        statsToggle.hidden = false;
      } else {
        statsToggle.hidden = true;
      }
      card.classList.add("done");
    },
    error(message) {
      errorBox.textContent = message;
      errorBox.hidden = false;
    },
    /** Only the newest response can be regenerated. */
    setLatest(isLatest) {
      latest = isLatest;
      regen.hidden = !latest || !finished;
    },
  };
}

function setRunning(isRunning) {
  els.send.hidden = isRunning;
  els.stop.hidden = !isRunning;
  els.newChat.hidden = isRunning || history.length === 0;
  els.input.disabled = false;
}

let latestCard = null;

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
  await respond();
}

/**
 * Drops the newest response, from history and from the page, and asks the
 * model again. The question stays.
 */
async function regenerate(card, start) {
  if (running || card !== latestCard) return;
  history.length = start;
  card.element.remove();
  await respond();
}

/** Runs the agent on the conversation so far and draws its response. */
async function respond() {
  const start = history.length;
  latestCard?.setLatest(false);
  const card = createAgentCard({ onRegenerate: () => regenerate(card, start) });
  latestCard = card;
  card.setLatest(true);

  running = new AbortController();
  const signal = running.signal;
  setRunning(true);
  scrollToBottom(true);

  let reason = "error";
  try {
    if (modelState !== "ready") card.status("Loading the model…");
    await ensureModel();
    await toolsReady;
    if (!tools.length) throw new Error("The documentation tools are not available. Reload the page to try again.");
    if (signal.aborted) {
      reason = "aborted";
      return;
    }

    let raw = "";
    const steps = new Map();
    const events = runAgent({
      history,
      systemPrompt: config.systemPrompt,
      tools,
      maxIterations: config.maxIterations,
      signal,
      generate: generate((text, tokens) => {
        raw += text;
        card.stream(raw, tokens);
      }),
      callTool: (name, args) => callTool(name, args, signal),
    });

    for await (const event of events) {
      switch (event.type) {
        case "turn_start":
          raw = "";
          card.startTurn();
          break;
        case "turn_end":
          card.endTurn(event.message, event.stats);
          break;
        case "tool_start":
          steps.set(event.call.id, card.toolStart(event.call));
          break;
        case "tool_end":
          card.toolEnd(steps.get(event.call.id), event.ok, event.message.content, event.durationMs);
          break;
        case "error":
          card.error(`Something went wrong: ${event.message}`);
          break;
        case "done":
          reason = event.reason;
          break;
      }
    }
  } catch (error) {
    card.error(error.message);
  } finally {
    card.finish(reason);
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
  latestCard = null;
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

checkWebGPU().then(() => {
  if (gpuSupported && loadWasInterrupted()) {
    showGpuError(
      "The page reloaded while the model was loading. Browsers close a page that uses too much memory, " +
        "and this model needs several gigabytes while it loads, which can be more than a phone allows. " +
        "The download is kept, so trying again does not download it again.",
    );
    els.load.textContent = "Try again";
  }
});
