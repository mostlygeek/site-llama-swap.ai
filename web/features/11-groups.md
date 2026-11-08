---
title: Several models at once with groups
level: Advanced
summary: Keep small models loaded next to a large one.
prompt: How can I keep several models loaded at once?
docs:
  - guides/routing/groups-and-matrix
---

llama-swap runs one model at a time unless you tell it otherwise. Groups say
which models can run together. Each model belongs to at most one group.

```yaml
routing:
  router:
    use: group
    settings:
      groups:
        chat:              # the big models: one at a time
          swap: true
          exclusive: true
          members: [qwen, gemma, gpt-oss]
        helpers:           # small models that can all run together
          swap: false
          exclusive: false
          members: [embeddings, reranker]
        always-on:         # nothing else can unload these
          persistent: true
          swap: false
          exclusive: false
          members: [whisper]
```

The three settings:

| Setting | `true` means |
| --- | --- |
| `swap` | only one member of this group runs at a time |
| `exclusive` | starting a member stops models in other groups |
| `persistent` | other groups can never stop this group's models |
