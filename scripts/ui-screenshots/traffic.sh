#!/usr/bin/env bash
# Sends chat requests to llama-swap on :8080 so the Activity, Models and
# Logs pages have something to show. Takes about a minute and a half.
set -u
req() { # model stream session
  curl -s -o /dev/null localhost:8080/v1/chat/completions -H 'Content-Type: application/json' -H "X-Session-ID: $3" \
    -d "{\"model\":\"$1\",\"stream\":$2,\"messages\":[{\"role\":\"system\",\"content\":\"You are a helpful assistant.\"},{\"role\":\"user\",\"content\":\"Explain how llama-swap swaps models, with an example config.\"}]}"
}
req bge-m3 false rag-indexer
req qwen3-8b true chat-ui-7f3a
req qwen3-8b true chat-ui-7f3a &
req gpt-4o-mini false n8n-flow &
wait
req qwen3-coder-30b true vscode-continue
req qwen3-coder-30b true vscode-continue &
req qwen3-coder-30b true opencode-2c1 &
wait
req gpt-oss-120b:high true chat-ui-7f3a
req gpt-oss-120b:low false n8n-flow
req gemma-3-12b true open-webui
req qwen3-8b true chat-ui-7f3a
req qwen3-coder-30b true vscode-continue
