// The docs pages: copy buttons on code blocks.
import { addCopyButtons } from "./copy.js";

for (const body of document.querySelectorAll(".doc-body")) addCopyButtons(body);
