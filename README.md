# site-llama-swap.ai

The website for [llama-swap.ai](https://llama-swap.ai). It is one Go binary that serves:

| Path | What it is |
|---|---|
| `/` | The landing page. |
| `/help` | The help agent. It runs [llama-swap/LFM3-1.7B-HelpAgent](https://huggingface.co/llama-swap/LFM3-1.7B-HelpAgent) in the browser with WebGPU. |
| `/api/mcp` | The documentation and source code tools the help agent calls. The same MCP protocol and tools as llama-swap's own `/api/mcp`, minus the ones that need a running llama-swap. |
| `/healthz` | Returns `ok`. |

## Running it

```shell
make build
./build/site-llama-swap -listen :8080
```

Flags:

| Flag | Default | Use |
|---|---|---|
| `-listen` | `:8080` | Address to listen on. |
| `-model` | `llama-swap/LFM3-1.7B-HelpAgent` | Hugging Face repository the browser loads the model from. |
| `-model-revision` | a pinned commit | Repository commit or branch. Pinned so a new upload does not change the site without a deploy. |
| `-transformers-url` | Transformers.js 4.3.2 on jsdelivr | The Transformers.js build the browser imports. The model card only tests 4.3.2. |

## How the help agent works

```
browser                                          site binary
───────                                          ───────────
help.js   agent loop (agent.js)  ── tools/call ──▶  /api/mcp
   │                                                  ├─ docs__*  llama-swap's docs, config example and schema
   ▼                                                  └─ code__*  llama-swap's source code
worker.js Transformers.js + WebGPU
          model files from huggingface.co (cached by the browser)
```

- The model, its weights and the conversation stay in the browser. Only tool calls reach the server.
- `worker.js` picks `q4f16` when the GPU has `shader-f16`, and `q4` otherwise. It prefills the prompt in 256 token chunks, as the model card requires.
- `agent.js` parses the model's raw `<think>` and `<tool_call>` output, which llama-server would parse on llama-swap's own Help page. It has no DOM or network code, so `make test-js` tests it under Node.
- The system prompt is llama-swap's Docs Agent prompt, which the model was fine-tuned on. The server removes the lines for `config__get_config` and `sys__now`, since there is no running llama-swap here.

## Updating llama-swap's docs and code

The doc tools live in llama-swap's `internal/` packages, which Go does not let another module import. `scripts/sync-llama-swap.sh` copies them into `internal/llamaswap/` and rewrites their imports. It also copies:

- the knowledge base, `config.example.yaml`, `README.md` and `config-schema.json`
- the Docs Agent system prompt from `ui/src/lib/prompts/docsAgent.ts`
- the Go and TypeScript source, without tests, as one tarball for the `code__*` tools

To update, check out llama-swap next to this repository and run:

```shell
make sync LLAMA_SWAP=../llama-swap
make test
```

Do not edit files under `internal/llamaswap/` by hand; the next sync replaces them. The commit they came from is `source.Commit` in `internal/llamaswap/source/embed.go`.

## Development

- `make run` starts the site on `localhost:8080`.
- `make test` runs `gofmt`, `go vet`, the Go tests and the JavaScript tests. The JavaScript tests need Node 22 or later.
- The page uses no build step. `web/static/js/vendor/` holds pinned copies of [marked](https://github.com/markedjs/marked) 15.0.12 and [DOMPurify](https://github.com/cure53/DOMPurify) 3.2.7.
