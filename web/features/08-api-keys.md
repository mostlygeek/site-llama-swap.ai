---
title: API keys
level: Intermediate
summary: Require a key for every request.
prompt: How do I require an API key?
docs:
  - guides/api-integration/api-keys-and-auth
  - guides/api-integration/cors
---

llama-swap accepts any request by default. Add `apiKeys` and every request,
including the web UI and `/api/` routes, needs one of them.

```yaml
apiKeys:
  - "${env.LLAMA_SWAP_KEY}"     # keep the key out of the file
  - "sk-old-key-being-rotated"
```

Clients send the key as `Authorization: Bearer <key>`, `x-api-key: <key>`, or
HTTP Basic auth. All keys have the same access. To rotate a key, add the new
one, move clients to it, then remove the old one.

Make a random key:

```shell
printf "sk-%s\n" "$(head -c 48 /dev/urandom | base64)"
```

An API key does not make it safe to put llama-swap on the internet: it starts
processes on your machine. Keep it behind a firewall or VPN.
