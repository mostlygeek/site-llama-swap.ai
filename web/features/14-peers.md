---
title: Peers and remote models
level: Advanced
summary: Use models on other machines and cloud APIs through one address.
prompt: How do I use models on another computer?
docs:
  - guides/connectivity/remote-peers
  - examples/peers-and-multi-host
---

A peer is another llama-swap, or any OpenAI compatible API, that this
llama-swap forwards to. Its models appear as `peer-name/model-id` next to your
local models.

```yaml
peers:
  gpu-box:
    proxy: http://192.168.1.23:8080
    apiKey: ${env.GPU_BOX_KEY}
    models: [qwen3-235b, flux-dev]

  openrouter:
    proxy: https://openrouter.ai/api
    apiKey: ${env.OPENROUTER_API_KEY}
    models: [deepseek/deepseek-v3.2]
    filters:
      setParams:
        provider: { data_collection: deny }
```

Clients now ask for `gpu-box/qwen3-235b` or `openrouter/deepseek/deepseek-v3.2`
at the same address they use for local models. Peers accept the same
`filters` as models.

Combine peers with a `spillover` selector to use a local model first and
move extra requests to a remote one when it is busy.
