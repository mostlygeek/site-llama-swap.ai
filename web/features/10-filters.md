---
title: Rewriting requests with filters
level: Intermediate
summary: Remove or force request parameters per model.
prompt: How do I force a temperature for a model no matter what the client sends?
docs:
  - guides/api-integration/filters-and-request-rewriting
  - guides/api-integration/set-if-undefined
---

Filters change the JSON request body before it reaches the model's server.

```yaml
models:
  qwen:
    cmd: llama-server --port ${PORT} -m /models/qwen3-8b.gguf
    filters:
      # delete these keys from the request
      stripParams: "top_k, min_p"
      setParams:
        temperature: 0.7      # always this value
        max_tokens?: 4096     # "?" sets it only when the client did not
```

`setParamsByID` goes further: it creates extra model IDs that share one loaded
model and differ only in the parameters they set. Switching between them does
not reload anything.

```yaml
models:
  gpt-oss:
    cmd: llama-server --port ${PORT} -m /models/gpt-oss-20b.gguf
    filters:
      setParamsByID:
        "${MODEL_ID}:high":
          chat_template_kwargs: { reasoning_effort: high }
        "${MODEL_ID}:low":
          chat_template_kwargs: { reasoning_effort: low }
```

Clients can now ask for `gpt-oss:high` or `gpt-oss:low`. The `model` field
itself can never be changed by a filter.
