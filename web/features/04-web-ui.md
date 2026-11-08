---
title: The web UI
level: Basics
summary: Test models, watch requests and read logs at /ui.
prompt: What can I do in llama-swap's web UI?
docs:
  - guides/operations/observability-storage-and-activity
screenshots:
  - title: Playground
    src: https://github.com/user-attachments/assets/a79b3cea-5ee1-45f1-8db9-5f5331690e64
    alt: The Playground page, chatting with a local model.
    caption: Try any model in the browser. Tabs cover chat, images, speech, transcription, realtime, rerank and a load test.
  - title: Activity
    src: https://github.com/user-attachments/assets/145f4ece-af2f-4a45-a3c1-45ae5d3c7e7f
    alt: The Activity page, a table of recent requests with token counts and speeds.
    caption: Every request with its prompt and generated tokens, cache hits, prefill and decode speed.
  - title: Request inspector
    src: https://github.com/user-attachments/assets/947cda4f-9aa1-4fa5-a550-5c469968c1d9
    alt: A captured request and response shown side by side.
    caption: Open a request on the Activity page to see the exact request and response bodies.
  - title: Models
    src: https://github.com/user-attachments/assets/b6b850f3-c5b0-4c14-ba90-be2de25b51c7
    alt: The Models page, listing each model with load and unload buttons.
    caption: Every configured model and its state. Load or unload one by hand.
  - title: Logs
    src: https://github.com/user-attachments/assets/9bb0c362-862c-4e68-820c-4c977fc9de4e
    alt: The Logs page, streaming llama-swap and model server output.
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
