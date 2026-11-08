---
title: Macros
level: Intermediate
summary: Write a long command once and reuse it.
prompt: How do I avoid repeating a long command for every model?
docs:
  - guides/configuration/macros
---

Model commands are often long and mostly the same. A macro is a named piece of
text you insert with `${name}`.

```yaml
macros:
  llama: /opt/llama.cpp/build/bin/llama-server --port ${PORT} -ngl 99
  ctx: 8192
  models_dir: ${env.HOME}/models   # read an environment variable

models:
  qwen:
    cmd: ${llama} -m ${models_dir}/qwen3-8b.gguf --ctx-size ${ctx}
  qwen-long:
    macros:
      ctx: 32768                     # overrides the global value here only
    cmd: ${llama} -m ${models_dir}/qwen3-8b.gguf --ctx-size ${ctx}
```

Rules to know:

- A macro can use other macros, but only ones defined above it.
- `PORT`, `MODEL_ID` and `PID` are built in and cannot be redefined.
- `${env.NAME}` reads an environment variable, which keeps paths and secrets
  out of the file.
