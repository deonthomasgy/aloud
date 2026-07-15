"use strict";

// ---------- helpers ----------
const app = document.getElementById("app");
const player = document.getElementById("player");
const highlighter = window.AloudHighlight;
const el = (tag, attrs = {}, ...kids) => {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") n.className = v;
    else if (k === "html") n.innerHTML = v;
    else if (k.startsWith("on")) n.addEventListener(k.slice(2), v);
    else if (v !== null && v !== undefined && v !== false) n.setAttribute(k, v);
  }
  for (const kid of kids) if (kid != null) n.append(kid);
  return n;
};

async function api(method, path, body, isForm) {
  const opts = { method, headers: {} };
  if (body && isForm) opts.body = body;
  else if (body) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
  const res = await fetch(path, opts);
  if (res.status === 204) return null;
  const ct = res.headers.get("content-type") || "";
  const data = ct.includes("json") ? await res.json() : await res.text();
  if (!res.ok) throw new Error((data && data.error) || ("Request failed (" + res.status + ")"));
  return data;
}

let VOICES = null;
async function voices() { if (!VOICES) VOICES = await api("GET", "/api/voices"); return VOICES; }
// "Heart (F)" -> "Heart ♀", "Adam (M)" -> "Adam ♂" (drop the raw voice id).
function voiceLabel(name) { return name.replace(/\s*\(F\)\s*$/, " ♀").replace(/\s*\(M\)\s*$/, " ♂"); }
function voiceSelect(selected, cls) {
  const sel = el("select", { class: cls || "" });
  for (const g of VOICES.groups) {
    const og = el("optgroup", { label: g.language });
    for (const v of g.voices) {
      const o = el("option", { value: v.id }, voiceLabel(v.name));
      if (v.id === (selected || VOICES.default)) o.selected = true;
      og.append(o);
    }
    sel.append(og);
  }
  return sel;
}

const ICON = {
  play: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>',
  pause: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>',
  prev: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/></svg>',
  next: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M16 6h2v12h-2zM6 18l8.5-6L6 6z"/></svg>',
};
const logoSvg = '<span class="dot"><svg viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"></polygon><path d="M15.54 8.46a5 5 0 0 1 0 7.07"></path><path d="M19.07 4.93a10 10 0 0 1 0 14.14"></path></svg></span>';

function topbar(...mid) {
  const logo = el("div", { class: "logo", html: logoSvg + "Aloud", onclick: () => { location.hash = "#/"; } });
  const health = el("div", { class: "health", id: "health", html: '<span class="ds"></span><span id="healthText">…</span>' });
  return el("div", { class: "topbar" }, logo, ...mid, el("div", { class: "spacer" }), health);
}

// ---------- router ----------
window.addEventListener("hashchange", route);
window.addEventListener("DOMContentLoaded", async () => { await voices(); route(); });

