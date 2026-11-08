---
title: Preloading at startup
level: Advanced
summary: Have models ready before the first request.
prompt: How do I load a model when llama-swap starts?
docs:
  - guides/operations/startup-preloading-and-hooks
---

The `on_startup` hook loads models, and can pick a profile, as soon as
llama-swap starts. The first request then does not wait for a model to load.

```yaml
hooks:
  on_startup:
    profile: coding       # activate this profile at startup and on reload
    preload:
      - qwen
      - embeddings

routing:
  router:
    use: group
    settings:
      groups:
        warm:
          swap: false
          exclusive: false
          members: [qwen, embeddings]
```

Preloading several models only keeps them all loaded if they are allowed to run
together. Without the group above, loading `embeddings` would unload `qwen`.
Give preloaded models `ttl: 0` if they should never unload.
