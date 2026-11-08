---
title: Names, aliases and listings
level: Intermediate
summary: Control what clients see in /v1/models.
prompt: How do I make a model answer to another name, like gpt-4o-mini?
docs:
  - guides/model-runtime/capabilities-and-model-listings
  - guides/configuration/configuration-overview
---

Clients find models with `GET /v1/models`. These settings change what they
see and which names a model answers to:

```yaml
models:
  qwen:
    cmd: llama-server --port ${PORT} -m /models/qwen3-8b.gguf
    name: "Qwen3 8B"                # display name in listings and the UI
    description: "Fast general chat model"
    aliases:
      - gpt-4o-mini                 # requests for this name go to qwen
    useModelName: "qwen3-8b"        # model name sent to the upstream server

  draft-model:
    cmd: llama-server --port ${PORT} -m /models/qwen3-0.6b.gguf
    unlisted: true                  # works, but is not listed
```

Aliases help with clients that only offer a fixed list of model names. Set
`includeAliasesInList: true` at the top level to list aliases too.
