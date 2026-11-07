// Copy buttons for code blocks, used on the Help and docs pages.

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
