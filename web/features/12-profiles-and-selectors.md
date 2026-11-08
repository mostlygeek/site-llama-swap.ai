---
title: Profiles and selectors
level: Advanced
summary: Change which real model a name points to.
prompt: What is the difference between profiles and selectors, and when should I use each?
docs:
  - guides/routing/profiles-and-selectors
---

Both let clients ask for a fixed name and get a different real model.

A **profile** is a set of name mappings that you switch on by hand, from the
web UI or the API. Your editor always asks for `llm-code`; the active profile
decides which model that is.

```yaml
profiles:
  coding:
    description: "Coding models"
    pins:
      llm-code: qwen3-coder-30b
      llm-chat: gpt-oss-120b
  writing:
    pins:
      llm-code: gemma-3-27b
      llm-chat: gemma-3-27b
```

```shell
curl -X PUT localhost:8080/api/profiles/active -d '{"name": "coding"}'
```

A **selector** is a name that is resolved on every request. With the `warm`
strategy it uses whichever target is already loaded, so you do not wait for a
swap.

```yaml
selectors:
  any-coder:
    strategy: warm        # also: pin, spillover
    targets: [qwen3-coder-30b, gpt-oss-120b]
```
