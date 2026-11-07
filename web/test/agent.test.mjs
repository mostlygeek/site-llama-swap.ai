// Tests for web/static/js/agent.js. Run with: node --test web/test/
import { test } from "node:test";
import assert from "node:assert/strict";

import { runAgent, toolDefinitions } from "../static/js/agent.js";
import { generateTurn, parseArguments } from "../static/js/model.js";

const tools = toolDefinitions([
  {
    name: "docs__search_docs",
    description: "Search the docs.",
    inputSchema: {
      type: "object",
      properties: { query: { type: "string" }, max_results: { type: "integer" } },
      required: ["query"],
    },
  },
  {
    name: "docs__get_doc",
    description: "Read a doc.",
    inputSchema: { type: "object", properties: { id: { type: "string" }, offset: { type: "integer" } } },
  },
]);
const toolsByName = new Map(tools.map((t) => [t.function.name, t]));

// A turn that calls a tool, as model.js returns it.
const toolCallTurn = {
  reasoning: "The user wants TTL. I should search.\n",
  content: "",
  toolCalls: [{ name: "docs__search_docs", arguments: { query: "ttl unload", max_results: 3 } }],
  finishReason: "tool_calls",
};
const answerTurn = (content) => ({ reasoning: "done", content, toolCalls: [], finishReason: "stop" });

test("toolDefinitions keeps the fields llama-swap sends to llama-server", () => {
  assert.deepEqual(Object.keys(tools[0]), ["type", "function"]);
  assert.deepEqual(Object.keys(tools[0].function), ["name", "description", "parameters"]);
});

/** A wllama stand-in that streams the given chunks. */
function fakeWllama(chunks) {
  const seen = [];
  return {
    seen,
    async createChatCompletion(options) {
      seen.push(options);
      return (async function* () {
        yield* chunks;
      })();
    },
  };
}

const chunk = (delta, extra = {}) => ({ choices: [{ index: 0, delta, finish_reason: null }], ...extra });

test("generateTurn joins streamed reasoning, content and tool calls", async () => {
  const timings = { cache_n: 900, prompt_n: 100, prompt_ms: 50, predicted_n: 6, predicted_ms: 120 };
  const wllama = fakeWllama([
    chunk({ reasoning_content: "Search " }, { timings: { ...timings, predicted_n: 1 } }),
    chunk({ reasoning_content: "first." }, { timings: { ...timings, predicted_n: 2 } }),
    chunk({ content: "Let me look." }, { timings: { ...timings, predicted_n: 3 } }),
    chunk(
      { tool_calls: [{ index: 0, id: "x", type: "function", function: { name: "docs__search_docs", arguments: '{"query":' } }] },
      { timings: { ...timings, predicted_n: 5 } },
    ),
    chunk({ tool_calls: [{ index: 0, function: { arguments: '"ttl"}' } }] }, { timings }),
    {
      choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }],
      usage: { prompt_tokens: 1000, completion_tokens: 6 },
      timings,
    },
  ]);
  const views = [];
  const result = await generateTurn(wllama, {
    messages: [{ role: "user", content: "q" }],
    tools,
    maxNewTokens: 64,
    onUpdate: (view) => views.push(view),
  });

  assert.equal(wllama.seen[0].stream, true);
  assert.equal(wllama.seen[0].max_tokens, 64);
  assert.equal(wllama.seen[0].tools, tools);

  assert.equal(result.reasoning, "Search first.");
  assert.equal(result.content, "Let me look.");
  assert.deepEqual(result.toolCalls, [{ name: "docs__search_docs", arguments: { query: "ttl" } }]);
  assert.equal(result.finishReason, "tool_calls");
  assert.equal(result.promptTokens, 1000);
  assert.equal(result.promptTokensRead, 100);
  assert.equal(result.completionTokens, 6);
  assert.equal(result.reasoningTokens, 2);
  assert.equal(result.prefillMs, 50);
  assert.equal(result.generateMs, 120);

  assert.equal(views[0].thinking, true);
  assert.equal(views[2].thinking, false);
  assert.equal(views[2].callingTool, false);
  assert.equal(views[3].callingTool, true);
});

test("parseArguments returns an object for any input", () => {
  assert.deepEqual(parseArguments('{"id":"guides/a","offset":2}'), { id: "guides/a", offset: 2 });
  assert.deepEqual(parseArguments(""), {});
  assert.deepEqual(parseArguments("not json"), {});
  assert.deepEqual(parseArguments("[1]"), {});
});

test("runAgent calls tools until the model answers", async () => {
  const outputs = [toolCallTurn, answerTurn("Use `ttl`.")];
  const seen = [];
  const history = [{ role: "user", content: "How do I unload?" }];

  const events = [];
  for await (const event of runAgent({
    history,
    systemPrompt: "SYSTEM",
    tools,
    generate: async (messages) => {
      seen.push(messages);
      return { ...outputs.shift(), completionTokens: 10, generateMs: 100 };
    },
    callTool: async (name, args) => ({ text: `${name}:${args.query}`, isError: false }),
  })) {
    events.push(event.type);
  }

  assert.deepEqual(events, ["turn_start", "turn_end", "tool_start", "tool_end", "turn_start", "turn_end", "done"]);
  assert.equal(seen[0][0].content, "SYSTEM");

  // The second request carries the call, with object arguments, and its
  // result.
  const [, , assistant, tool] = seen[1];
  assert.equal(assistant.role, "assistant");
  assert.deepEqual(assistant.tool_calls[0].function.arguments, { query: "ttl unload", max_results: 3 });
  assert.equal(assistant.reasoning_content, "The user wants TTL. I should search.\n");
  assert.deepEqual(tool, { role: "tool", tool_call_id: "call_1", name: "docs__search_docs", content: "docs__search_docs:ttl unload" });

  assert.equal(history.at(-1).content, "Use `ttl`.");
});

test("runAgent answers an unknown tool with an error the model can read", async () => {
  const outputs = [{ reasoning: "x", content: "", toolCalls: [{ name: "config__get_config", arguments: {} }] }, answerTurn("ok")];
  const history = [{ role: "user", content: "q" }];
  let ok;
  for await (const event of runAgent({
    history,
    systemPrompt: "",
    tools,
    generate: async () => outputs.shift(),
    callTool: async () => assert.fail("must not call an unknown tool"),
  })) {
    if (event.type === "tool_end") ok = event.ok;
  }
  assert.equal(ok, false);
  assert.match(history[2].content, /no tool named "config__get_config"/);
});

test("runAgent stops after maxIterations", async () => {
  const history = [{ role: "user", content: "q" }];
  let last;
  for await (const event of runAgent({
    history,
    systemPrompt: "",
    tools,
    maxIterations: 2,
    generate: async () => toolCallTurn,
    callTool: async () => ({ text: "r", isError: false }),
  })) {
    last = event;
  }
  assert.deepEqual(last, { type: "done", reason: "max_iterations" });
});

test("runAgent reports a turn that ran out of tokens while reasoning", async () => {
  const history = [{ role: "user", content: "q" }];
  let last;
  for await (const event of runAgent({
    history,
    systemPrompt: "",
    tools,
    generate: async () => ({ reasoning: "still thinking", content: "", toolCalls: [], finishReason: "length" }),
    callTool: async () => assert.fail("no tool call"),
  })) {
    last = event;
  }
  assert.deepEqual(last, { type: "done", reason: "length" });
});
