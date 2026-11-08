---
title: The web UI
level: Basics
summary: Test models, watch requests and read logs at /ui.
prompt: What can I do in llama-swap's web UI?
docs:
  - guides/operations/observability-storage-and-activity
screenshots:
  - title: Playground
    src: img/features/playground.webp
    alt: The Playground page, with a chat sent to the qwen3-8b model and its reply, showing prompt and generation speeds.
    caption: Try any model in the browser. Tabs cover chat, images, speech, transcription, realtime, rerank and a load test. The model in these screenshots is a test server that replies with placeholder text.
  - title: Activity
    src: img/features/activity.webp
    alt: The Activity page, with histograms of prompt and generation speed above a table of recent requests with token counts and speeds.
    caption: Every request with its prompt and generated tokens, cache hits, prefill and decode speed.
  - title: Request inspector
    src: img/features/inspect.webp
    alt: The capture of one chat request, showing its headers and JSON body.
    caption: Select View on an Activity row to see the exact request and response, headers included.
  - title: Models
    src: img/features/models.webp
    alt: The Models page, listing five models with their aliases, two of them ready and three stopped.
    caption: Every configured model, its aliases and its state. Load or unload one by hand.
  - title: Logs
    src: img/features/logs.webp
    alt: The Logs page, with llama-swap's log beside the model servers' log showing models loading and shutting down.
    caption: Live output from llama-swap and from each model's server.
---

llama-swap serves a web UI at `http://localhost:8080/ui`. Nothing to install
or configure. The sidebar has these pages:

- **Activity**: recent requests and their token metrics.
- **Playground**: chat and other tests against any model.
- **Models**: model state, with load and unload buttons.
- **Logs**: live logs from llama-swap and each model.
- **Performance** and **Hardware**: speed over time, and GPU and memory use.
- **Help**: a chat that answers questions about llama-swap and your config.

The UI can tag Activity rows with a session ID read from request headers:

```yaml
ui:
  activity:
    session_id: ["X-Session-ID"]
```
