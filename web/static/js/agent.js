// The Help agent's loop and the parsing of the model's raw output.
//
// llama-swap's own Help page sends requests to llama-server, which applies the
// chat template and parses tool calls on the server. Here the model runs in
// the browser, so both jobs happen in this file. Nothing in it touches the DOM,
// fetch or the model directly; help.js passes those in, which keeps this file
// testable under Node (see web/static/js/agent.test.mjs).

/**
 * Converts MCP tools/list entries into the OpenAI-shaped tool definitions the
 * chat template renders. The field set and order match what llama-swap's UI
 * sends to llama-server, which is what the model was fine-tuned on.
 */
export function toolDefinitions(mcpTools) {
  return mcpTools.map((tool) => ({
    type: "function",
    function: {
      name: tool.name,
      description: tool.description ?? tool.title ?? "",
      parameters: tool.inputSchema ?? { type: "object", properties: {} },
    },
  }));
}

const THINK_CLOSE = "</think>";
const END_OF_TURN = "<|im_end|>";

// The model writes tool calls in LFM3's XML format, which the chat template
// also uses to render past calls:
//
//   <tool_call>
//   <function=docs__search_docs>
//   <parameter=query>
//   ttl unload
//   </parameter>
//   </function>
//   </tool_call>
const TOOL_CALL = /<tool_call>\s*<function=([^\n>]+)>([\s\S]*?)<\/function>\s*<\/tool_call>/g;
const PARAMETER = /<parameter=([^>\n]+)>\n?([\s\S]*?)\n?<\/parameter>/g;

/**
 * Converts a parameter's text to the type its JSON schema asks for, the way
 * llama-server does: strings stay as written, anything else is parsed as JSON
 * when it can be.
 */
