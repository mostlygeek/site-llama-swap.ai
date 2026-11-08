---
title: The matrix router
level: Advanced
summary: List the model combinations that fit, and let llama-swap choose.
prompt: How do I use the matrix router to describe which models fit in VRAM together?
docs:
  - guides/routing/groups-and-matrix
  - guides/routing/capacity-and-queues
---

Groups can not describe every setup. The matrix router takes a list of model
combinations that fit on your hardware. On each request it picks the
combination that contains the requested model and stops the fewest, or
cheapest, running models.

```yaml
routing:
  router:
    use: matrix
    settings:
      matrix:
        vars:              # short names for use in sets
          g: gemma
          q: qwen
          v: voxtral
        evict_costs:       # how costly a model is to stop (default 1)
          v: 50            # slow to start again, so avoid stopping it
        sets:
          chat_and_voice: "(g | q) & v"
          big:            "llama-70b"
```

In set expressions, `&` means "run together", `|` means "one of", and `()`
groups. `"(g | q) & v"` allows gemma with voxtral, or qwen with voxtral. A
model that is in no set can only run alone.

A third router, `capacity`, needs no list: you declare resource limits, such as
the memory of each GPU, and how much each model uses. Any combination that fits
can run.
