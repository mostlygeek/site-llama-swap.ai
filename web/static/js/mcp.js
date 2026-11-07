// A minimal client for this site's /api/mcp endpoint, which speaks the same
// stateless MCP revision as llama-swap's own: every request is one POST with
// the method and tool name repeated in headers.

const ENDPOINT = "/api/mcp";
const PROTOCOL_VERSION = "2026-07-28";

let nextId = 1;

async function rpc(method, params, toolName, signal) {
  const headers = {
    "Content-Type": "application/json",
    "Mcp-Protocol-Version": PROTOCOL_VERSION,
    "Mcp-Method": method,
  };
  if (toolName) headers["Mcp-Name"] = toolName;

  const response = await fetch(ENDPOINT, {
    method: "POST",
    headers,
    body: JSON.stringify({ jsonrpc: "2.0", id: nextId++, method, ...(params === undefined ? {} : { params }) }),
    signal,
  });
  if (!response.ok) {
    throw new Error(`MCP request failed: ${response.status} ${await response.text()}`);
  }
  const body = await response.json();
  if (body.error) {
    throw new Error(body.error.message ?? `MCP ${method} failed`);
  }
  return body.result;
}

/** Lists every tool, following pagination. */
export async function listTools(signal) {
  const tools = [];
  let cursor;
  do {
    const result = await rpc("tools/list", cursor ? { cursor } : undefined, undefined, signal);
    tools.push(...(result.tools ?? []));
    cursor = result.nextCursor;
  } while (cursor);
  return tools;
}

/**
 * Calls one tool. A tool that ran but could not answer returns isError with
 * an explanation for the model; only transport and protocol failures throw.
 */
export async function callTool(name, args, signal) {
  const result = await rpc("tools/call", { name, arguments: args ?? {} }, name, signal);
  const text = (result?.content ?? [])
    .filter((block) => block?.type === "text" && typeof block.text === "string")
    .map((block) => block.text)
    .join("\n");
  return { text, isError: result?.isError === true };
}
