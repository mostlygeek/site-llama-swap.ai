---
title: What llama-swap does
level: Basics
summary: One API in front of all your local models.
prompt: What is llama-swap and when should I use it?
docs:
  - guides/configuration/configuration-overview
---

Inference servers like llama-server and vLLM load one model each. To use
several models you would run several servers, on several ports, and start and
stop them yourself.

llama-swap is a proxy that does this for you. Clients talk to one address. Each
request names a model in its `model` field, and llama-swap:

1. starts the server for that model, if it is not running,
2. stops the server it replaces, so the GPU memory is free,
3. forwards the request and streams the response back.

```
client ── "model": "qwen" ──▶ llama-swap :8080 ──▶ llama-server (qwen)
                                           └─ stops llama-server (gemma)
```

It is one Go binary and one YAML file, with no other dependencies. It works with
any server that speaks the OpenAI or Anthropic API: llama.cpp and its forks,
vLLM, stable-diffusion.cpp, whisper.cpp, ComfyUI and others.
