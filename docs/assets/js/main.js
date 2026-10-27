// Reva website behaviour. No dependencies.

const store = {
  get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private mode */ } },
};

// Theme: follows the system until the visitor picks one.
document.querySelectorAll("[data-theme-toggle]").forEach((btn) => {
  btn.addEventListener("click", () => {
    const root = document.documentElement;
    const dark = root.dataset.theme
      ? root.dataset.theme === "dark"
      : matchMedia("(prefers-color-scheme: dark)").matches;
    root.dataset.theme = dark ? "light" : "dark";
    store.set("theme", root.dataset.theme);
  });
});

// Disclosure buttons for the main menu and the docs sidebar on small screens.
function disclosure(selector, target) {
  const btn = document.querySelector(selector);
  const el = document.getElementById(target);
  if (!btn || !el) return;
  btn.addEventListener("click", () => {
    const open = el.classList.toggle("open");
    btn.setAttribute("aria-expanded", String(open));
  });
}
disclosure("[data-menu-toggle]", "main-nav");
disclosure("[data-sidebar-toggle]", "sidebar");

// Keep the active sidebar entry in view.
document.querySelector(".sidebar a.active")?.scrollIntoView({ block: "center" });

// Copy buttons on code blocks.
document.querySelectorAll(".content pre").forEach((pre) => {
  if (!navigator.clipboard) return;
  const wrap = document.createElement("div");
  wrap.className = "code-wrap";
  pre.parentNode.insertBefore(wrap, pre);
  wrap.appendChild(pre);
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "copy-btn";
  btn.textContent = "Copy";
  btn.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(pre.innerText.replace(/\n$/, ""));
      btn.textContent = "Copied";
    } catch (e) {
      btn.textContent = "Failed";
    }
    setTimeout(() => { btn.textContent = "Copy"; }, 1500);
  });
  wrap.appendChild(btn);
});

// Filters: settings on a component page, components on the overview.
document.querySelectorAll("[data-config-filter]").forEach((input) => {
  const rows = [...document.querySelectorAll(".config-table tr[data-key]")];
  input.addEventListener("input", () => {
    const q = input.value.trim().toLowerCase();
    rows.forEach((r) => { r.hidden = q !== "" && !r.dataset.key.toLowerCase().includes(q); });
  });
});
document.querySelectorAll("[data-list-filter]").forEach((input) => {
  const items = [...document.querySelectorAll("[data-filterable] > a")];
  input.addEventListener("input", () => {
    const q = input.value.trim().toLowerCase();
    items.forEach((a) => { a.hidden = q !== "" && !a.textContent.toLowerCase().includes(q); });
    document.querySelectorAll("[data-filterable]").forEach((list) => {
      const empty = [...list.children].every((a) => a.hidden);
      list.hidden = empty;
      const heading = list.previousElementSibling;
      if (heading && /^H[2-4]$/.test(heading.tagName)) heading.hidden = empty;
    });
  });
});

// Search over an index Hugo writes at /search.json.
(function search() {
  const dialog = document.getElementById("search");
  if (!dialog) return;
  const input = dialog.querySelector("input");
  const list = dialog.querySelector(".search-results");
  let index = null;
  let sel = 0;

  async function load() {
    if (index) return index;
    const res = await fetch("/search.json");
    index = (await res.json()).map((d) => ({ ...d, hay: (d.title + " " + d.section + " " + d.text).toLowerCase() }));
    return index;
  }

  function score(d, terms) {
    let s = 0;
    const title = d.title.toLowerCase();
    for (const t of terms) {
      if (!d.hay.includes(t)) return 0;
      if (title === t) s += 20;
      else if (title.startsWith(t)) s += 10;
      else if (title.includes(t)) s += 6;
      else s += 1;
    }
    return s + d.boost;
  }

  function excerpt(text, term) {
    const i = text.toLowerCase().indexOf(term);
    if (i < 0) return text.slice(0, 120);
    const start = Math.max(0, i - 40);
    return (start > 0 ? "…" : "") + text.slice(start, start + 140);
  }

  function esc(s) {
    return s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
  }

  async function render() {
    const q = input.value.trim().toLowerCase();
    if (!q) { list.innerHTML = ""; return; }
    const terms = q.split(/\s+/);
    const docs = await load();
    const hits = docs.map((d) => [score(d, terms), d]).filter(([s]) => s > 0)
      .sort((a, b) => b[0] - a[0]).slice(0, 20);
    sel = 0;
    list.innerHTML = hits.length
      ? hits.map(([, d], i) => `<li><a href="${d.url}"${i === 0 ? ' class="sel"' : ""}>${esc(d.title)}<span class="s">${esc(d.section)}</span><span class="x">${esc(excerpt(d.text, terms[0]))}</span></a></li>`).join("")
      : '<li class="none">No results.</li>';
  }

  function move(delta) {
    const links = [...list.querySelectorAll("a")];
    if (!links.length) return;
    links[sel]?.classList.remove("sel");
    sel = (sel + delta + links.length) % links.length;
    links[sel].classList.add("sel");
    links[sel].scrollIntoView({ block: "nearest" });
  }

  function open() {
    if (dialog.open) return;
    dialog.showModal();
    input.select();
    load();
  }

  document.querySelectorAll("[data-search-open]").forEach((b) => b.addEventListener("click", open));
  document.addEventListener("keydown", (e) => {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName);
    if ((e.key === "/" && !typing) || (e.key === "k" && (e.metaKey || e.ctrlKey))) {
      e.preventDefault();
      open();
    }
  });
  input.addEventListener("input", render);
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") { e.preventDefault(); move(1); }
    if (e.key === "ArrowUp") { e.preventDefault(); move(-1); }
    if (e.key === "Enter") { list.querySelectorAll("a")[sel]?.click(); }
  });
  dialog.addEventListener("click", (e) => { if (e.target === dialog) dialog.close(); });
})();

