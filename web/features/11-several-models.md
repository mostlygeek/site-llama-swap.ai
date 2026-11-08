---
title: Running several models at once
level: Advanced
summary: Keep more than one model loaded, as far as your hardware allows.
prompt: How do I choose between the group, matrix and capacity routers to run several models at once?
docs:
  - guides/routing/groups-and-matrix
  - guides/routing/capacity-and-queues
tabs:
  - title: group
    body: |
      Name groups of models and say how each group behaves. Each model belongs
      to at most one group.

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

      | Setting | `true` means |
      | --- | --- |
      | `swap` | only one member of this group runs at a time |
      | `exclusive` | starting a member stops models in other groups |
      | `persistent` | other groups can never stop this group's models |
  - title: matrix
    body: |
      List the combinations of models that fit. On each request llama-swap
      picks a combination that contains the requested model and stops the
      running models that cost least to stop.

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

      In a set, `&` means "run together", `|` means "one of", and `()` groups.
      `"(g | q) & v"` allows gemma with voxtral, or qwen with voxtral. A model
      that is in no set can only run alone.
  - title: capacity
    body: |
      Declare resource limits, such as the memory of each GPU, and how much of
      each one every model uses. Any combination that fits can run; there is no
      list to write.

      ```yaml
      routing:
        router:
          use: capacity
          settings:
            capacity:
              pools:             # your own names and units, here MB
                gpu0: 24000
                ram: 64000

      models:
        qwen-32b:
          cmd: llama-server --port ${PORT} -m /models/qwen3-32b.gguf
          capacities: { gpu0: 20000, ram: 4000 }
          evict_cost: 100        # prefer stopping other models first
        embeddings:
          cmd: llama-server --port ${PORT} -m /models/bge-m3.gguf --embedding
          capacities: { gpu0: 2000 }
        whisper:
          cmd: whisper-server --port ${PORT} -m /models/whisper-large.bin
          capacities: { gpu0: 3000 }
      ```

      llama-swap does not measure memory. Use numbers that include the context
      and the server's own overhead.
---

By default llama-swap runs one model at a time: starting a model stops the
one before it. The `routing` section lets several models stay loaded together.
Pick one of three routers with `use`:

| Router | Use it when |
| --- | --- |
| `group` | your setup is "these run together, those swap out". You name groups of models. The simplest, and the default. |
| `matrix` | what fits depends on the models, such as a 70B that needs every GPU alone but several small models that fit together. You list the combinations. |
| `capacity` | you know how much memory each model needs. You give the limits and llama-swap works out what fits. |

In every router, only the requested model is started. The others start when
they are first requested.
