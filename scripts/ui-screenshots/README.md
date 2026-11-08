# Web UI screenshots

These files make the screenshots on the Features tour's web UI slide
(`web/features/04-web-ui.md`). They run a real llama-swap whose models are
[fake-model](https://github.com/mostlygeek/llama-swap/tree/main/cmd/fake-model)
servers, which load nothing and reply with placeholder text at a set speed, so
no GPU or model files are needed.

1. Build llama-swap with its UI, and fake-model, from a llama-swap checkout:

   ```shell
   make ui
   go build -tags embed_ui -o /tmp/llama-swap .
   go build -o /tmp/fake-model ./cmd/fake-model
   ```

2. From this directory, start llama-swap and send it some requests:

   ```shell
   FAKE_MODEL=/tmp/fake-model /tmp/llama-swap -config config.yaml -listen 127.0.0.1:8080 &
   ./traffic.sh
   ```

3. Take the screenshots and convert them for the site. `shoot.js` needs
   Playwright with Chromium; `webp.py` needs Pillow.

   ```shell
   node shoot.js        # writes out/*.png
   python3 webp.py      # writes web/static/img/features/*.webp
   ```

`shoot.js` uses the UI's dark mode with the Sunset theme. It also sends one
more chat from the Playground, so the Playground
screenshot shows a reply. The Performance, Hardware and Help pages are left
out: on a machine without a GPU the first two show no GPU data, and Help
needs a model that can call tools.
