---
title: Models in containers
level: Intermediate
summary: Run servers with Docker or Podman.
prompt: How do I run a model with docker run and stop the container when it unloads?
docs:
  - guides/model-runtime/writing-cmd
  - guides/operations/container-security
---

`cmd` can be any command that starts a server, including `docker run`. Two
extra settings make containers work well:

- `cmdStop` stops the container. Without it llama-swap only stops the local
  `docker` client, and the container keeps its GPU memory.
- `proxy` is the address llama-swap forwards to, when it is not the default
  `http://localhost:${PORT}`.

```yaml
models:
  vllm-qwen:
    proxy: http://127.0.0.1:${PORT}
    cmd: |
      docker run --name ${MODEL_ID} --rm --gpus all
      -p ${PORT}:8000 -v /models:/models
      vllm/vllm-openai:latest --model /models/Qwen3-8B
    cmdStop: docker stop ${MODEL_ID}
    unloadTimeout: 30     # seconds to wait for a slow shutdown
    ttl: 600
```

`cmd` can span several lines; llama-swap joins them into one command. The
llama-swap Docker images also come with llama-server, whisper.cpp and
stable-diffusion.cpp built in, so many setups need no extra containers.
