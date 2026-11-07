// Tests for web/static/js/agent.js. Run with: node --test web/test/
import { test } from "node:test";
import assert from "node:assert/strict";

import { coerceParameter, parseOutput, partialView, runAgent, toolDefinitions } from "../static/js/agent.js";

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

const toolCallOutput =
  "The user wants TTL. I should search.\n</think>\n\n" +
  "<tool_call>\n<function=docs__search_docs>\n<parameter=query>\nttl unload\n</parameter>\n" +
  "<parameter=max_results>\n3\n</parameter>\n</function>\n</tool_call><|im_end|>";

test("toolDefinitions keeps the fields llama-swap sends to llama-server", () => {
  assert.deepEqual(Object.keys(tools[0]), ["type", "function"]);
  assert.deepEqual(Object.keys(tools[0].function), ["name", "description", "parameters"]);
});

test("parseOutput splits reasoning from a tool call and types its arguments", () => {
  const parsed = parseOutput(toolCallOutput, toolsByName);
  assert.equal(parsed.reasoning, "The user wants TTL. I should search.\n");
  assert.equal(parsed.content, "");
  assert.equal(parsed.finished, true);
  assert.deepEqual(parsed.toolCalls, [{ name: "docs__search_docs", arguments: { query: "ttl unload", max_results: 3 } }]);
});

test("parseOutput reads several tool calls and a final answer", () => {
  const two =
    "thinking</think>Let me look.\n<tool_call>\n<function=docs__get_doc>\n<parameter=id>\nguides/a\n</parameter>\n</function>\n</tool_call>" +
    "\n\n<tool_call>\n<function=docs__get_doc>\n<parameter=id>\nguides/b\n</parameter>\n</function>\n</tool_call><|im_end|>";
  const parsed = parseOutput(two, toolsByName);
  assert.equal(parsed.content, "Let me look.");
  assert.deepEqual(
    parsed.toolCalls.map((c) => c.arguments.id),
    ["guides/a", "guides/b"],
  );

  const answer = parseOutput("ok</think>\n\nSet `ttl: 300`.\n\n```yaml\nmodels:\n  a:\n    ttl: 300\n```<|im_end|>");
  assert.equal(answer.toolCalls.length, 0);
  assert.equal(answer.content, "Set `ttl: 300`.\n\n```yaml\nmodels:\n  a:\n    ttl: 300\n```");
});

test("parseOutput keeps multi-line string parameters intact", () => {
  const out =
    "x</think><tool_call>\n<function=docs__search_docs>\n<parameter=query>\nline one\nline two\n</parameter>\n</function>\n</tool_call>";
  assert.equal(parseOutput(out, toolsByName).toolCalls[0].arguments.query, "line one\nline two");
});

test("parseOutput reports reasoning that never closed", () => {
  const parsed = parseOutput("still thinking about it");
  assert.equal(parsed.thinkingUnclosed, true);
  assert.equal(parsed.reasoning, "still thinking about it");
  assert.equal(parsed.toolCalls.length, 0);
});

test("coerceParameter follows the schema type", () => {
  assert.equal(coerceParameter("42", { type: "integer" }), 42);
  assert.equal(coerceParameter("42", { type: "string" }), "42");
  assert.equal(coerceParameter("true", { type: "boolean" }), true);
  assert.equal(coerceParameter("abc", { type: "integer" }), "abc");
  assert.deepEqual(coerceParameter('["a"]', undefined), ["a"]);
  assert.equal(coerceParameter("plain text", undefined), "plain text");
});

test("partialView hides tool call markup while streaming", () => {
  assert.deepEqual(partialView("reasoning so f"), {
    reasoning: "reasoning so f",
    content: "",
    thinking: true,
    callingTool: false,
  });
  const v = partialView("r</think>\n\nHere is<tool_call>\n<function=docs");
  assert.equal(v.content, "Here is");
  assert.equal(v.callingTool, true);
  assert.equal(partialView("r</think>Answer <tool_").content, "Answer");
});

test("runAgent calls tools until the model answers", async () => {
  const outputs = [toolCallOutput, "done</think>\n\nUse `ttl`.<|im_end|>"];
  const seen = [];
  const history = [{ role: "user", content: "How do I unload?" }];

  const events = [];
  for await (const event of runAgent({
    history,
    systemPrompt: "SYSTEM",
    tools,
    generate: async (messages) => {
      seen.push(messages);
      return { text: outputs.shift(), completionTokens: 10, generateMs: 100 };
    },
    callTool: async (name, args) => ({ text: `${name}:${args.query}`, isError: false }),
  })) {
    events.push(event.type);
  }

  assert.deepEqual(events, ["turn_start", "turn_end", "tool_start", "tool_end", "turn_start", "turn_end", "done"]);
  assert.equal(seen[0][0].content, "SYSTEM");

  // The second request carries the call, with object arguments as the chat
  // template requires, and its result.
  const [, , assistant, tool] = seen[1];
  assert.equal(assistant.role, "assistant");
  assert.deepEqual(assistant.tool_calls[0].function.arguments, { query: "ttl unload", max_results: 3 });
  assert.equal(assistant.reasoning_content, "The user wants TTL. I should search.\n");
  assert.deepEqual(tool, { role: "tool", tool_call_id: "call_1", name: "docs__search_docs", content: "docs__search_docs:ttl unload" });

  assert.equal(history.at(-1).content, "Use `ttl`.");
});

test("runAgent answers an unknown tool with an error the model can read", async () => {
  const outputs = ["x</think><tool_call>\n<function=config__get_config>\n</function>\n</tool_call>", "y</think>ok"];
  const history = [{ role: "user", content: "q" }];
  let ok;
  for await (const event of runAgent({
    history,
    systemPrompt: "",
    tools,
    generate: async () => ({ text: outputs.shift() }),
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
    generate: async () => ({ text: toolCallOutput }),
    callTool: async () => ({ text: "r", isError: false }),
  })) {
    last = event;
  }
  assert.deepEqual(last, { type: "done", reason: "max_iterations" });
});
