---
title: Unloading idle models
level: Basics
summary: Free GPU memory after a model sits unused.
prompt: How do I unload a model after five minutes idle?
docs:
  - guides/model-runtime/ttl-and-unloading
---

By default a model stays loaded until another model needs its place. Set a
`ttl` (time to live, in seconds) to unload it after it has been idle that long.
Each request resets the timer.

```yaml
# default for every model; 0 means never unload
globalTTL: 600

models:
  qwen:
    cmd: llama-server --port ${PORT} -m /models/qwen3-8b.gguf
    ttl: 300   # unload after 5 minutes idle
  embeddings:
    cmd: llama-server --port ${PORT} -m /models/bge-m3.gguf --embedding
    ttl: 0     # never unload
```

| `ttl` | Meaning |
| --- | --- |
| not set, or `-1` | use `globalTTL` |
| `0` | never unload automatically |
| more than `0` | unload after this many idle seconds |

To unload by hand, use the Models page in the UI, or
`curl -X POST localhost:8080/api/models/unload`.
