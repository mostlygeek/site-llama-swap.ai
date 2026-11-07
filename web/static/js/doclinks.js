// Links the document ids the Help agent names in its answers, such as
// guides/model-runtime/ttl-and-unloading or reference/config/models, to their
// pages on the site.

const ID = /\b(?:guides|examples|tutorials|reference)\/[A-Za-z0-9_./-]*[A-Za-z0-9_]/g;

function urlFor(text, links) {
  const id = text.replace(/^kb\//, "").replace(/\.md$/, "");
  return links[id];
}

function makeLink(url) {
  const a = document.createElement("a");
  a.href = url;
  a.target = "_blank";
  a.rel = "noopener";
  return a;
}

/**
 * Wraps known ids under root in links. Inline code that is exactly an id is
 * linked whole; ids in plain text are linked where they appear. Code blocks
 * and existing links are left alone.
 */
export function linkDocs(root, links) {
  if (!links) return;

  for (const code of root.querySelectorAll("code")) {
    if (code.closest("pre, a")) continue;
    const url = urlFor(code.textContent.trim(), links);
    if (url) {
      const a = makeLink(url);
      code.replaceWith(a);
      a.append(code);
    }
  }

  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) =>
      node.parentElement.closest("pre, code, a") ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT,
  });
  const nodes = [];
  while (walker.nextNode()) nodes.push(walker.currentNode);

  for (const node of nodes) {
    const text = node.textContent;
    ID.lastIndex = 0;
    if (!ID.test(text)) continue;
    ID.lastIndex = 0;
    const parts = document.createDocumentFragment();
    let last = 0;
    for (const m of text.matchAll(ID)) {
      const url = urlFor(m[0], links);
      if (!url) continue;
      parts.append(text.slice(last, m.index));
      const a = makeLink(url);
      a.textContent = m[0];
      parts.append(a);
      last = m.index + m[0].length;
    }
    if (last === 0) continue;
    parts.append(text.slice(last));
    node.replaceWith(parts);
  }
}
