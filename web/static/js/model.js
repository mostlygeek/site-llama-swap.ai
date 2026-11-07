// Loading and running LFM3-1.7B-HelpAgent with wllama, llama.cpp built for
// WebAssembly and WebGPU.
//
// llama.cpp applies the chat template and parses the model's reasoning and
// tool calls, as llama-server does with --jinja on llama-swap's own Help page.
// The wllama instance is passed in, so tests can use a fake one.

/** The sampling settings the model card's scores were measured with. */
export const SAMPLING = { temperature: 0.1, top_k: 50, repeat_penalty: 1.1 };

/**
 * The wllama loadModel() parameters.
 *
 * @param {number} contextSize the context window in tokens
 */
export function loadParams(contextSize) {
  return {
    n_ctx: contextSize,
    // One conversation at a time. Its cache carries over between turns, so
    // each turn only reads the tokens added since the last one.
    n_parallel: 1,
    jinja: true,
    // Return the reasoning separately from the answer.
    reasoning_format: "deepseek",
  };
}

/**
 * Runs one model turn.
 *
 * @param {object} wllama a loaded Wllama instance
 * @param {object} opts
 * @param {object[]} opts.messages chat messages, system prompt first
 * @param {object[]} opts.tools OpenAI-shaped tool definitions
 * @param {number} opts.maxNewTokens
 * @param {(view: {reasoning: string, content: string, thinking: boolean, callingTool: boolean, tokens: number}) => void} [opts.onUpdate]
 *   called as text streams in
 * @param {AbortSignal} [opts.signal]
 */
export async function generateTurn(wllama, { messages, tools, maxNewTokens, onUpdate, signal }) {
  const stream = await wllama.createChatCompletion({
    messages,
    tools,
    max_tokens: maxNewTokens,
    stream: true,
    timings_per_token: true,
    abortSignal: signal,
    ...SAMPLING,
  });

  let reasoning = "";
  let content = "";
  const calls = []; // by index: { name, arguments } with arguments as a JSON string
  let finishReason = null;
  let timings = null;
  let usage = null;
  let tokens = 0;
  // Reasoning comes first, so the token count when anything else arrives is
  // how many were reasoning.
  let reasoningTokens;

  for await (const chunk of stream) {
    if (chunk.timings) timings = chunk.timings;
    if (chunk.usage) usage = chunk.usage;
    const choice = chunk.choices?.[0];
    if (!choice) continue;
    const delta = choice.delta ?? {};
    if (choice.finish_reason) finishReason = choice.finish_reason;

    tokens = timings?.predicted_n ?? tokens + 1;
    if (delta.reasoning_content) reasoning += delta.reasoning_content;
    if ((delta.content || delta.tool_calls) && reasoningTokens === undefined) {
      reasoningTokens = reasoning ? Math.max(0, tokens - 1) : 0;
    }
    if (delta.content) content += delta.content;
    for (const call of delta.tool_calls ?? []) {
      const slot = (calls[call.index ?? calls.length] ??= { name: "", arguments: "" });
      if (call.function?.name) slot.name += call.function.name;
      if (call.function?.arguments) slot.arguments += call.function.arguments;
    }

    onUpdate?.({
      reasoning,
      content,
      thinking: !content && calls.length === 0,
      callingTool: calls.length > 0,
      tokens,
    });
  }

  const toolCalls = calls.filter(Boolean).map((call) => ({ name: call.name, arguments: parseArguments(call.arguments) }));

  return {
    reasoning,
    content: content.trim(),
    toolCalls,
    finishReason,
    // The whole prompt, and how much of it was read this turn rather than
    // taken from the cache of the turn before.
    promptTokens: usage?.prompt_tokens ?? (timings ? timings.prompt_n + (timings.cache_n ?? 0) : undefined),
    promptTokensRead: timings?.prompt_n,
    completionTokens: usage?.completion_tokens ?? timings?.predicted_n ?? tokens,
    reasoningTokens: reasoningTokens ?? (reasoning ? tokens : 0),
    prefillMs: timings?.prompt_ms,
    generateMs: timings?.predicted_ms,
  };
}

/** Tool call arguments arrive as a JSON string; the agent keeps objects. */
export function parseArguments(text) {
  if (!text) return {};
  try {
    const value = JSON.parse(text);
    return value && typeof value === "object" && !Array.isArray(value) ? value : {};
  } catch {
    return {};
  }
}
