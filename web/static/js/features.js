// The Features tour: moves between slides without reloading the page.
// The server renders every slide and hides all but one, so each slide also
// has its own URL that works on its own.
import { addCopyButtons } from "./copy.js";

const slides = [...document.querySelectorAll(".slide")];
const bySlug = new Map(slides.map((s) => [s.dataset.slug, s]));
const topic = document.getElementById("deck-topic");
const prev = document.getElementById("deck-prev");
const next = document.getElementById("deck-next");
const count = document.getElementById("deck-count");
const progress = document.getElementById("deck-progress");

let current = slides.find((s) => !s.hidden) ?? slides[0];

for (const body of document.querySelectorAll(".slide-body")) addCopyButtons(body);

function setLink(link, slug) {
  if (slug) {
    link.href = `/features/${slug}`;
    link.removeAttribute("aria-disabled");
    link.dataset.go = slug;
  } else {
    link.removeAttribute("href");
    link.setAttribute("aria-disabled", "true");
    delete link.dataset.go;
  }
}

function show(slug, { push = true } = {}) {
  const slide = bySlug.get(slug);
  if (!slide) return;
  if (slide !== current) {
    current.hidden = true;
    slide.hidden = false;
    current = slide;
    window.scrollTo({ top: 0 });
  }
  topic.value = slug;
  setLink(prev, slide.dataset.prev);
  setLink(next, slide.dataset.next);
  count.textContent = `${slide.dataset.number} of ${slides.length}`;
  progress.style.setProperty("--n", slide.dataset.number);
  document.title = slide.dataset.title;
  if (push && location.pathname !== `/features/${slug}`) {
    history.pushState({ slug }, "", `/features/${slug}`);
  }
}

// Back, Next, and the links at the foot of each slide.
document.addEventListener("click", (event) => {
  const link = event.target.closest("a[data-go]");
  if (!link || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  show(link.dataset.go);
});
for (const link of [prev, next]) {
  if (link.getAttribute("href")) link.dataset.go = link.getAttribute("href").split("/").pop();
}

topic.addEventListener("change", () => show(topic.value));

document.addEventListener("keydown", (event) => {
  if (event.altKey || event.ctrlKey || event.metaKey) return;
  if (event.target.closest("input, textarea, select, [contenteditable]")) return;
  if (event.key === "ArrowLeft" && current.dataset.prev) show(current.dataset.prev);
  if (event.key === "ArrowRight" && current.dataset.next) show(current.dataset.next);
});

window.addEventListener("popstate", () => {
  const slug = location.pathname.replace(/^\/features\/?/, "") || slides[0].dataset.slug;
  show(slug, { push: false });
});
history.replaceState({ slug: current.dataset.slug }, "");

// Screenshot tabs: show one screenshot at a time.
for (const shots of document.querySelectorAll("[data-shots]")) {
  shots.classList.add("js");
  const tabs = [...shots.querySelectorAll(".shot-tab")];
  const figures = [...shots.querySelectorAll(".shot")];
  const select = (i) => {
    tabs.forEach((t, j) => t.setAttribute("aria-selected", String(i === j)));
    figures.forEach((f, j) => (f.hidden = i !== j));
  };
  tabs.forEach((tab, i) => tab.addEventListener("click", () => select(i)));
  select(0);
}