export function coerceParameter(value, schema) {
  const type = schema?.type;
  if (type === "string") return value;
  if (type === "integer" || type === "number") {
    const n = Number(value.trim());
    return Number.isFinite(n) ? n : value;
  }
  if (type === "boolean") {
    const v = value.trim().toLowerCase();
    if (v === "true") return true;
    if (v === "false") return false;
    return value;
  }
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

/**
 * Splits one complete model turn into reasoning, visible content and tool
 * calls. The generation prompt ends with "<think>", so the raw text starts
 * inside the reasoning.
 *
 * @param {string} raw the decoded model output, special tokens included
 * @param {Map<string, object>} [toolsByName] tool definitions, for argument types
 */
export function parseOutput(raw, toolsByName = new Map()) {
  let text = raw;
  const end = text.indexOf(END_OF_TURN);
  if (end >= 0) text = text.slice(0, end);

  let reasoning = "";
  const close = text.indexOf(THINK_CLOSE);
  if (close >= 0) {
    reasoning = text.slice(0, close).replace(/^<think>/, "").replace(/^\n/, "");
    text = text.slice(close + THINK_CLOSE.length).replace(/^\s+/, "");
  } else if (!text.includes("<tool_call>")) {
    // The turn ran out of tokens before the reasoning closed.
    return { reasoning: text.replace(/^<think>\n?/, ""), content: "", toolCalls: [], finished: end >= 0, thinkingUnclosed: true };
  }

  const toolCalls = [];
  for (const match of text.matchAll(TOOL_CALL)) {
    const name = match[1].trim();
    const properties = toolsByName.get(name)?.function?.parameters?.properties ?? {};
    const args = {};
    for (const param of match[2].matchAll(PARAMETER)) {
      const key = param[1].trim();
      args[key] = coerceParameter(param[2], properties[key]);
    }
    toolCalls.push({ name, arguments: args });
  }

  const content = text.replace(TOOL_CALL, "").trim();
  return { reasoning, content, toolCalls, finished: end >= 0, thinkingUnclosed: false };
}

/**
 * What to show while a turn is still streaming: reasoning until the model
 * closes it, then the answer with any half-written tool call hidden.
 */
export function partialView(raw) {
  const close = raw.indexOf(THINK_CLOSE);
  if (close < 0) {
    return { reasoning: raw.replace(/^<think>\n?/, ""), content: "", thinking: true, callingTool: false };
  }
  let content = raw.slice(close + THINK_CLOSE.length).replace(/^\s+/, "");
  const end = content.indexOf(END_OF_TURN);
  if (end >= 0) content = content.slice(0, end);
  const toolStart = content.indexOf("<tool_call>");
  const callingTool = toolStart >= 0;
  if (callingTool) content = content.slice(0, toolStart);
  // Hide a tag the model is part way through writing.
  content = content.replace(/<[a-z_|]*$/, "");
  return { reasoning: raw.slice(0, close).replace(/^\n/, ""), content: content.trimEnd(), thinking: false, callingTool };
}

/**
 * Runs the agent until the model answers without calling a tool, yielding
 * events for the UI. `history` is the conversation so far, without the
 * system prompt; the loop appends to it in place, so a follow-up question
 * continues the same conversation.
 *
 * Messages use the shape the chat template expects. In particular tool call
 * arguments are objects, not JSON strings: the template refuses strings.
 *
 * @param {object} opts
 * @param {object[]} opts.history
 * @param {string} opts.systemPrompt
 * @param {object[]} opts.tools from toolDefinitions()
 * @param {(messages: object[], tools: object[]) => Promise<{text: string, promptTokens?: number, completionTokens?: number, generateMs?: number}>} opts.generate
 *   runs one model turn; the caller shows the streamed text itself
 * @param {(name: string, args: object) => Promise<{text: string, isError: boolean}>} opts.callTool
 * @param {number} [opts.maxIterations]
 * @param {AbortSignal} [opts.signal]
 */
export async function* runAgent({ history, systemPrompt, tools, generate, callTool, maxIterations = 12, signal }) {
  const toolsByName = new Map(tools.map((t) => [t.function.name, t]));
  let callCounter = history.filter((m) => m.role === "tool").length;

  for (let iteration = 1; iteration <= maxIterations; iteration++) {
    if (signal?.aborted) {
      yield { type: "done", reason: "aborted" };
      return;
    }

    yield { type: "turn_start", iteration };

    const messages = [{ role: "system", content: systemPrompt }, ...history];
    let result;
    try {
      result = await generate(messages, tools);
    } catch (error) {
      if (signal?.aborted || error?.name === "AbortError") {
        yield { type: "done", reason: "aborted" };
        return;
      }
      yield { type: "error", message: error?.message ?? String(error) };
      yield { type: "done", reason: "error" };
      return;
    }

    const parsed = parseOutput(result.text, toolsByName);
    const assistant = { role: "assistant", content: parsed.content };
    if (parsed.reasoning) assistant.reasoning_content = parsed.reasoning;
    if (parsed.toolCalls.length) {
      assistant.tool_calls = parsed.toolCalls.map((call) => ({
        id: `call_${++callCounter}`,
        type: "function",
        function: { name: call.name, arguments: call.arguments },
      }));
    }
    history.push(assistant);
    yield { type: "turn_end", message: assistant, stats: result, parsed };

    if (!assistant.tool_calls) {
      yield { type: "done", reason: parsed.thinkingUnclosed ? "length" : "stop" };
      return;
    }

    for (const call of assistant.tool_calls) {
      if (signal?.aborted) {
        // A call without a result would break the next request, so record
        // why it has none.
        history.push({ role: "tool", tool_call_id: call.id, name: call.function.name, content: "Cancelled by the user." });
        continue;
      }
      yield { type: "tool_start", call };
      const started = Date.now();
      let text;
      let ok;
      if (!toolsByName.has(call.function.name)) {
        ok = false;
        text = `Error: there is no tool named "${call.function.name}". Available tools: ${[...toolsByName.keys()].join(", ")}.`;
      } else {
        try {
          const outcome = await callTool(call.function.name, call.function.arguments);
          text = outcome.text;
          ok = !outcome.isError;
        } catch (error) {
          ok = false;
          text = `Error: ${error?.message ?? "the tool failed"}`;
        }
      }
      const message = { role: "tool", tool_call_id: call.id, name: call.function.name, content: text };
      history.push(message);
      yield { type: "tool_end", call, message, ok, durationMs: Date.now() - started };
    }
  }

  yield { type: "done", reason: "max_iterations" };
}
