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

export { addCopyButtons } from "./copy.js";
