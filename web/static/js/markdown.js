// Renders the agent's Markdown answers to sanitized HTML.

import { Marked } from "./vendor/marked-15.0.12.esm.js";
import DOMPurify from "./vendor/purify-3.2.7.es.js";

const marked = new Marked({ gfm: true, breaks: false });

DOMPurify.addHook("afterSanitizeAttributes", (node) => {
  if (node.tagName === "A" && node.getAttribute("href")) {
    node.setAttribute("target", "_blank");
    node.setAttribute("rel", "noopener noreferrer");
  }
});

export function renderMarkdown(text) {
  return DOMPurify.sanitize(marked.parse(text ?? ""));
}

/** Adds a copy button to each code block inside root. */
export function addCopyButtons(root) {
  for (const pre of root.querySelectorAll("pre")) {
    if (pre.querySelector(".copy")) continue;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "copy";
    button.textContent = "Copy";
    button.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(pre.querySelector("code")?.innerText ?? pre.innerText);
        button.textContent = "Copied";
      } catch {
        button.textContent = "Failed";
      }
      setTimeout(() => (button.textContent = "Copy"), 1500);
    });
    pre.appendChild(button);
  }
}
