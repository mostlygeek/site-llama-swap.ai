---
title: Your first config
level: Basics
summary: A model is a name and the command that starts its server.
prompt: How do I write a minimal llama-swap config for two llama.cpp models?
docs:
  - guides/configuration/configuration-overview
  - guides/model-runtime/writing-cmd
---

Each entry under `models` is a model ID that clients can ask for. `cmd` is the
command that starts its server. llama-swap gives each model its own port
(from 5800 up) and puts it in `${PORT}`.

```yaml
# config.yaml
models:
  qwen:
    cmd: llama-server --port ${PORT} -m /models/qwen3-8b.gguf
  gemma:
    cmd: llama-server --port ${PORT} -m /models/gemma-3-12b.gguf
```

Start llama-swap with the file. It listens on port 8080.

```shell
llama-swap --config config.yaml

# add -watch-config to reload the file when it changes
llama-swap --config config.yaml -watch-config
```

Then call it like any OpenAI compatible server:

```shell
curl localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model": "qwen", "messages": [{"role": "user", "content": "Hi"}]}'
```

Run `llama-swap --config config.yaml -validate` to check a config without
starting the server.
