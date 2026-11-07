# site-llama-swap.ai

The website for [llama-swap.ai](https://llama-swap.ai). It is one Go binary that serves:

| Path | What it is |
|---|---|
| `/` | The landing page. |
| `/help` | The help agent. It runs [llama-swap/LFM3-1.7B-HelpAgent](https://huggingface.co/llama-swap/LFM3-1.7B-HelpAgent) in the browser with WebGPU. |
| `/api/mcp` | The documentation and source code tools the help agent calls, served by llama-swap's `docbot` package: the same MCP protocol and tools as llama-swap's own `/api/mcp`, minus the ones that need a running llama-swap. |
| `/healthz` | Returns `ok`. |

## Running it

```shell
make
./build/site-llama-swap -listen :8080
```

`make` builds against llama-swap's latest commit on `main`, looked up on GitHub
at build time, and builds that commit's source into the binary for the
`code__*` tools. `./build/site-llama-swap -version` and the startup log show
which llama-swap commit it has. To build against a specific commit instead:

```shell
make LLAMA_SWAP_REF=<commit>
```

A plain `go build ./cmd/site` also works. It uses the llama-swap version
committed in `go.mod` and serves the `docs__*` tools only, logging a warning.

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
- The system prompt is llama-swap's Docs Agent prompt, which the model was fine-tuned on. `docbot` removes the lines for `config__get_config` and `sys__now`, since there is no running llama-swap here.

## Where llama-swap's docs and code come from

The docs and tools come from llama-swap's public
[`docbot`](https://github.com/mostlygeek/llama-swap/tree/main/docbot) package.
`make` copies `go.mod` to `build/go.mod`, moves that copy to llama-swap `main`,
and builds with it, so the committed `go.mod` does not change on every build.
For the `code__*` tools it runs docbot's `gensource` tool, which archives the
same commit's source into `internal/llamasrc/archive/` (not committed). The
docs and the code therefore always come from the same commit.

`make update-llama-swap` moves the committed `go.mod` to `main` as well. Builds
do not need it, but it keeps `go build`, `go test` and editors close to what
`make` builds.

## Development

- `make run` builds and starts the site on `localhost:8080`.
- The program is in `cmd/site`; the site itself is in `internal/server` and `web/`.
- `make test` runs `gofmt`, `go vet`, the Go tests (against llama-swap `main`, like `make`) and the JavaScript tests. The JavaScript tests need Node 22 or later.
- The page uses no build step. `web/static/js/vendor/` holds pinned copies of [marked](https://github.com/markedjs/marked) 15.0.12 and [DOMPurify](https://github.com/cure53/DOMPurify) 3.2.7.