function route() {
  stopPlayback();
  clearSpeechCache();
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  const m = location.hash.match(/^#\/projects\/(\d+)/);
  if (m) renderProject(parseInt(m[1], 10));
  else renderHome();
  checkHealth();
}

// ---------- home ----------
async function renderHome() {
  app.innerHTML = "";
  app.append(topbar());
  const home = el("div", { class: "home" });
  app.append(home);

  home.append(
    el("h1", {}, "Your library"),
    el("div", { class: "sub" }, "Create a project, add photos of book pages or text, then listen and read along."),
  );

  // Create panel
  const speedVal = el("span", { class: "muted small" }, "1.00×");
  const speed = el("input", { type: "range", min: "0.5", max: "2", step: "0.05", value: "1",
    oninput: (e) => speedVal.textContent = (+e.target.value).toFixed(2) + "×" });
  const title = el("input", { type: "text", placeholder: "e.g. A Tale of Two Cities — Ch. 1" });
  const vsel = voiceSelect();
  const cstatus = el("div", { class: "status" });
  home.append(el("div", { class: "panel" },
    el("h2", {}, "New project"),
    el("div", { class: "create-grid" },
      el("div", { class: "field" }, el("label", {}, "Title"), title),
      el("div", { class: "field" }, el("label", {}, "Default voice"), vsel),
      el("div", { class: "field full", style: "flex-direction:row; align-items:center; gap:14px" },
        el("label", { style: "min-width:46px" }, "Speed"), speed, speedVal,
        el("div", { style: "flex:1" }),
        el("button", { class: "btn", onclick: async () => {
          cstatus.textContent = "Creating…"; cstatus.classList.remove("error");
          try {
            const p = await api("POST", "/api/projects", { title: title.value.trim(), voice: vsel.value, speed: parseFloat(speed.value), format: "mp3" });
            location.hash = "#/projects/" + p.id;
          } catch (e) { cstatus.textContent = e.message; cstatus.classList.add("error"); }
        } }, "Create project"),
      ),
      el("div", { class: "full" }, cstatus),
    ),
  ));

  // Project cards
  const grid = el("div", { class: "cards" }, el("div", { class: "muted small" }, "Loading…"));
  home.append(el("div", { class: "panel" }, el("h2", {}, "Projects"), grid));
  try {
    const projects = await api("GET", "/api/projects");
    grid.innerHTML = "";
    if (!projects.length) { grid.append(el("div", { class: "muted small" }, "No projects yet — create one above.")); return; }
    for (const p of projects) {
      grid.append(el("div", { class: "pcard", onclick: () => location.hash = "#/projects/" + p.id },
        el("button", { class: "del", title: "Delete", onclick: async (e) => {
          e.stopPropagation();
          if (!confirm("Delete “" + p.title + "” and all its pages?")) return;
          await api("DELETE", "/api/projects/" + p.id); renderHome();
        } }, "×"),
        el("div", { class: "pt" }, p.title),
        el("div", { class: "pm" }, (p.page_count || 0) + " page" + (p.page_count === 1 ? "" : "s") + " · " + p.voice),
      ));
    }
  } catch (e) { grid.innerHTML = '<div class="status error">' + e.message + "</div>"; }
}

// ---------- project / reader ----------
let state = { project: null, selectedPageId: null };
let pollTimer = null;

async function renderProject(id) {
  app.innerHTML = "";
  let p;
  try { p = await api("GET", "/api/projects/" + id); }
  catch (e) { app.append(topbar(), el("div", { class: "home" }, el("div", { class: "status error" }, e.message))); return; }
  p.pages = p.pages || [];
  state.project = p;
  // Restore the last page the user was on (survives browser refresh).
  if (!state.selectedPageId || !p.pages.some(pg => pg.id === state.selectedPageId)) {
    const saved = parseInt(localStorage.getItem(pageKey(p.id)) || "", 10);
    state.selectedPageId = (saved && p.pages.some(pg => pg.id === saved)) ? saved
      : (p.pages.length ? p.pages[0].id : null);
  }

  // Single canonical voice/speed controls — placed in the top bar (desktop) or
  // in the bottom player (mobile). curVoice()/curSpeed() read these by id.
  const vsel = voiceSelect(p.voice);
  vsel.id = "voice";
  vsel.addEventListener("change", clearSpeechCache);
  const speed = el("input", { id: "pspeed", type: "range", min: "0.5", max: "2", step: "0.05", value: String(p.speed),
    oninput: updateSpeedLabel, onchange: clearSpeechCache });
  const back = el("a", { href: "#/", title: "All projects",
    html: '<svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5M12 19l-7-7 7-7"/></svg>' });

  const mobile = isMobileView();
  if (mobile) {
    const burger = el("button", { class: "iconbtn", title: "Menu", onclick: openMobileMenu,
      html: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="4" y1="7" x2="20" y2="7"/><line x1="4" y1="12" x2="20" y2="12"/><line x1="4" y1="17" x2="20" y2="17"/></svg>' });
    const tb = el("div", { class: "topbar mobile" },
      el("div", { class: "logo", html: logoSvg, onclick: () => { location.hash = "#/"; } }),
      back,
      el("div", { class: "titlewrap" },
        el("div", { class: "title", title: p.title }, p.title),
        el("div", { class: "msub", id: "mobSub" }, "")),
      el("div", { class: "spacer" }),
      burger,
      el("div", { class: "health", id: "health", html: '<span class="ds"></span><span id="healthText"></span>' }),
    );
    app.append(tb);
    app.append(el("div", { class: "reader", id: "reader" },
      el("div", { class: "sidebar", id: "sidebar" }),
      el("div", { class: "reading", id: "reading" })));
    app.append(buildMobilePlayer(vsel, speed));
  } else {
    app.append(topbar(
      el("span", { class: "sep" }, "/"),
      back,
      el("span", { class: "title", title: p.title }, p.title),
      voiceControl(vsel),
      speedControl(speed),
    ));
    app.append(el("div", { class: "reader", id: "reader" },
      el("div", { class: "sidebar", id: "sidebar" }),
      el("div", { class: "reading", id: "reading" })));
  }

  renderSidebar();
  renderReading();
  updateSpeedLabel();
  maybePoll();
  watchBreakpoint();
}

// voiceControl / speedControl: top-bar controls styled to match the 1a design.
const CHEVRON = '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="#a59a86" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>';
function voiceControl(vsel) {
  vsel.classList.add("voice-select");
  return el("div", { class: "ctl" },
    el("label", {}, "Voice"),
    el("div", { class: "voice-pill" },
      el("span", { class: "vdot" }), vsel, el("span", { class: "chevwrap", html: CHEVRON })));
}
function speedControl(speed) {
  return el("div", { class: "ctl" },
    el("div", { class: "srow" }, el("label", {}, "Speed"), el("span", { class: "sval", id: "speedVal" }, "1.0×")),
    speed);
}

function isMobileView() { return window.matchMedia("(max-width: 760px)").matches; }

// Re-render the project when crossing the mobile/desktop breakpoint.
let lastIsMobile = null, breakpointHooked = false;
function watchBreakpoint() {
  lastIsMobile = isMobileView();
  if (breakpointHooked) return;
  breakpointHooked = true;
  window.addEventListener("resize", () => {
    if (!state.project || !location.hash.startsWith("#/projects/")) return;
    if (isMobileView() !== lastIsMobile) { stopPlayback(); renderProject(state.project.id); }
  });
}

// ---------- mobile player (2a) ----------
function buildMobilePlayer(vsel, speed) {
  const voicePill = el("div", { class: "vpill" }, el("span", { class: "vdot" }), vsel);

  const speedPop = el("div", { class: "spop hidden" }, el("label", {}, "Speed"), speed);
  const spill = el("button", { class: "spill", id: "spill",
    onclick: (e) => { e.stopPropagation(); speedPop.classList.toggle("hidden"); } }, "1.0×");
  const speedWrap = el("div", { class: "swrap" }, spill, speedPop);

  const prev = el("button", { class: "mt", title: "Previous", html: ICON.prev, onclick: () => step(-1) });
  const main = el("button", { class: "mt main", id: "mMain", title: "Play", html: ICON.play, onclick: mobileMainClick });
  const next = el("button", { class: "mt", title: "Next", html: ICON.next, onclick: () => step(1) });

  const seek = el("div", { class: "mseek", id: "mSeek", onclick: seekClick }, el("div", { class: "mseek-fill", id: "mSeekFill" }));
  const cap = el("div", { class: "mcap", id: "mcap" }, "Tap a paragraph to begin");

  return el("div", { class: "mplayer", id: "mplayer" },
    seek,
    el("div", { class: "mrow" }, voicePill, el("div", { class: "mtransport" }, prev, main, next), speedWrap),
    cap,
  );
}

function mobileMainClick() {
  if (play.unitIdx < 0) {
    const pg = selectedPage();
    if (pg && (pg.paragraphs || []).length) startPlay(pg.paragraphs[0].id, "page");
  } else pauseToggle();
}

function seekClick(e) {
  const bar = document.getElementById("mSeek");
  if (!bar || !player.duration) return;
  const r = bar.getBoundingClientRect();
  player.currentTime = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)) * player.duration;
}

function updateSpeedLabel() {
  const sp = document.getElementById("pspeed");
  if (!sp) return;
  const txt = parseFloat(sp.value).toFixed(1) + "×";
  const spill = document.getElementById("spill"); if (spill) spill.textContent = txt;
  const sval = document.getElementById("speedVal"); if (sval) sval.textContent = txt;
  // green fill up to the current value (matches the 1a slider)
  const pct = ((parseFloat(sp.value) - 0.5) / (2.0 - 0.5)) * 100;
  sp.style.background = "linear-gradient(to right, var(--accent) 0 " + pct + "%, var(--line) " + pct + "% 100%)";
}

function updateMobileCaption() {
  const cap = document.getElementById("mcap");
  if (!cap) return;
  if (play.unitIdx < 0 || !play.paraId) { cap.textContent = "Tap a paragraph to begin"; return; }
  const list = flat();
  const e = list[flatIndex(list, play.paraId)];
  if (!e) return;
  const pg = state.project.pages[e.pageIdx];
  const n = (pg.paragraphs || []).findIndex(x => x.id === play.paraId) + 1;
  cap.textContent = "Reading paragraph " + n + " of " + (pg.paragraphs || []).length + " · Page " + (e.pageIdx + 1);
}

function openMobileMenu() {
  const pg = selectedPage();
  const bg = el("div", { class: "sheet-bg", onclick: (e) => { if (e.target === bg) bg.remove(); } });
  const item = (label, fn) => el("button", { class: "sheet-item", onclick: () => { bg.remove(); fn(); } }, label);
  const sheet = el("div", { class: "sheet" },
    item("＋  Add pages", openUpload),
    state.project.pages.length > 1 ? item("↕  Auto-order pages", autoOrder) : null,
    (pg && pg.has_image) ? item("🔍  View original image", () => openLightbox(pg, state.project.pages.indexOf(pg))) : null,
    item("←  All projects", () => { location.hash = "#/"; }),
  );
  bg.append(sheet);
  document.body.append(bg);
}

player.addEventListener("timeupdate", () => {
  const fill = document.getElementById("mSeekFill");
  if (fill) fill.style.width = (player.duration ? (player.currentTime / player.duration * 100) : 0) + "%";
});

function pageLabelOrder(p) { return p.pages.map((pg, i) => ({ pg, i })); }
function pageBusy() { return state.project.pages.some(pg => pg.ocr_status === "pending" || pg.ocr_status === "processing"); }

function renderSidebar() {
  const sb = document.getElementById("sidebar");
  if (!sb) return;
  sb.innerHTML = "";
  const head = el("div", { class: "shead" }, el("h3", {}, "Pages"));
  if (state.project.pages.length > 1) {
    head.append(el("button", { class: "btn ghost tiny", id: "autoOrderBtn", title: "Reorder automatically by page number or content flow", onclick: autoOrder }, "↕ Auto-order"));
  }
  sb.append(head);
  sb.append(el("button", { class: "btn secondary addbtn", onclick: openUpload }, "＋ Add pages"));

  const pages = state.project.pages;
  if (!pages.length) { sb.append(el("div", { class: "muted small", style: "padding:8px 4px" }, "No pages yet.")); return; }

  pages.forEach((pg, i) => {
    const l1 = el("div", { class: "l1" }, "Page " + (i + 1));
    if (pg.page_number != null) l1.append(el("span", { class: "badge num" }, "p." + pg.page_number));
    if (pg.ocr_status === "processing" || pg.ocr_status === "pending") l1.append(el("span", { class: "spinner" }));
    if (pg.ocr_status === "error") l1.append(el("span", { class: "badge error" }, "error"));

    const sub = pg.ocr_status === "done"
      ? ((pg.paragraphs || []).length + " paragraphs")
      : (pg.ocr_status === "error" ? "OCR failed" : "Reading…");

    const thumb = pg.has_image
      ? el("img", { class: "thumb zoom", src: "/api/pages/" + pg.id + "/image", alt: "", title: "Click to view full page",
          onclick: (e) => { e.stopPropagation(); openLightbox(pg, i); } })
      : el("div", { class: "thumb zoom" });

    const handle = el("div", { class: "drag-handle", title: "Drag to reorder",
      html: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="9" cy="6" r="1.6"/><circle cx="9" cy="12" r="1.6"/><circle cx="9" cy="18" r="1.6"/><circle cx="15" cy="6" r="1.6"/><circle cx="15" cy="12" r="1.6"/><circle cx="15" cy="18" r="1.6"/></svg>' });

    const row = el("div", { class: "pagebtn" + (pg.id === state.selectedPageId ? " active" : ""),
      draggable: "true", onclick: () => selectPage(pg.id) },
      thumb,
      el("div", { class: "pinfo" }, l1, el("div", { class: "l2" }, sub)),
      el("div", { class: "pstrip-label" }, "P" + (i + 1)),
      handle,
    );
    // drag-and-drop reordering
    row.addEventListener("dragstart", (e) => { dragFrom = i; e.dataTransfer.effectAllowed = "move"; });
    row.addEventListener("dragover", (e) => { e.preventDefault(); row.classList.add("dragover"); });
    row.addEventListener("dragleave", () => row.classList.remove("dragover"));
    row.addEventListener("drop", (e) => { e.preventDefault(); row.classList.remove("dragover"); dropOnPage(i); });
    row.addEventListener("dragend", () => { dragFrom = null; });
    sb.append(row);
  });
}

let dragFrom = null;
async function dropOnPage(to) {
  const from = dragFrom; dragFrom = null;
  if (from == null || from === to) return;
  const pages = state.project.pages;
  const [moved] = pages.splice(from, 1);
  pages.splice(to, 0, moved);            // optimistic local reorder
  renderSidebar(); renderReading();
  const order = pages.map(p => p.id);
  try { state.project = await api("POST", "/api/projects/" + state.project.id + "/reorder", { order }); renderSidebar(); renderReading(); }
  catch (e) { console.error(e); }
}

function pageKey(projectId) { return "invtts:proj:" + projectId + ":page"; }

function selectPage(pageId) {
  state.selectedPageId = pageId;
  if (state.project) localStorage.setItem(pageKey(state.project.id), String(pageId));
  renderSidebar();
  renderReading();
}

async function movePage(index, dir) {
  const pages = state.project.pages;
  const j = index + dir;
  if (j < 0 || j >= pages.length) return;
  const order = pages.map(p => p.id);
  [order[index], order[j]] = [order[j], order[index]];
  // optimistic
  [pages[index], pages[j]] = [pages[j], pages[index]];
  renderSidebar();
  try { state.project = await api("POST", "/api/projects/" + state.project.id + "/reorder", { order }); renderSidebar(); renderReading(); }
  catch (e) { console.error(e); }
}

async function autoOrder() {
  const btn = document.getElementById("autoOrderBtn");
  if (btn) { btn.disabled = true; btn.textContent = "Ordering…"; }
  try {
    state.project = await api("POST", "/api/projects/" + state.project.id + "/auto-order");
    renderSidebar(); renderReading();
  } catch (e) { console.error(e); if (btn) { btn.disabled = false; btn.textContent = "↕ Auto-order"; } }
}

// openLightbox shows the full-size page image with prev/next navigation.
function openLightbox(pg, idx) {
  const pages = state.project.pages;
  let i = idx;
  const img = el("img", { class: "lb-img", src: "/api/pages/" + pages[i].id + "/image", alt: "" });
  const caption = el("div", { class: "lb-cap" });
  const bg = el("div", { class: "lightbox", onclick: (e) => { if (e.target === bg) bg.remove(); } });

  function show(n) {
    i = (n + pages.length) % pages.length;
    // skip text-only pages
    let guard = 0;
    while (!pages[i].has_image && guard++ < pages.length) i = (i + (n >= idx ? 1 : -1) + pages.length) % pages.length;
    img.src = "/api/pages/" + pages[i].id + "/image";
    caption.textContent = "Page " + (i + 1) + " of " + pages.length + (pages[i].page_number != null ? " · printed page " + pages[i].page_number : "");
  }
  show(i);

  const prev = el("button", { class: "lb-nav prev", title: "Previous", onclick: (e) => { e.stopPropagation(); show(i - 1); } }, "‹");
  const next = el("button", { class: "lb-nav next", title: "Next", onclick: (e) => { e.stopPropagation(); show(i + 1); } }, "›");
  const close = el("button", { class: "lb-close", title: "Close (Esc)", onclick: () => bg.remove() }, "×");
  bg.append(prev, el("div", { class: "lb-stage" }, img, caption), next, close);
  document.body.append(bg);

  const onKey = (e) => {
    if (e.key === "Escape") { bg.remove(); document.removeEventListener("keydown", onKey); }
    else if (e.key === "ArrowRight") show(i + 1);
    else if (e.key === "ArrowLeft") show(i - 1);
  };
  document.addEventListener("keydown", onKey);
}

function selectedPage() { return state.project.pages.find(p => p.id === state.selectedPageId); }

function renderReading() {
  const box = document.getElementById("reading");
  if (!box) return;
  box.innerHTML = "";
  const pages = state.project.pages;
  if (!pages.length) {
    box.append(el("div", { class: "empty" },
      el("div", { class: "big" }, "This project is empty"),
      el("div", {}, "Add photos of book pages (they’ll be transcribed and ordered automatically) or paste text."),
      el("div", { style: "margin-top:18px" }, el("button", { class: "btn", onclick: openUpload }, "＋ Add pages")),
    ));
    return;
  }
  const pg = selectedPage();
  const idx = pages.indexOf(pg);

  const mobSub = document.getElementById("mobSub");
  if (mobSub) mobSub.textContent = "Page " + (idx + 1) + " of " + pages.length + (pg.has_image ? " · from photo" : "");

  const sub = [];
  if (pg.page_number != null) sub.push("printed page " + pg.page_number);
  if (pg.has_image) sub.push("from photo");
  const rhead = el("div", { class: "rhead" },
    el("div", {},
      el("div", { class: "pgno" }, "Page " + (idx + 1) + " ", el("span", { class: "faint", style: "font-weight:500;font-size:14px" }, "of " + pages.length)),
      sub.length ? el("div", { class: "pgsub" }, sub.join(" · ")) : null,
    ),
  );
  const headActions = el("div", { style: "display:flex; gap:8px; align-items:center" });
  if (pg.has_image) headActions.append(el("button", { class: "btn ghost tiny", onclick: () => openLightbox(pg, idx) }, "🔍 View original"));
  if (pg.ocr_status === "done" && (pg.paragraphs || []).length) {
    headActions.append(el("button", { class: "btn tiny", onclick: () => startPlay(pg.paragraphs[0].id, "page") }, "▶ Read this page"));
  }
  rhead.append(headActions);
  box.append(rhead);

  const col = el("div", { class: "col" });
  box.append(col);

  if (pg.ocr_status !== "done") {
    if (pg.ocr_status === "error") col.append(el("div", { class: "status error" }, pg.ocr_error || "OCR failed for this page."));
    else col.append(el("div", { class: "muted", style: "display:flex;gap:10px;align-items:center" }, el("span", { class: "spinner" }), "Transcribing this page with Gemini…"));
    return;
  }
  const paras = pg.paragraphs || [];
  // Detect cross-page paragraph flow so we can hint it to the reader. Uses the
  // OCR continuation flag when available, else the punctuation heuristic.
  const prevPage = idx > 0 ? pages[idx - 1] : null;
  const nextPage = idx < pages.length - 1 ? pages[idx + 1] : null;
  const pageContinues = (prev, cur) =>
    typeof cur.cont_start === "boolean" ? cur.cont_start
      : (prev && (prev.paragraphs || []).length && endsOpen(prev.paragraphs[prev.paragraphs.length - 1].text));
  const continuedFromPrev = prevPage && (prevPage.paragraphs || []).length && pageContinues(prevPage, pg);
  const continuesToNext = nextPage && paras.length && pageContinues(pg, nextPage);

  if (continuedFromPrev) col.append(el("div", { class: "flow-note top" }, "↪ continues from the previous page"));

  paras.forEach((para, i) => {
    const text = el("div", { class: "ptext" }, para.text);
    const playBtn = el("button", { class: "play", title: "Play from here", html: ICON.play, onclick: () => startPlay(para.id) });
    const node = el("div", { class: "para", "data-pid": para.id },
      el("div", { class: "gutter" }, playBtn,
        el("div", { class: "eq", html: "<span></span><span></span><span></span>" }),
        el("div", { class: "pnum" }, String(i + 1))),
      text,
      el("div", { class: "para-tools" },
        el("button", { class: "ptool", title: "Regenerate audio (re-do this paragraph)", onclick: () => startPlay(para.id, "single", true) }, "↻"),
        el("button", { class: "ptool", title: "Edit text", onclick: () => editParagraph(node, para) }, "✎"),
      ),
    );
    if (continuesToNext && i === paras.length - 1) node.classList.add("flows-next");
    col.append(node);
  });

  if (continuesToNext) col.append(el("div", { class: "flow-note bottom" }, "this paragraph flows into the next page ↦ (played as one)"));
}

// editParagraph swaps a paragraph into an inline editor; saving updates the text
// and clears cached audio so the next play re-synthesizes it.
function editParagraph(node, para) {
  if (play.paraId === para.id) stopPlayback();
  const ta = el("textarea", { class: "edit-area" });
  ta.value = para.text;
  const status = el("span", { class: "status small" });
  const editor = el("div", { class: "para-edit" }, ta,
    el("div", { class: "edit-actions" },
      el("button", { class: "btn tiny", onclick: save }, "Save & re-do"),
      el("button", { class: "btn ghost tiny", onclick: () => renderReading() }, "Cancel"),
      status,
    ),
  );
  node.querySelector(".ptext").replaceWith(editor);
  node.querySelector(".para-tools")?.remove();
  ta.focus();

  async function save() {
    const text = ta.value.trim();
    if (!text) { status.textContent = "Text can’t be empty."; status.classList.add("error"); return; }
    status.textContent = "Saving…"; status.classList.remove("error");
    try {
      const updated = await api("PUT", "/api/paragraphs/" + para.id, { text });
      // update local state
      for (const pg of state.project.pages) {
        const idx = (pg.paragraphs || []).findIndex(p => p.id === para.id);
        if (idx >= 0) { pg.paragraphs[idx] = updated; break; }
      }
      renderReading();
      startPlay(para.id, "single", true); // regenerate audio with the new text
    } catch (e) { status.textContent = e.message; status.classList.add("error"); }
  }
}

// ---------- upload modal ----------
function openUpload() {
  const fileInput = el("input", { type: "file", accept: "image/*,.pdf,application/pdf", multiple: "", class: "hidden" });
  const dz = el("div", { class: "dropzone", onclick: () => fileInput.click() },
    el("div", {}, "Drop page photos or PDFs here, or click to choose"),
    el("div", { class: "hint" }, "PDFs are split into pages; photo order is detected from page numbers."));
  dz.addEventListener("dragover", e => { e.preventDefault(); dz.classList.add("drag"); });
  dz.addEventListener("dragleave", () => dz.classList.remove("drag"));
  dz.addEventListener("drop", e => { e.preventDefault(); dz.classList.remove("drag"); doUpload(e.dataTransfer.files, null); });
  fileInput.addEventListener("change", () => doUpload(fileInput.files, null));
  const ta = el("textarea", { placeholder: "…or paste text to add as one page" });
  const st = el("div", { class: "status" });

  const bg = el("div", { class: "modal-bg", onclick: (e) => { if (e.target === bg) bg.remove(); } });
  const modal = el("div", { class: "modal" },
    el("h3", {}, "Add pages"),
    dz, fileInput,
    el("div", { class: "divider" }, "— or —"),
    ta,
    st,
    el("div", { class: "modal-actions" },
      el("button", { class: "btn ghost", onclick: () => bg.remove() }, "Close"),
      el("button", { class: "btn", onclick: () => doUpload(null, ta.value) }, "Add text page"),
    ),
  );
  bg.append(modal);
  document.body.append(bg);

  async function doUpload(files, text) {
    const fd = new FormData();
    let any = false;
    if (files && files.length) {
      for (const f of files) {
        const isPDF = f.type === "application/pdf" || f.name.toLowerCase().endsWith(".pdf");
        fd.append(isPDF ? "pdf" : "images", f);
      }
      any = true;
    }
    if (text && text.trim()) { fd.append("text", text.trim()); any = true; }
    if (!any) { st.textContent = "Choose images or PDFs, or enter text first."; st.classList.add("error"); return; }
    st.textContent = "Uploading…"; st.classList.remove("error");
    try {
      const p = await api("POST", "/api/projects/" + state.project.id + "/pages", fd, true);
      state.project = p;
      if (!state.selectedPageId && p.pages.length) state.selectedPageId = p.pages[0].id;
      bg.remove();
      renderSidebar(); renderReading(); maybePoll();
    } catch (e) { st.textContent = e.message; st.classList.add("error"); }
  }
}

function maybePoll() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  if (!state.project || !pageBusy()) return;
  pollTimer = setInterval(async () => {
    try {
      const fresh = await api("GET", "/api/projects/" + state.project.id);
      state.project = fresh;
      if (!fresh.pages.some(p => p.id === state.selectedPageId) && fresh.pages.length) state.selectedPageId = fresh.pages[0].id;
      renderSidebar(); renderReading();
      if (!pageBusy()) { clearInterval(pollTimer); pollTimer = null; }
    } catch (_) {}
  }, 1800);
}

// ---------- playback engine (unit-based) ----------
// A "unit" is one playable chunk: usually a single paragraph, but when a
// paragraph is split across a page break (the bottom of one page flows into the
// top of the next) the pieces are joined into one unit and synthesized as a
// single clip — so it reads continuously with no pause.
let play = { paraId: null, unitIdx: -1, mode: "page", words: [], raf: 0 };
let activeHighlightSpan = null;
// Persistent reading mode chosen by the Page / All-pages toggle. Clicking any
// paragraph continues in this mode (default: read to the end of the page).
let readMode = "page";

let speechCache = new Map(); // key -> { audioUrl, timestamps }
const PREFETCH_AHEAD = 3;
function settingsKey() { return curVoice() + "|" + curSpeed(); }
function clearSpeechCache() { speechCache.clear(); }

// Flattened ordered list of paragraphs across all pages.
function flat() {
  const out = [];
  state.project.pages.forEach((pg, pi) => (pg.paragraphs || []).forEach(pa =>
    out.push({ pageId: pg.id, pageIdx: pi, para: pa, contStart: pg.cont_start })));
  return out;
}
function flatIndex(list, paraId) { return list.findIndex(x => x.para.id === paraId); }

// A paragraph "ends open" (mid-sentence/clause) if its last meaningful char is
// not sentence-final punctuation — a heuristic signal it continues onto the next page.
function endsOpen(text) {
  const t = (text || "").replace(/[\s"'”’)\]]+$/, "");
  const last = t.slice(-1);
  return !!last && !".!?".includes(last);
}
function isLastOfPage(list, i) { return i + 1 >= list.length || list[i + 1].pageIdx !== list[i].pageIdx; }
function isFirstOfPage(list, i) { return i === 0 || list[i - 1].pageIdx !== list[i].pageIdx; }

// Decide whether paragraph b (first of its page) continues paragraph a (last of
// the previous page). Prefer the OCR-detected flag on b's page; fall back to the
// punctuation heuristic when OCR didn't record it (text pages / older scans).
function joinsAcross(a, b) {
  if (typeof b.contStart === "boolean") return b.contStart;
  return endsOpen(a.para.text);
}

// Group the flat paragraph list into play units, joining cross-page splits.
function buildUnits(list) {
  const units = [];
  let i = 0;
  while (i < list.length) {
    const idxs = [i];
    let j = i;
    while (isLastOfPage(list, j) && j + 1 < list.length && isFirstOfPage(list, j + 1) && joinsAcross(list[j], list[j + 1])) {
      idxs.push(j + 1); j++;
    }
    units.push({ indices: idxs, entries: idxs.map(k => list[k]) });
    i = j + 1;
  }
  return units;
}
function unitIndexForPara(units, paraId) {
  return units.findIndex(u => u.entries.some(e => e.para.id === paraId));
}

async function fetchUnitSpeech(unit, force) {
  const key = settingsKey();
  if (unit.entries.length === 1) {
    const id = unit.entries[0].para.id;
    const ck = "p" + id + "|" + key;
    if (!force) { const hit = speechCache.get(ck); if (hit) return hit; }
    const d = await api("POST", "/api/paragraphs/" + id + "/speech",
      { voice: curVoice(), speed: curSpeed(), format: "mp3", force: !!force });
    const v = { audioUrl: d.audioUrl, timestamps: d.timestamps || [] };
    speechCache.set(ck, v); return v;
  }
  // Joined unit: synthesize the combined text as one clip.
  const text = unit.entries.map(e => e.para.text).join(" ");
  const ck = "t" + text + "|" + key;
  if (!force) { const hit = speechCache.get(ck); if (hit) return hit; }
  const d = await api("POST", "/api/speak",
    { text, voice: curVoice(), speed: curSpeed(), format: "mp3", force: !!force });
  const v = { audioUrl: d.audioUrl, timestamps: d.timestamps || [] };
  speechCache.set(ck, v); return v;
}

let prefetchRun = 0;
function prefetchAhead(units, uIdx) {
  const run = ++prefetchRun;
  (async () => {
    for (let k = 1; k <= PREFETCH_AHEAD && uIdx + k < units.length; k++) {
      if (run !== prefetchRun) return;
      try { await fetchUnitSpeech(units[uIdx + k], false); } catch (_) { /* best effort */ }
    }
  })();
}

function stopPlayback() {
  if (play.raf) cancelAnimationFrame(play.raf);
  clearActiveHighlight();
  try { player.pause(); } catch (_) {}
  const node = document.querySelector(".para.active");
  if (node) resetParaNode(node);
  play = { paraId: null, unitIdx: -1, mode: "single", words: [], raf: 0 };
  const pb = document.getElementById("playbar");
  if (pb) pb.remove();
  const r = document.getElementById("reader");
  if (r) r.classList.remove("playing");
  const m = document.getElementById("mMain"); if (m) m.innerHTML = ICON.play;
  const fill = document.getElementById("mSeekFill"); if (fill) fill.style.width = "0%";
  updateMobileCaption();
}

function resetParaNode(node) {
  node.classList.remove("active");
  node.classList.remove("playing");
  const btn = node.querySelector(".play");
  if (btn) btn.innerHTML = ICON.play;
  const t = node.querySelector(".ptext");
  if (t) {
    if (t.dataset.orig !== undefined) { t.textContent = t.dataset.orig; delete t.dataset.orig; delete t.dataset.plain; }
    else if (t.dataset.plain) { t.textContent = t.dataset.plain; delete t.dataset.plain; }
  }
}

async function startPlay(paraId, mode, force) {
  if (mode === "page" || mode === "all") readMode = mode; // explicit choice updates the toggle
  const units = buildUnits(flat());
  const uIdx = unitIndexForPara(units, paraId);
  if (uIdx < 0) return;
  if (!force && play.unitIdx === uIdx && !player.paused) { pauseToggle(); return; }
  // "single" is only used for one-off regenerate; a plain paragraph click uses
  // the persistent reading mode so it continues to the end of the page.
  play.mode = (mode === "single") ? "single" : (mode || readMode);
  await playUnit(units, uIdx, force);
}

async function playUnit(units, uIdx, force) {
  if (play.raf) cancelAnimationFrame(play.raf);
  play.raf = 0;
  clearActiveHighlight();
  const prev = document.querySelector(".para.active");
  if (prev) resetParaNode(prev);

  const unit = units[uIdx];
  if (!unit) { stopPlayback(); return; }
  const isChain = unit.entries.length > 1;
  const headEntry = unit.entries[0];

  // Prefer to display on a page already in view if it's part of this unit,
  // otherwise jump to the unit's first page.
  let displayEntry = unit.entries.find(e => e.pageId === state.selectedPageId) || headEntry;
  if (displayEntry.pageId !== state.selectedPageId) selectPage(displayEntry.pageId);

  play.unitIdx = uIdx;
  play.paraId = headEntry.para.id;
  ensurePlaybar();
  updatePlaybar();

  const node = document.querySelector('.para[data-pid="' + displayEntry.para.id + '"]');
  if (!node) return;
  node.classList.add("active");
  node.scrollIntoView({ behavior: "smooth", block: "center" });
  const btn = node.querySelector(".play");
  if (btn) btn.innerHTML = '<span class="spinner"></span>';

  // For a joined unit, temporarily show the full combined text in this element.
  if (isChain) {
    const c = node.querySelector(".ptext");
    if (c.dataset.orig === undefined) c.dataset.orig = c.textContent;
    const combined = unit.entries.map(e => e.para.text).join(" ");
    c.dataset.plain = combined;
    c.textContent = combined;
  }

  let data;
  try { data = await fetchUnitSpeech(unit, force); }
  catch (e) { if (btn) btn.innerHTML = ICON.play; node.classList.remove("active"); console.error(e); return; }
  if (play.unitIdx !== uIdx) return; // superseded

  player.src = data.audioUrl;
  try { await waitForAudioMetadata(); }
  catch (e) { if (btn) btn.innerHTML = ICON.play; node.classList.remove("active"); console.error(e); return; }
  if (play.unitIdx !== uIdx) return; // superseded while audio loaded

  buildWords(node, data.timestamps || [], player.duration);
  if (btn) btn.innerHTML = ICON.pause;
  node.classList.add("playing");
  try { await player.play(); }
  catch (e) {
    if (btn) btn.innerHTML = ICON.play;
    node.classList.remove("playing");
    console.error(e);
    return;
  }
  if (play.unitIdx !== uIdx) return;
  setPlaybarIcon(true);
  loopHighlight();
  prefetchAhead(units, uIdx);
}

function waitForAudioMetadata() {
  if (player.readyState >= 1 && Number.isFinite(player.duration)) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const loaded = () => { cleanup(); resolve(); };
    const failed = () => { cleanup(); reject(new Error("Could not load speech audio")); };
    const cleanup = () => {
      player.removeEventListener("loadedmetadata", loaded);
      player.removeEventListener("error", failed);
    };
    player.addEventListener("loadedmetadata", loaded);
    player.addEventListener("error", failed);
  });
}

function buildWords(node, timestamps, duration) {
  const c = node.querySelector(".ptext");
  if (!c.dataset.plain) c.dataset.plain = c.textContent;
  const original = c.dataset.plain;
  let timings = highlighter.repairTimings(timestamps);
  if (highlighter.needsEstimatedTimings(timings, duration)) {
    timings = highlighter.estimateTimings(original, duration);
  }
  const aligned = highlighter.alignText(original, timings);
  const spans = new Array(timings.length).fill(null);

  c.innerHTML = "";
  let cursor = 0;
  aligned.ranges.forEach((range, index) => {
    if (!range || range.start < cursor || range.end <= range.start) return;
    if (range.start > cursor) c.append(document.createTextNode(aligned.displayText.slice(cursor, range.start)));
    const span = el("span", { class: "word" }, aligned.displayText.slice(range.start, range.end));
    c.append(span);
    spans[index] = span;
    cursor = range.end;
  });
  if (cursor < aligned.displayText.length) {
    c.append(document.createTextNode(aligned.displayText.slice(cursor)));
  }
  play.words = timings.map((timing, index) => ({ ...timing, span: spans[index] }));
}

function mappedHighlightSpan(index) {
  if (index < 0 || index >= play.words.length) return null;
  if (play.words[index].span) return play.words[index].span;
  for (let distance = 1; distance < play.words.length; distance++) {
    if (index + distance < play.words.length && play.words[index + distance].span) {
      return play.words[index + distance].span;
    }
    if (index - distance >= 0 && play.words[index - distance].span) {
      return play.words[index - distance].span;
    }
  }
  return null;
}

function setActiveHighlight(index) {
  const next = mappedHighlightSpan(index);
  if (next === activeHighlightSpan) return;
  if (activeHighlightSpan) activeHighlightSpan.classList.remove("spoken");
  activeHighlightSpan = next;
  if (activeHighlightSpan) activeHighlightSpan.classList.add("spoken");
}

function clearActiveHighlight() {
  if (activeHighlightSpan) activeHighlightSpan.classList.remove("spoken");
  activeHighlightSpan = null;
}

function syncHighlight() {
  const index = highlighter.activeIndex(play.words, player.currentTime, player.duration);
  setActiveHighlight(index);
}

function loopHighlight() {
  if (play.raf) cancelAnimationFrame(play.raf);
  const tick = () => {
    if (player.paused || player.ended) {
      play.raf = 0;
      if (player.ended) clearActiveHighlight();
      return;
    }
    syncHighlight();
    play.raf = requestAnimationFrame(tick);
  };
  play.raf = requestAnimationFrame(tick);
}

function pauseToggle() {
  if (play.unitIdx < 0) return;
  const active = document.querySelector(".para.active");
  if (player.paused) {
    player.play().then(() => loopHighlight()).catch(console.error);
    setPlaybarIcon(true);
    if (active) { active.classList.add("playing"); active.querySelector(".play").innerHTML = ICON.pause; }
  }
  else { player.pause(); if (play.raf) cancelAnimationFrame(play.raf); play.raf = 0; setPlaybarIcon(false);
    if (active) { active.classList.remove("playing"); active.querySelector(".play").innerHTML = ICON.play; } }
}

function step(dir) {
  const units = buildUnits(flat());
  let i = play.unitIdx;
  if (i < 0) return;
  i += dir;
  if (i < 0 || i >= units.length) { stopPlayback(); return; }
  playUnit(units, i, false);
}

player.addEventListener("ended", () => {
  clearActiveHighlight();
  const n = document.querySelector(".para.active .play"); if (n) n.innerHTML = ICON.play;
  if (play.mode === "single") { stopPlayback(); return; }
  const units = buildUnits(flat());
  const cur = units[play.unitIdx];
  const next = units[play.unitIdx + 1];
  if (!cur || !next) { stopPlayback(); return; }
  // "Page" mode stops when the next unit starts on a different page.
  if (play.mode === "page" && next.entries[0].pageId !== cur.entries[0].pageId) { stopPlayback(); return; }
  playUnit(units, play.unitIdx + 1, false);
});
player.addEventListener("seeked", syncHighlight);

function curVoice() { const s = document.getElementById("voice"); return s ? s.value : undefined; }
function curSpeed() { const s = document.getElementById("pspeed"); return s ? parseFloat(s.value) : undefined; }

// ---------- playbar ----------
function ensurePlaybar() {
  if (isMobileView()) { updateMobileCaption(); return; } // mobile uses the persistent player
  if (document.getElementById("playbar")) { updatePlaybar(); return; }
  const reader = document.getElementById("reader");
  if (reader) reader.classList.add("playing");
  const bar = el("div", { class: "playbar", id: "playbar" },
    el("button", { class: "pbtn", title: "Previous", html: ICON.prev, onclick: () => step(-1) }),
    el("button", { class: "pbtn main", id: "pbMain", title: "Play/Pause", html: ICON.pause, onclick: pauseToggle }),
    el("button", { class: "pbtn", title: "Next", html: ICON.next, onclick: () => step(1) }),
    el("div", { class: "now", id: "pbNow" }),
    el("div", { class: "seg" },
      el("button", { id: "segPage", onclick: () => setMode("page") }, "Page"),
      el("button", { id: "segAll", onclick: () => setMode("all") }, "All pages"),
    ),
    el("button", { class: "close", title: "Stop", onclick: stopPlayback }, "×"),
  );
  app.append(bar);
  updatePlaybar();
}
function setMode(m) { readMode = (m === "single") ? "page" : m; play.mode = readMode; updatePlaybar(); }
function setPlaybarIcon(playing) {
  const b = document.getElementById("pbMain"); if (b) b.innerHTML = playing ? ICON.pause : ICON.play;
  const m = document.getElementById("mMain"); if (m) m.innerHTML = playing ? ICON.pause : ICON.play;
}
function updatePlaybar() {
  const now = document.getElementById("pbNow");
  if (!now) return;
  const list = flat();
  const i = flatIndex(list, play.paraId);
  const e = list[i];
  if (e) {
    const para = e.para;
    const pIdx = (state.project.pages[e.pageIdx].paragraphs || []).indexOf(para) + 1;
    now.innerHTML = "";
    now.append(el("div", { class: "l1" }, (para.text || "").slice(0, 80) + (para.text.length > 80 ? "…" : "")),
      el("div", { class: "l2" }, "Page " + (e.pageIdx + 1) + " · paragraph " + pIdx));
  }
  const eff = play.mode === "single" ? "page" : play.mode;
  const sp = document.getElementById("segPage"), sa = document.getElementById("segAll");
  if (sp && sa) { sp.classList.toggle("on", eff === "page"); sa.classList.toggle("on", eff === "all"); }
}

// ---------- health ----------
async function checkHealth() {
  const h = document.getElementById("health"); if (!h) return;
  const t = document.getElementById("healthText");
  try {
    const res = await fetch("/api/health"); const d = await res.json();
    if (res.ok && d.status === "ok") { h.classList.add("ok"); t.textContent = "Kokoro"; }
    else { h.classList.add("bad"); t.textContent = "Kokoro down"; }
  } catch (_) { h.classList.add("bad"); t.textContent = "offline"; }
}