// Architecture diagram: describe a node on hover, play request traces.
document.querySelectorAll("[data-diagram]").forEach((root) => {
  const svg = root.querySelector("svg.arch");
  const info = root.querySelector("[data-diagram-info]");
  const intro = info.innerHTML;
  const scenarios = JSON.parse(root.querySelector("[data-scenarios]").textContent);
  const pulse = svg.querySelector(".pulse");
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  let playing = null;

  const node = (id) => svg.querySelector(`#node-${id}`);
  const edge = (a, b) => {
    const e = svg.querySelector(`#edge-${a}-${b}`);
    if (e) return { el: e, reverse: false };
    const r = svg.querySelector(`#edge-${b}-${a}`);
    return r ? { el: r, reverse: true } : null;
  };

  function clear() {
    svg.classList.remove("dim");
    svg.querySelectorAll(".lit").forEach((el) => el.classList.remove("lit"));
  }

  function describe(n) {
    if (playing) return;
    clear();
    svg.classList.add("dim");
    n.classList.add("lit");
    const id = n.dataset.id;
    svg.querySelectorAll(`.edge[data-from="${id}"], .edge[data-to="${id}"]`).forEach((e) => {
      e.classList.add("lit");
      node(e.dataset.from === id ? e.dataset.to : e.dataset.from)?.classList.add("lit");
    });
    const link = n.tagName.toLowerCase() === "a" ? `<p><a href="${n.getAttribute("href")}">Open the ${n.dataset.title} page →</a></p>` : "";
    info.innerHTML = `<h3>${n.dataset.title}</h3><p>${n.dataset.desc}</p>${link}`;
  }

  svg.querySelectorAll(".node").forEach((n) => {
    n.addEventListener("mouseenter", () => describe(n));
    n.addEventListener("focus", () => describe(n));
    n.addEventListener("mouseleave", () => { if (!playing) { clear(); info.innerHTML = intro; } });
  });

  function animate(path, reverse, ms) {
    if (reduced) return Promise.resolve();
    const len = path.getTotalLength();
    return new Promise((done) => {
      const t0 = performance.now();
      function frame(t) {
        if (!playing) return done();
        const k = Math.min(1, (t - t0) / ms);
        const p = path.getPointAtLength((reverse ? 1 - k : k) * len);
        pulse.setAttribute("cx", p.x);
        pulse.setAttribute("cy", p.y);
        if (k < 1) requestAnimationFrame(frame); else done();
      }
      requestAnimationFrame(frame);
    });
  }

  const wait = (ms) => new Promise((r) => setTimeout(r, ms));

  async function play(sc, btn) {
    const token = {};
    playing = token;
    root.querySelectorAll("[data-scenario]").forEach((b) => b.setAttribute("aria-pressed", String(b === btn)));
    clear();
    svg.classList.add("dim");
    info.innerHTML = `<h3>${sc.title}</h3><p>${sc.intro}</p><ol>${sc.steps.map((s) => `<li>${s.text}</li>`).join("")}</ol>`;
    const items = info.querySelectorAll("li");
    for (let i = 0; i < sc.steps.length; i++) {
      if (playing !== token) return;
      const s = sc.steps[i];
      items.forEach((li, j) => { li.className = j < i ? "done" : j === i ? "current" : ""; });
      node(s.from)?.classList.add("lit");
      const e = edge(s.from, s.to);
      if (e) {
        e.el.classList.add("lit");
        await animate(e.el, e.reverse, 900);
      }
      node(s.to)?.classList.add("lit");
      await wait(reduced ? 1200 : 700);
    }
    if (playing !== token) return;
    items.forEach((li) => { li.className = "done"; });
    pulse.setAttribute("cx", -20);
    playing = null;
    btn.setAttribute("aria-pressed", "false");
  }

  root.querySelectorAll("[data-scenario]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const sc = scenarios.find((s) => s.id === btn.dataset.scenario);
      if (btn.getAttribute("aria-pressed") === "true") {
        playing = null;
        btn.setAttribute("aria-pressed", "false");
        pulse.setAttribute("cx", -20);
        clear();
        info.innerHTML = intro;
        return;
      }
      play(sc, btn);
    });
  });

  // A link to #trace=<scenario> starts that trace, so a trace can be shared.
  const m = location.hash.match(/^#trace=([\w-]+)$/);
  const btn = m && root.querySelector(`[data-scenario="${m[1]}"]`);
  if (btn) {
    root.scrollIntoView({ block: "start" });
    btn.click();
  }
});
