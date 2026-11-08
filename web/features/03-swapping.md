---
title: Swapping models on demand
level: Basics
summary: How llama-swap decides a model is ready.
prompt: How does llama-swap know when a model has finished loading?
docs:
  - guides/model-runtime/writing-cmd
  - guides/model-runtime/client-compatibility-and-loading-feedback
  - guides/model-runtime/troubleshooting-model-wont-load
---

When a request asks for a model that is not running, llama-swap stops the
running one, starts the new one, and holds the request until the new server
answers its health check. By default it polls `/health` on the server.

```yaml
# seconds to wait for a model to become ready (default 120)
healthCheckTimeout: 300

# show "loading…" messages in chat clients while a model starts
sendLoadingState: true

models:
  qwen:
    cmd: llama-server --port ${PORT} -m /models/qwen3-8b.gguf
    # the server's health endpoint, if it is not /health
    checkEndpoint: /health
```

Clients see a slow first response and nothing else: no errors, no retries.
`sendLoadingState` sends loading progress in the reasoning field, so chat
apps like Open WebUI show that the model is starting.

If a model never becomes ready, its server log explains why. The web UI's
Logs page shows it, as does `curl localhost:8080/logs/stream/qwen`.
