# site-llama-swap.ai

The website for [llama-swap.ai](https://llama-swap.ai). It is one Go binary that serves:

| Path | What it is |
|---|---|
| `/` | The landing page. |
| `/docs/chat` | Chat with the docs (the help agent; `/help` redirects here). It runs the Q4_K_M GGUF of [llama-swap/LFM3-1.7B-HelpAgent](https://huggingface.co/llama-swap/LFM3-1.7B-HelpAgent) in the browser with [wllama](https://github.com/ngxson/wllama) and WebGPU. |
| `/features` | A tour of llama-swap's features, one slide each, from a first config to the advanced routing settings. `/features/<slug>` opens one slide. |
| `/docs` | llama-swap's documentation as HTML: the knowledge base, a configuration reference from `config.example.yaml`, and search. |
| `/releases` | llama-swap's release notes as HTML, from the `changelog` package: the index lists every release, newest first, and `/releases/<version>` shows one release. The old `/changelog` paths redirect here. |
| `/api/mcp` | The documentation and source code tools the help agent calls, served by llama-swap's `docbot` package: the same MCP protocol and tools as llama-swap's own `/api/mcp`, minus the ones that need a running llama-swap. |
| `/health` | Returns `ok`. |

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
| `-model-file` | `LFM3-1.7B-HelpAgent-Q4_K_M.gguf` | GGUF file in the model repository. |
| `-tls-cert-file`, `-tls-key-file` | none | Serve HTTPS with this certificate and key. Set both or neither. |

## HTTPS for development

Browsers only turn on WebGPU for pages served over HTTPS or from
`http://localhost`. If you open the site on another machine by name, such as
`http://devbox:8181`, the help agent reports that WebGPU needs HTTPS. Either
open it through an SSH tunnel (`ssh -L 8181:localhost:8181 devbox`, then
`http://localhost:8181/docs/chat`), or serve it over HTTPS with a development
certificate as below.

[mkcert](https://github.com/FiloSottile/mkcert) makes a certificate signed by
your own local certificate authority (CA), which your browser can trust
without warnings.

1. On the machine that runs the site, install mkcert and make a certificate
   for every name you will open the site with. `certs/` is gitignored.

   ```shell
   go install filippo.io/mkcert@v1.4.4   # or: brew install mkcert
   mkdir -p certs
   mkcert -cert-file certs/dev-cert.pem -key-file certs/dev-key.pem devbox localhost 127.0.0.1
   ```

2. Start the site with the certificate:

   ```shell
   make
   ./build/site-llama-swap -listen :8443 -tls-cert-file certs/dev-cert.pem -tls-key-file certs/dev-key.pem
   ```

3. Trust the CA on the computer that runs the browser. If that is the same
   machine, run `mkcert -install`. If it is another computer, copy only
   `rootCA.pem` from the directory `mkcert -CAROOT` prints, then on that
   computer run:

   ```shell
   CAROOT=/path/to/folder/with/rootCA.pem mkcert -install
   ```

   Never copy `rootCA-key.pem`: anyone with it can make certificates your
   browser trusts for any site.

4. Restart the browser and open `https://devbox:8443/docs/chat`.

If the browser still warns about the certificate, check that the name in the
address bar is one you passed to `mkcert` in step 1. Make a new certificate
to add a name.

## How the help agent works

```
browser                                          site binary
───────                                          ───────────
help.js   agent loop (agent.js)  ── tools/call ──▶  /api/mcp
   │                                                  ├─ docs__*  llama-swap's docs, config example and schema
   ▼                                                  └─ code__*  llama-swap's source code
worker.js wllama (llama.cpp in WebAssembly) + WebGPU
          the GGUF from huggingface.co (cached by the browser)
```

- The model, its weights and the conversation stay in the browser. Only tool calls reach the server.
- `worker.js` downloads the Q4_K_M GGUF (1.1 GB) into Cache Storage, resuming a partial download (`download.js`), and loads it with wllama with all layers on the GPU.
- llama.cpp applies the chat template and parses the model's reasoning and tool calls, as `llama-server --jinja` does on llama-swap's own Help page. `model.js` uses the model card's sampling settings (temperature 0.1, top-k 50, repeat penalty 1.1) and a 32K token context. The conversation stays in llama.cpp's cache between turns, so each turn reads only the new tokens.
- `agent.js` runs the loop of model turns and tool calls. It has no DOM or network code, so `make test-js` tests it under Node.
- The site sends `Cross-Origin-Opener-Policy` and `Cross-Origin-Embedder-Policy` headers, so the page is cross-origin isolated and wllama can use several threads.
- The system prompt is llama-swap's Docs Agent prompt, which the model was fine-tuned on. `docbot` removes the lines for `config__get_config` and `sys__now`, since there is no running llama-swap here.

## How the docs pages work

`internal/docsite` renders llama-swap's documentation once, when the site
starts, from the `docs` package of the llama-swap version the site is built
with:

- Each knowledge base article in `docs/kb/` becomes `/docs/<id>`, using the
  same ids as the Help agent's tools (`/docs/guides/model-runtime/ttl-and-unloading`).
  Its front matter gives the title, summary and navigation group.
- [goldmark](https://github.com/yuin/goldmark) turns the Markdown into HTML,
  with GitHub-style tables, heading anchors, and code coloured by
  [Chroma](https://github.com/alecthomas/chroma) for light and dark themes.
  Raw HTML in the Markdown is escaped.
- Links between documents are rewritten to the site's pages. Links to other
  files in the llama-swap repository go to GitHub.
- `/docs/reference/config` shows `config.example.yaml` split at its top-level
  keys, each with an anchor (`#models`).
- `/docs/search` runs the Help agent's own `docs__search_docs` tool, so the two
  find the same things.
- Document ids in the Help agent's answers link to these pages.

## How the releases page works

`internal/changelogsite` renders llama-swap's release notes once, when the
site starts, from the `changelog` package of the llama-swap version the site
is built with, with the same goldmark rendering as the docs pages. `/releases`
lists every release, newest first; `/releases/<version>` shows one release
note, in the docs page layout, and accepts the version with or without the
leading `v`.

## How the features tour works

Each slide is a Markdown file in `web/features/`, named `NN-slug.md`: the
number sets the order and the slug is the URL, `/features/<slug>`. Its front
matter sets the title, the level (`Basics`, `Intermediate` or `Advanced`;
slides go from basics to advanced), the question the "Ask the llama" button
puts in the chat, the related docs pages by knowledge base id, and optional
screenshots. `internal/featuresite` renders them once, at startup, with the
same goldmark and Chroma setup as the docs pages.

Every slide is in the page, and `features.js` shows one at a time, so Back,
Next, the topic menu and the arrow keys change slides without a reload while
keeping the URL and browser history in step. "Ask the llama" links to
`/docs/chat?q=<question>`; the chat page fills its question box from `q`
without sending it.

The web UI slide's screenshots are in `web/static/img/features/`. They are
taken from a real llama-swap whose models are fake-model test servers;
`scripts/ui-screenshots/` has the config, the request traffic and the
Playwright script that make them, and its README explains how to take them
again when the UI changes.

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

## Deploying to Fly.io

The site runs on [Fly.io](https://fly.io) from the `Dockerfile` and
`fly.toml` in this repository. The image is about 30 MB: the Go binary on a
distroless base. It builds with `make build`, so each deploy carries
llama-swap's latest `main`.

`.github/workflows/deploy.yml` runs `make test` on every pull request and
push. When `main` changes and the tests pass, it deploys with `flyctl deploy`.
To pick up new llama-swap docs without a change here, run the workflow by hand
from the Actions tab.

One-time setup:

1. Create the app. Change `app` and `primary_region` in `fly.toml` if you
   want others.

   ```shell
   fly apps create llama-swap-ai
   ```

2. Make a deploy token and save it in the GitHub repository as the Actions
   secret `FLY_API_TOKEN` (Settings → Secrets and variables → Actions).

   ```shell
   fly tokens create deploy -a llama-swap-ai
   ```

3. Point the domain at the app and follow the DNS instructions it prints.

   ```shell
   fly certs add llama-swap.ai -a llama-swap-ai
   ```

To deploy from your own machine instead: `fly deploy --build-arg VERSION=$(git rev-parse --short HEAD)`.

Fly.io serves the site over HTTPS, which the help agent needs for WebGPU.
Machines stop when idle and start on the next request; the site starts in
well under a second.

## Development

- `make run` builds and starts the site on `localhost:8080`.
- The program is in `cmd/site`; the site itself is in `internal/server` and `web/`.
- `make test` runs `gofmt`, `go vet`, the Go tests (against llama-swap `main`, like `make`) and the JavaScript tests. The JavaScript tests need Node 22 or later.
- The page uses no build step. `web/static/js/vendor/` holds pinned copies of [marked](https://github.com/markedjs/marked) 15.0.12, [DOMPurify](https://github.com/cure53/DOMPurify) 3.2.7 and wllama (see below).

## The wllama build

The model's `lfm3` architecture needs llama.cpp b11960 or later. The newest
wllama release on npm (3.8.1) bundles an older llama.cpp, so
`web/static/js/vendor/wllama/` holds a build of wllama's `main` branch
instead. `VERSION` in that directory names the wllama and llama.cpp commits.
It is the main build only, which needs WebGPU, JSPI and 64-bit WebAssembly
memory (Chrome and Edge). The compatibility build for other browsers is not
included.

To rebuild it, with [emsdk](https://emscripten.org/docs/getting_started/downloads.html)
4.0.20 active:

```shell
git clone https://github.com/ngxson/wllama && cd wllama
git checkout <commit> && git submodule update --init --depth 1 llama.cpp
npm ci
SKIP_COMPAT=1 ./scripts/build_wasm.sh   # or the emcmake steps in scripts/docker-compose.yml
npm run build:worker && npx tsup src/index.ts --format esm --clean && ./scripts/post_build.sh
cp esm/index.js esm/wasm/wllama.wasm <this repo>/web/static/js/vendor/wllama/
```

Once a wllama release on npm includes LFM3 support, the vendored copy can
be replaced by that release.
