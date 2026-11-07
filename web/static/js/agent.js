// The Help agent's loop.
//
// The model runs in the browser with wllama, whose llama.cpp applies the chat
// template and parses reasoning and tool calls, as llama-server does for
// llama-swap's own Help page. Nothing in this file touches the DOM, fetch or
// the model directly; help.js passes those in, which keeps it testable under
// Node (see web/test/agent.test.mjs).

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

/**
 * Runs the agent until the model answers without calling a tool, yielding
 * events for the UI. `history` is the conversation so far, without the
 * system prompt; the loop appends to it in place, so a follow-up question
 * continues the same conversation.
 *
 * Messages use the OpenAI shape. Tool call arguments are kept as objects,
 * which llama.cpp passes to the chat template as they are.
 *
 * @param {object} opts
 * @param {object[]} opts.history
 * @param {string} opts.systemPrompt
 * @param {object[]} opts.tools from toolDefinitions()
 * @param {(messages: object[], tools: object[]) => Promise<{reasoning: string, content: string, toolCalls: {name: string, arguments: object}[], finishReason?: string}>} opts.generate
 *   runs one model turn (see model.js); the caller shows the streamed text itself
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

    const toolCalls = result.toolCalls ?? [];
    const assistant = { role: "assistant", content: result.content ?? "" };
    if (result.reasoning) assistant.reasoning_content = result.reasoning;
    if (toolCalls.length) {
      assistant.tool_calls = toolCalls.map((call) => ({
        id: `call_${++callCounter}`,
        type: "function",
        function: { name: call.name, arguments: call.arguments },
      }));
    }
    history.push(assistant);
    yield { type: "turn_end", message: assistant, stats: result };

    if (!assistant.tool_calls) {
      // Out of tokens before writing an answer: still reasoning.
      const ranOut = result.finishReason === "length" && !assistant.content;
      yield { type: "done", reason: ranOut ? "length" : "stop" };
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
