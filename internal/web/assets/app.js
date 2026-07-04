"use strict";

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const api = (p, opt) => {
  opt = opt || {};
  const tok = localStorage.getItem("cogo.token");
  if (tok) opt.headers = Object.assign({ Authorization: "Bearer " + tok }, opt.headers);
  return fetch(p, opt).then(r => r.json());
};
const cls = c => "c-" + (c || "ungraded");
function el(tag, className, text) {
  const e = document.createElement(tag);
  if (className) e.className = className;
  if (text != null) e.textContent = text;
  return e;
}
// Cabecera de vista al estilo Escriba: eyebrow + título + bajada.
function viewHead(main, eyebrow, title, sub) {
  const h = el("div", "viewhead");
  h.appendChild(el("div", "vh-eyebrow", eyebrow));
  h.appendChild(el("h2", "vh-title", title));
  if (sub) h.appendChild(el("div", "vh-sub", sub));
  main.appendChild(h);
}

// ---- Markdown mínimo y seguro (sin dependencias) ----
function mdEscape(s) {
  return (s || "").replace(/[&<>"]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
}
function mdInline(s) {
  s = mdEscape(s);
  s = s.replace(/`([^`]+)`/g, (_, c) => "<code>" + c + "</code>");
  s = s.replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
  s = s.replace(/\[\[([^\]]+)\]\]/g, (_, id) => '<a class="wikilink" data-id="' + mdEscape(id.trim()) + '">' + mdEscape(id.trim()) + "</a>");
  s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  s = s.replace(/(^|[^*])\*([^*\n]+)\*/g, "$1<em>$2</em>");
  return s;
}
// mdToHtml: cubre lo que usan las notas — headings, negrita/itálica, código y
// fences, listas, citas, links, [[wikilinks]] y hr. Escapa HTML (anti-inyección).
function mdToHtml(src) {
  const lines = (src || "").replace(/\r\n/g, "\n").split("\n");
  let html = "", i = 0, list = null;
  const closeList = () => { if (list) { html += "</" + list + ">"; list = null; } };
  const special = /^(#{1,6}\s|```|>|\s*[-*]\s|\s*\d+\.\s|-{3,}\s*$|\*{3,}\s*$)/;
  while (i < lines.length) {
    const ln = lines[i];
    if (/^```/.test(ln)) {
      closeList(); i++;
      let code = "";
      while (i < lines.length && !/^```/.test(lines[i])) { code += lines[i] + "\n"; i++; }
      i++;
      html += "<pre><code>" + mdEscape(code.replace(/\n$/, "")) + "</code></pre>";
      continue;
    }
    const h = ln.match(/^(#{1,6})\s+(.*)$/);
    if (h) { closeList(); const l = h[1].length; html += "<h" + l + ">" + mdInline(h[2]) + "</h" + l + ">"; i++; continue; }
    if (/^(-{3,}|\*{3,})\s*$/.test(ln)) { closeList(); html += "<hr>"; i++; continue; }
    if (/^>\s?/.test(ln)) { closeList(); html += "<blockquote>" + mdInline(ln.replace(/^>\s?/, "")) + "</blockquote>"; i++; continue; }
    const ul = ln.match(/^\s*[-*]\s+(.*)$/), ol = ln.match(/^\s*\d+\.\s+(.*)$/);
    if (ul || ol) {
      const t = ul ? "ul" : "ol";
      if (list !== t) { closeList(); html += "<" + t + ">"; list = t; }
      html += "<li>" + mdInline(ul ? ul[1] : ol[1]) + "</li>"; i++; continue;
    }
    if (/^\s*$/.test(ln)) { closeList(); i++; continue; }
    closeList();
    let para = ln; i++;
    while (i < lines.length && !/^\s*$/.test(lines[i]) && !special.test(lines[i])) { para += " " + lines[i]; i++; }
    html += "<p>" + mdInline(para) + "</p>";
  }
  closeList();
  return html;
}

// confirmDialog: un modal de confirmación al estilo Suite Escriba (reemplaza al
// confirm() nativo del navegador). Devuelve una Promise<boolean>.
function confirmDialog({ title, message, note, hint, confirmText = "Aceptar", cancelText = "Cancelar", danger = false } = {}) {
  return new Promise(resolve => {
    const back = el("div", "modal-back confirm-back");
    const card = el("div", "modal-card confirm-card");
    card.appendChild(el("h2", "modal-tit", title));
    const cuerpo = el("div", "modal-cuerpo");
    if (note) cuerpo.appendChild(el("div", "confirm-note", note));
    if (message) cuerpo.appendChild(el("p", "confirm-msg", message));
    if (hint) cuerpo.appendChild(el("div", "confirm-hint", hint));
    card.appendChild(cuerpo);
    const acc = el("div", "modal-acciones");
    const cancel = el("button", "ghost", cancelText);
    const ok = el("button", danger ? "danger-btn" : "", confirmText);
    acc.appendChild(cancel);
    acc.appendChild(ok);
    card.appendChild(acc);
    back.appendChild(card);
    document.body.appendChild(back);
    requestAnimationFrame(() => back.classList.add("show"));

    const close = val => {
      document.removeEventListener("keydown", onKey);
      back.classList.remove("show");
      setTimeout(() => back.remove(), 160);
      resolve(val);
    };
    const onKey = e => {
      if (e.key === "Escape") close(false);
      else if (e.key === "Enter") close(true);
    };
    cancel.addEventListener("click", () => close(false));
    ok.addEventListener("click", () => close(true));
    back.addEventListener("click", e => { if (e.target === back) close(false); });
    document.addEventListener("keydown", onKey);
    setTimeout(() => ok.focus(), 40);
  });
}

const state = { view: "vault", project: "", hideGreen: false, showArchived: false, editing: null, llmConfigured: false, scrubEnabled: false };

// ---------- chrome ----------
function initTheme() {
  const t = $("#themeToggle");
  t.checked = document.documentElement.dataset.theme === "dark";
  t.addEventListener("change", () => {
    if (t.checked) { document.documentElement.dataset.theme = "dark"; localStorage.setItem("cogo.theme", "dark"); }
    else { delete document.documentElement.dataset.theme; localStorage.setItem("cogo.theme", "light"); }
    window.dispatchEvent(new Event("cogo-theme"));
  });
}

function initMenu() {
  const menu = $("#menu");
  $("#kebab").addEventListener("click", async e => {
    e.stopPropagation();
    menu.classList.toggle("hidden");
    if (!menu.classList.contains("hidden")) {
      try { const c = await api("/api/config"); state.tokens = c.tokens || 0; updateTokenBadge(); } catch (_) {}
    }
  });
  menu.addEventListener("click", e => e.stopPropagation());
  document.addEventListener("click", () => menu.classList.add("hidden"));
  $("#settingsBtn").addEventListener("click", openSettings);
  $("#aboutBtn").addEventListener("click", () => { $("#aboutModal").classList.remove("hidden"); menu.classList.add("hidden"); });
  $("#aboutClose").addEventListener("click", () => $("#aboutModal").classList.add("hidden"));
  $("#aboutModal").addEventListener("click", e => { if (e.target.id === "aboutModal") $("#aboutModal").classList.add("hidden"); });
}

function initTabs() {
  $$(".tab").forEach(b => b.addEventListener("click", () => {
    state.editing = null; // salir a una pestaña siempre cierra el editor abierto
    state.view = b.dataset.view;
    $$(".tab").forEach(x => x.classList.toggle("active", x === b));
    render();
  }));
}

function fmtTokens(n) {
  n = n || 0;
  if (n >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, "") + "M";
  if (n >= 1e3) return (n / 1e3).toFixed(1).replace(/\.0$/, "") + "k";
  return String(n);
}
function updateTokenBadge() {
  const node = $("#menuTokens");
  if (node) node.textContent = "≈ " + fmtTokens(state.tokens) + " tokens IA";
}

async function loadConfig() {
  const c = await api("/api/config");
  state.llmConfigured = !!c.llm_configured;
  state.scrubEnabled = !!c.scrub_enabled;
  state.tokens = c.tokens || 0;
  updateTokenBadge();
  $("#aboutVersion").textContent = c.version;
  $("#aboutCount").textContent = c.count;
  const sel = $("#projsel");
  (c.projects || []).forEach(p => { const o = el("option", null, p); o.value = p; sel.appendChild(o); });
  sel.addEventListener("change", () => { state.editing = null; state.project = sel.value; render(); });
}

// ---------- shared ----------
function matchesProject(n) { return !state.project || n.project === state.project; }

function legend(notes) {
  const counts = { green: 0, yellow: 0, red: 0, ungraded: 0 };
  notes.forEach(n => counts[n.color] = (counts[n.color] || 0) + 1);
  const wrap = el("div", "legend");
  [["green", "verde"], ["yellow", "amarillo"], ["red", "rojo"], ["ungraded", "s/grado"]].forEach(([c, label]) => {
    if (!counts[c]) return;
    const lg = el("span", "lg " + cls(c));
    lg.appendChild(el("span", "dot"));
    lg.appendChild(el("span", null, counts[c] + " " + label));
    wrap.appendChild(lg);
  });
  return wrap;
}

// edgeLegend: muestra qué significa cada color/estilo de arista presente en el grafo.
function edgeLegend(edges) {
  const kinds = window.CogoGraphKinds || {};
  const present = new Set(edges.map(e => e.kind));
  const wrap = el("div", "edge-legend");
  ["depends_on", "supersedes", "caused_by", "wikilink"].forEach(k => {
    if (!present.has(k) || !kinds[k]) return;
    const item = el("span", "el-item");
    const NS = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(NS, "svg");
    svg.setAttribute("width", "26"); svg.setAttribute("height", "10"); svg.setAttribute("class", "el-swatch");
    const ln = document.createElementNS(NS, "line");
    ln.setAttribute("x1", "1"); ln.setAttribute("y1", "5"); ln.setAttribute("x2", "25"); ln.setAttribute("y2", "5");
    ln.setAttribute("stroke", kinds[k].color); ln.setAttribute("stroke-width", "2.4"); ln.setAttribute("stroke-linecap", "round");
    if (kinds[k].dash.length) ln.setAttribute("stroke-dasharray", kinds[k].dash.join(" "));
    svg.appendChild(ln);
    item.appendChild(svg);
    item.appendChild(el("span", null, kinds[k].label));
    wrap.appendChild(item);
  });
  return wrap;
}

function render() {
  const main = $("#main");
  main.innerHTML = "";
  if (state.editing) { renderEditor(main); return; }
  ({ vault: renderVault, fresh: renderFresh, pack: renderPack, graph: renderGraph, lint: renderLint, guard: renderGuard }[state.view])(main);
}

// ---------- vault ----------
function renderWelcome(main) {
  const w = el("div", "welcome");
  const img = el("img", "welcome-logo"); img.src = "/cogo.svg"; img.alt = "";
  w.appendChild(img);
  w.appendChild(el("h2", "welcome-h", "Tu vault está vacío"));
  w.appendChild(el("p", "welcome-sub", "COGO recuerda lo que sabés de tu proyecto y le pone un color de confianza. Cada nota dice qué tan confiable es —y por qué."));
  const leg = el("div", "welcome-legend");
  [["green", "verde · verificado"], ["yellow", "amarillo · probable"], ["red", "rojo · suposición"]].forEach(([c, t]) => {
    const s = el("span", "lg " + cls(c)); s.appendChild(el("span", "dot")); s.appendChild(el("span", null, t)); leg.appendChild(s);
  });
  w.appendChild(leg);
  const btn = el("button", "welcome-btn", "Crear primera nota");
  btn.addEventListener("click", () => openEditor(null));
  w.appendChild(btn);
  main.appendChild(w);
}

// stateLabel names the lifecycle state in Spanish for the badge.
function stateLabel(s) {
  return s === "archived" ? "archivada"
    : s === "superseded" ? "reemplazada"
    : s === "retracted" ? "retractada" : s;
}

async function archiveNote(id) {
  await api("/api/archive?id=" + encodeURIComponent(id), { method: "POST" });
  render();
}
async function restoreNote(id) {
  await api("/api/restore?id=" + encodeURIComponent(id), { method: "POST" });
  render();
}
async function deleteNote(id) {
  const ok = await confirmDialog({
    title: "Borrar nota",
    note: id,
    message: "Sale de COGO y se mueve a la papelera del vault (.cogo/trash). Es recuperable a mano, pero deja de aparecer en la app.",
    hint: "¿Solo querés sacarla del grafo sin perderla? Usá archivar.",
    confirmText: "Borrar",
    danger: true,
  });
  if (!ok) return;
  await api("/api/delete?id=" + encodeURIComponent(id), { method: "POST" });
  render();
}

async function renderVault(main) {
  const notes = (await api("/api/notes" + (state.showArchived ? "?archived=1" : ""))).filter(matchesProject);
  if (!notes.length && !state.project && !state.showArchived) { renderWelcome(main); return; }
  viewHead(main, "Suite Escriba · Memoria", "Vault", "Todo lo que sabés del proyecto, con un color de confianza que COGO computa solo: verde confiá, amarillo ojo, rojo no.");
  const bar = el("div", "viewbar");
  const addBtn = el("button", "mini", "+ Nueva nota");
  addBtn.addEventListener("click", () => openEditor(null));
  bar.appendChild(addBtn);
  bar.appendChild(legend(notes));
  const hg = el("label", "hg");
  const cb = el("input"); cb.type = "checkbox"; cb.checked = state.hideGreen;
  cb.addEventListener("change", () => { state.hideGreen = cb.checked; render(); });
  hg.appendChild(cb); hg.appendChild(el("span", null, "ocultar verdes"));
  bar.appendChild(hg);
  const ha = el("label", "hg");
  const acb = el("input"); acb.type = "checkbox"; acb.checked = state.showArchived;
  acb.addEventListener("change", () => { state.showArchived = acb.checked; render(); });
  ha.appendChild(acb); ha.appendChild(el("span", null, "mostrar archivadas"));
  bar.appendChild(ha);
  main.appendChild(bar);

  const shown = notes.filter(n => !(state.hideGreen && n.color === "green"));
  if (!shown.length) { main.appendChild(el("div", "empty", "Sin notas para mostrar.")); return; }

  const list = el("div", "note-list");
  shown.forEach(n => {
    const card = el("div", "note-card " + cls(n.color) + (n.state ? " archived" : ""));
    card.addEventListener("click", () => openEditor(n.id));
    card.appendChild(el("span", "dot"));
    const body = el("div", "nc-body");
    const head = el("div", "nc-head");
    head.appendChild(el("span", "nc-id", n.id));
    head.appendChild(el("span", "nc-type", n.type + (n.project ? " · " + n.project : "")));
    if (n.state) head.appendChild(el("span", "nc-badge", stateLabel(n.state)));
    if (n.stale_at) {
      const f = freshnessLabel(n.stale_at);
      const st = el("span", "nc-stale " + f.cls, f.text);
      st.title = "Fresca hasta " + n.stale_at + " · después conviene revalidar (pestaña Frescura).";
      head.appendChild(st);
    }
    body.appendChild(head);
    body.appendChild(el("div", "nc-claim", n.claim || "—"));
    body.appendChild(el("div", "nc-reason", n.reason));

    const acts = el("div", "nc-actions");
    acts.addEventListener("click", e => e.stopPropagation());
    if (n.state === "archived" || n.state === "retracted") {
      const rb = el("button", "nc-act", "restaurar");
      rb.addEventListener("click", () => restoreNote(n.id));
      acts.appendChild(rb);
    } else if (!n.state) {
      const ab = el("button", "nc-act", "archivar");
      ab.addEventListener("click", () => archiveNote(n.id));
      acts.appendChild(ab);
    }
    const db = el("button", "nc-act danger", "borrar");
    db.addEventListener("click", () => deleteNote(n.id));
    acts.appendChild(db);
    body.appendChild(acts);

    card.appendChild(body);
    list.appendChild(card);
  });
  main.appendChild(list);
}

// ---------- freshness ----------
function daysUntil(iso) {
  const today = new Date(); today.setHours(0, 0, 0, 0);
  return Math.round((new Date(iso + "T00:00:00") - today) / 86400000);
}
// freshnessLabel muestra el vencimiento de frescura en forma RELATIVA, para que
// no se confunda con la fecha de hoy (stale_at es futuro: cuándo revalidar).
function freshnessLabel(iso) {
  const d = daysUntil(iso);
  if (d > 1) return { text: "↻ vence en " + d + "d", cls: "" };
  if (d === 1) return { text: "↻ vence mañana", cls: "warn" };
  if (d === 0) return { text: "↻ vence hoy", cls: "warn" };
  return { text: "↻ vencida hace " + (-d) + "d", cls: "bad" };
}

async function renderFresh(main) {
  const notes = (await api("/api/notes")).filter(matchesProject).filter(n => n.stale_at);
  const rows = notes.map(n => {
    const days = daysUntil(n.stale_at);
    const status = days < 0 ? "vencida" : (days <= 30 ? "pronto" : "fresca");
    return { ...n, days, status };
  }).filter(r => r.status !== "fresca");
  rows.sort((a, b) => a.stale_at < b.stale_at ? -1 : 1);

  viewHead(main, "Suite Escriba · Memoria", "Frescura", "Las cosas caducan: acá están las notas vencidas o por vencer en ≤30 días. Revalidá una que ya chequeaste.");
  if (!rows.length) { main.appendChild(el("div", "empty", "Nada vencido ni por vencer. Todo fresco.")); return; }

  rows.forEach(r => {
    const row = el("div", "fresh-row " + cls(r.color));
    row.appendChild(el("span", "status", r.status));
    row.appendChild(el("span", "dot"));
    row.appendChild(el("span", "fid", r.id));
    row.appendChild(el("span", "fwhen", r.days < 0 ? `hace ${-r.days}d` : `en ${r.days}d`));
    const btn = el("button", "mini ghost", "revalidar");
    btn.addEventListener("click", async () => {
      btn.disabled = true;
      await api("/api/verify?id=" + encodeURIComponent(r.id), { method: "POST" });
      render();
    });
    row.appendChild(btn);
    main.appendChild(row);
  });
}

// ---------- pack preview ----------
let packTimer = null;
async function renderPack(main) {
  viewHead(main, "Suite Escriba · Memoria", "Pack", "Armá el contexto coloreado de un tema para pasárselo a una IA. El rojo se degrada solo — la política viaja en el pack.");
  const form = el("div", "pack-form");
  const q = el("input", "q"); q.placeholder = "tema, ej: redis"; q.value = window.__packQ || "";
  const b = el("input", "b"); b.type = "number"; b.placeholder = "budget tokens"; b.value = window.__packB || "";
  form.appendChild(q); form.appendChild(b);
  main.appendChild(form);

  const summary = el("div", "pack-summary");
  const pre = el("pre", "pack-md");
  const copyRow = el("div", "copy-row");
  const copyBtn = el("button", "mini ghost", "copiar");
  copyRow.appendChild(copyBtn);
  main.appendChild(summary); main.appendChild(pre); main.appendChild(copyRow);

  async function run() {
    window.__packQ = q.value; window.__packB = b.value;
    const params = new URLSearchParams({ query: q.value, project: state.project, budget: b.value || "0" });
    const p = await api("/api/pack?" + params.toString());
    summary.innerHTML = "";
    [["green", p.greens, "verde"], ["yellow", p.yellows, "amarillo"], ["red", p.reds, "rojo"]].forEach(([c, n, label]) => {
      const s = el("span", "lg " + cls(c)); s.appendChild(el("span", "dot")); s.appendChild(el("span", null, n + " " + label));
      summary.appendChild(s);
    });
    if (p.dropped) summary.appendChild(el("span", null, p.dropped + " omitidas"));
    summary.appendChild(el("span", "tok", "~" + p.tokens + " tokens"));
    pre.textContent = p.markdown;
    copyBtn.onclick = () => { navigator.clipboard.writeText(p.markdown); copyBtn.textContent = "copiado"; setTimeout(() => copyBtn.textContent = "copiar", 1200); };
  }
  const debounced = () => { clearTimeout(packTimer); packTimer = setTimeout(run, 250); };
  q.addEventListener("input", debounced);
  b.addEventListener("input", debounced);
  run();
}

// ---------- graph (motor Canvas: graph.js) ----------
async function renderGraph(main) {
  const g = await api("/api/graph");
  if (!g.nodes.length) { main.appendChild(el("div", "empty", "Sin notas para graficar.")); return; }
  const nodes = g.nodes.filter(matchesProject);
  const keep = new Set(nodes.map(n => n.id));
  const edges = g.edges.filter(e => keep.has(e.from) && keep.has(e.to));
  if (!nodes.length) { main.appendChild(el("div", "empty", "Sin notas para este proyecto.")); return; }

  viewHead(main, "Suite Escriba · Memoria", "Grafo", "Cómo se relacionan tus notas, pintadas por confianza. Mirálo en 2D o entrá a la constelación 3D.");
  const view = el("div", "graph-view");
  const bar = el("div", "viewbar graph-bar");
  bar.appendChild(legend(nodes));
  bar.appendChild(el("span", "gb-sp"));
  const seg = el("div", "seg");
  const b2 = el("button", "seg-btn", "2D"), b3 = el("button", "seg-btn", "3D");
  seg.appendChild(b2); seg.appendChild(b3);
  const reset = el("button", "mini ghost", "recentrar");
  const fs = el("button", "mini ghost", "⛶ pantalla completa");
  bar.appendChild(seg); bar.appendChild(reset); bar.appendChild(fs);
  view.appendChild(bar);

  if (edges.length) view.appendChild(edgeLegend(edges));

  const wrap = el("div", "graph-wrap");
  view.appendChild(wrap);
  main.appendChild(view);

  fs.addEventListener("click", () => {
    if (document.fullscreenElement) document.exitFullscreen();
    else view.requestFullscreen().catch(() => {});
  });
  const onFs = () => { fs.textContent = document.fullscreenElement ? "⛶ salir" : "⛶ pantalla completa"; };
  document.addEventListener("fullscreenchange", onFs);

  const mode = window.__graphMode || "2d";
  const setActive = m => { b2.classList.toggle("on", m === "2d"); b3.classList.toggle("on", m === "3d"); };
  setActive(mode);
  const gv = CogoGraph.mount(wrap, { nodes, edges }, { mode, onSelect: id => openNoteModal(id) });
  b2.addEventListener("click", () => { window.__graphMode = "2d"; setActive("2d"); gv.setMode("2d"); });
  b3.addEventListener("click", () => { window.__graphMode = "3d"; setActive("3d"); gv.setMode("3d"); });
  reset.addEventListener("click", () => gv.resetView());

  // Muestras SVG con el mismo estilo de línea que el grafo, para que se distingan.
  const EDGE_DASH = { depends_on: "", supersedes: "11 6", caused_by: "4 4", wikilink: "1.5 5" };
  const EDGE_W = { depends_on: 2, supersedes: 2.2, caused_by: 2, wikilink: 1.6 };
  const lg = el("div", "edge-legend");
  [["depends_on", "depende de"], ["supersedes", "reemplaza"], ["caused_by", "causado por"], ["wikilink", "relaciona"]].forEach(([k, label]) => {
    const s = el("span");
    s.innerHTML = `<svg width="32" height="10" viewBox="0 0 32 10" aria-hidden="true"><line x1="1" y1="5" x2="31" y2="5" stroke="currentColor" stroke-width="${EDGE_W[k]}" stroke-dasharray="${EDGE_DASH[k]}" stroke-linecap="round"/></svg>`;
    s.appendChild(el("span", null, label));
    lg.appendChild(s);
  });
  main.appendChild(lg);
}

// ---------- editor / capture (the user-friendly front door) ----------
const TYPES = [["bug", "bug"], ["decision", "decisión"], ["architecture", "arquitectura"], ["runbook", "runbook"], ["constraint", "restricción"], ["command", "comando"], ["mistake", "error aprendido"]];
const KINDS = [["file_read", "archivo leído"], ["direct_log", "log observado"], ["command_output", "salida de comando"], ["test_result", "resultado de test"], ["doc", "documentación"], ["testimony", "testimonio"], ["inference", "inferencia"], ["hypothesis", "hipótesis"], ["absence", "ausencia (no hay señal)"]];

function colorWord(c) {
  return ({ green: "Verde — verificado", yellow: "Amarillo — probable", red: "Rojo — suposición / no confiar", ungraded: "Sin grado (informativo)" })[c] || c;
}
function field(labelText, control) {
  const f = el("div", "field");
  f.appendChild(el("label", "field-lbl", labelText));
  f.appendChild(control);
  return f;
}
function select(options, value, onchange) {
  const s = el("select");
  options.forEach(([v, label]) => { const o = el("option", null, label); o.value = v; if (v === value) o.selected = true; s.appendChild(o); });
  s.addEventListener("change", () => onchange(s.value));
  return s;
}

// relField/relSelect: piezas del bloque de relaciones del editor.
function relField(label, node) {
  const w = el("div", "rel-field");
  w.appendChild(el("label", "rel-lbl", label));
  w.appendChild(node);
  return w;
}
function relSelect(ids, value, onchange) {
  const s = el("select");
  const none = el("option", null, "— ninguna —"); none.value = ""; s.appendChild(none);
  ids.forEach(o => { const op = el("option", null, o); op.value = o; if (o === value) op.selected = true; s.appendChild(op); });
  s.addEventListener("change", () => onchange(s.value));
  return s;
}

// paintEvBadge pinta el resultado del resolver de evidencia en una fila del editor.
function paintEvBadge(node, status) {
  const map = {
    resolved: ["✓ resuelve", "ev-status ok", "El archivo citado existe."],
    broken: ["✗ no resuelve", "ev-status bad", "El archivo citado no existe → esta evidencia NO cuenta para el color."],
    unchecked: ["— sin chequear", "ev-status muted", "COGO no puede verificar esta ref sin conexión (log, comando, URL o ruta sin raíz)."],
  };
  const [text, className, title] = map[status] || ["", "ev-status", ""];
  node.textContent = text;
  node.className = className;
  node.title = title;
}

// openNoteModal: vista de solo lectura de una nota (clic en un nodo del grafo).
// Renderiza el cuerpo como Markdown, muestra evidencia (con badges), relaciones y
// un botón "Editar". Se monta dentro del elemento fullscreen si hay uno activo.
async function openNoteModal(id) {
  const n = await api("/api/note?id=" + encodeURIComponent(id)).catch(() => null);
  if (!n || !n.id) return;
  const back = el("div", "modal-back confirm-back note-modal-back");
  const card = el("div", "modal-card note-modal");
  card.appendChild(el("h2", "modal-tit", n.id));

  const meta = el("div", "nm-meta");
  meta.appendChild(el("span", "nm-type", n.type + (n.project ? " · " + n.project : "")));
  const col = el("span", "nm-color " + cls(n.color));
  col.appendChild(el("span", "dot")); col.appendChild(el("strong", null, colorWord(n.color)));
  meta.appendChild(col);
  card.appendChild(meta);
  card.appendChild(el("div", "nm-reason", n.reason));

  const body = el("div", "nm-body md-render");
  body.innerHTML = mdToHtml(n.body);
  card.appendChild(body);

  if (n.evidence && n.evidence.length) {
    const ev = el("div", "nm-block");
    ev.appendChild(el("div", "nm-label", "Evidencia"));
    n.evidence.forEach(e => {
      const row = el("div", "nm-ev-row");
      row.appendChild(el("span", "nm-ev-kind", e.kind));
      row.appendChild(el("span", "nm-ev-ref", e.ref));
      if (e.status) { const b = el("span"); paintEvBadge(b, e.status); row.appendChild(b); }
      ev.appendChild(row);
    });
    card.appendChild(ev);
  }
  const rels = [];
  if (n.depends_on && n.depends_on.length) rels.push(["depende de", n.depends_on.join(", ")]);
  if (n.supersedes) rels.push(["reemplaza a", n.supersedes]);
  if (n.caused_by) rels.push(["causada por", n.caused_by]);
  if (rels.length) {
    const rl = el("div", "nm-block");
    rl.appendChild(el("div", "nm-label", "Relaciones"));
    rels.forEach(([k, v]) => {
      const r = el("div", "nm-rel-row");
      r.appendChild(el("span", "nm-rel-k", k));
      r.appendChild(el("span", "nm-rel-v", v));
      rl.appendChild(r);
    });
    card.appendChild(rl);
  }

  const acc = el("div", "modal-acciones");
  const closeBtn = el("button", "ghost", "Cerrar");
  const editBtn = el("button", null, "Editar");
  acc.appendChild(closeBtn); acc.appendChild(editBtn);
  card.appendChild(acc);
  back.appendChild(card);
  (document.fullscreenElement || document.body).appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));

  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  closeBtn.addEventListener("click", close);
  editBtn.addEventListener("click", () => { close(); if (document.fullscreenElement) document.exitFullscreen(); openEditor(id); });
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
  body.querySelectorAll(".wikilink").forEach(a => a.addEventListener("click", () => { close(); openNoteModal(a.dataset.id); }));
}

async function openEditor(id) {
  let d = { id: "", type: "bug", project: state.project || "", body: "## Claim\n", evidence: [], check_test: "", depends_on: [], supersedes: "", caused_by: "" };
  const all = await api("/api/notes?archived=1").catch(() => []);
  state.editIds = (all || []).map(n => n.id);
  if (id) {
    const n = await api("/api/note?id=" + encodeURIComponent(id));
    d = { id: n.id, type: n.type, project: n.project || "", body: n.body || "## Claim\n", evidence: (n.evidence || []).map(e => ({ kind: e.kind, ref: e.ref, status: e.status })), check_test: n.check_test || "", depends_on: n.depends_on || [], supersedes: n.supersedes || "", caused_by: n.caused_by || "" };
  }
  state.editing = d;
  render();
}

function renderEditor(main) {
  const d = state.editing;
  const head = el("div", "editor-head");
  const back = el("button", "mini ghost", "← volver");
  back.addEventListener("click", () => { state.editing = null; render(); });
  head.appendChild(back);
  head.appendChild(el("h2", "editor-title", d.id ? "Editar · " + d.id : "Nueva nota"));
  main.appendChild(head);

  const form = el("div", "editor");
  if (state.scrubEnabled) form.appendChild(el("div", "scrub-note", "Las capturas se limpian con Anonimal (secretos/PII) antes de guardar."));
  const prev = el("div", "color-preview");
  const evBadges = []; // uno por fila de evidencia, refrescado por preview()
  let timer = null;
  function preview() {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      const p = await api("/api/preview", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(d) });
      prev.className = "color-preview " + cls(p.color);
      prev.innerHTML = "";
      prev.appendChild(el("span", "dot"));
      prev.appendChild(el("strong", null, colorWord(p.color)));
      prev.appendChild(el("span", "cp-reason", p.reason));
      // reflejar el resultado del resolver de evidencia en las badges
      (p.evidence || []).forEach((e, i) => {
        if (d.evidence[i]) d.evidence[i].status = e.status;
        if (evBadges[i]) paintEvBadge(evBadges[i], e.status);
      });
    }, 300);
  }

  const row1 = el("div", "form-row");
  row1.appendChild(field("Tipo", select(TYPES, d.type, v => { d.type = v; preview(); })));
  const proj = el("input"); proj.value = d.project; proj.placeholder = "proyecto";
  proj.addEventListener("input", () => { d.project = proj.value; preview(); });
  row1.appendChild(field("Proyecto", proj));
  form.appendChild(row1);

  const mdEd = el("div", "md-editor");
  const body = el("textarea", "md"); body.value = d.body; body.setAttribute("rows", "10");
  const previewPane = el("div", "md-render md-preview");
  const syncPrev = () => { if (mdEd.classList.contains("split")) previewPane.innerHTML = mdToHtml(body.value); };
  const touched = () => { d.body = body.value; preview(); syncPrev(); };
  body.addEventListener("input", touched);

  const ins = (before, after, ph) => {
    const s = body.selectionStart, e = body.selectionEnd, sel = body.value.slice(s, e) || ph || "";
    body.value = body.value.slice(0, s) + before + sel + after + body.value.slice(e);
    body.focus(); body.selectionStart = s + before.length; body.selectionEnd = s + before.length + sel.length;
    touched();
  };
  const linePfx = pfx => {
    const s = body.selectionStart, ls = body.value.lastIndexOf("\n", s - 1) + 1;
    body.value = body.value.slice(0, ls) + pfx + body.value.slice(ls);
    body.focus(); body.selectionStart = body.selectionEnd = s + pfx.length; touched();
  };
  const tbRow = el("div", "md-tb-row");
  [["B", () => ins("**", "**", "negrita"), "negrita"],
   ["I", () => ins("*", "*", "itálica"), "itálica"],
   ["‹›", () => ins("`", "`", "código"), "código"],
   ["H", () => linePfx("## "), "encabezado"],
   ["—", () => linePfx("- "), "lista"],
   ["❝", () => linePfx("> "), "cita"],
   ["🔗", () => ins("[", "](url)", "texto"), "link"]].forEach(([lab, fn, title]) => {
    const btn = el("button", "md-tb", lab); btn.type = "button"; btn.title = title;
    btn.addEventListener("click", ev => { ev.preventDefault(); fn(); });
    tbRow.appendChild(btn);
  });
  tbRow.appendChild(el("span", "md-tb-sp"));
  const prevBtn = el("button", "md-tb md-prev", "vista previa"); prevBtn.type = "button";
  prevBtn.addEventListener("click", ev => {
    ev.preventDefault();
    const on = mdEd.classList.toggle("split");
    prevBtn.classList.toggle("on", on);
    prevBtn.textContent = on ? "ocultar preview" : "vista previa";
    syncPrev();
  });
  tbRow.appendChild(prevBtn);

  mdEd.appendChild(tbRow);
  const bodyRow = el("div", "md-body-row");
  bodyRow.appendChild(body); bodyRow.appendChild(previewPane);
  mdEd.appendChild(bodyRow);
  form.appendChild(field("Nota (markdown) — empezá con ## Claim", mdEd));

  const evWrap = el("div", "ev-wrap");
  function renderEv() {
    evWrap.innerHTML = "";
    evBadges.length = 0;
    if (!d.evidence.length) evWrap.appendChild(el("div", "ev-empty", "Sin evidencia → la nota nace roja (suposición)."));
    d.evidence.forEach((e, i) => {
      const row = el("div", "ev-row");
      row.appendChild(select(KINDS, e.kind, v => { d.evidence[i].kind = v; preview(); }));
      const ref = el("input"); ref.value = e.ref; ref.placeholder = "ref real: archivo:línea, commit, log+hora, url";
      ref.addEventListener("input", () => { d.evidence[i].ref = ref.value; preview(); });
      row.appendChild(ref);
      const badge = el("span", "ev-status");
      paintEvBadge(badge, e.status);
      evBadges[i] = badge;
      row.appendChild(badge);
      const rm = el("button", "icon-btn ev-x"); rm.textContent = "×";
      rm.addEventListener("click", () => { d.evidence.splice(i, 1); renderEv(); preview(); });
      row.appendChild(rm);
      evWrap.appendChild(row);
    });
    const add = el("button", "mini ghost", "+ evidencia");
    add.addEventListener("click", () => { d.evidence.push({ kind: "file_read", ref: "" }); renderEv(); });
    evWrap.appendChild(add);
  }
  renderEv();
  form.appendChild(field("Evidencia", evWrap));

  const chk = el("input"); chk.value = d.check_test; chk.placeholder = "test mínimo que verificaría el claim";
  chk.addEventListener("input", () => { d.check_test = chk.value; preview(); });
  form.appendChild(field("Check mínimo", chk));

  // ---- relaciones (manuales) ----
  const others = (state.editIds || []).filter(x => x !== d.id);
  const relWrap = el("div", "rel-wrap");
  // depends_on: multi, con chips
  const depBox = el("div", "rel-deps");
  function renderDeps() {
    depBox.innerHTML = "";
    d.depends_on.forEach((dep, i) => {
      const chip = el("span", "rel-chip");
      chip.appendChild(el("span", null, dep));
      const x = el("button", "rel-chip-x", "×");
      x.addEventListener("click", () => { d.depends_on.splice(i, 1); renderDeps(); preview(); });
      chip.appendChild(x);
      depBox.appendChild(chip);
    });
    const avail = others.filter(o => !d.depends_on.includes(o));
    if (avail.length) {
      const pick = el("select", "rel-add");
      const ph = el("option", null, "+ depende de…"); ph.value = ""; pick.appendChild(ph);
      avail.forEach(o => { const op = el("option", null, o); op.value = o; pick.appendChild(op); });
      pick.addEventListener("change", () => { if (pick.value) { d.depends_on.push(pick.value); renderDeps(); preview(); } });
      depBox.appendChild(pick);
    }
  }
  renderDeps();
  relWrap.appendChild(relField("Depende de (dura: si es roja, esta cae a roja)", depBox));
  // supersedes + caused_by: single
  relWrap.appendChild(relField("Reemplaza a (la archiva)", relSelect(others, d.supersedes, v => { d.supersedes = v; preview(); })));
  relWrap.appendChild(relField("Causada por", relSelect(others, d.caused_by, v => { d.caused_by = v; preview(); })));
  form.appendChild(field("Relaciones (opcional) — o escribí [[id]] en la nota", relWrap));

  form.appendChild(field("Color computado (preview en vivo)", prev));

  const actions = el("div", "editor-actions");
  const cancel = el("button", "ghost", "Cancelar");
  cancel.addEventListener("click", () => { state.editing = null; render(); });
  const save = el("button", null, "Guardar");
  save.addEventListener("click", async () => {
    if (!d.type || !d.body.trim()) return;
    save.disabled = true;
    await api("/api/capture", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(d) });
    state.editing = null; state.view = "vault";
    $$(".tab").forEach(x => x.classList.toggle("active", x.dataset.view === "vault"));
    render();
  });
  actions.appendChild(cancel); actions.appendChild(save);
  form.appendChild(actions);

  main.appendChild(form);
  preview();
}

// ---------- revisión (lint) + ajustes ----------
async function renderLint(main) {
  viewHead(main, "Suite Escriba · Memoria", "Revisión", "Enlaces rotos, notas vencidas y —si conectaste un modelo— contradicciones entre notas. Una contradicción pinta esa nota de rojo en todo el visor.");

  const bar = el("div", "viewbar");
  const btn = el("button", null, "Revisar ahora");
  const status = el("span", "lint-status");
  bar.appendChild(btn); bar.appendChild(status);
  main.appendChild(bar);

  if (!state.llmConfigured) {
    const hint = el("div", "lint-hint");
    hint.appendChild(el("span", null, "Para detectar contradicciones, conectá un modelo en "));
    const a = el("a", "link", "Ajustes"); a.addEventListener("click", openSettings); hint.appendChild(a);
    hint.appendChild(el("span", null, "."));
    main.appendChild(hint);
  }

  const out = el("div", "lint-out");
  main.appendChild(out);

  btn.addEventListener("click", async () => {
    btn.disabled = true; status.textContent = "revisando…";
    const r = await api("/api/lint", { method: "POST" });
    btn.disabled = false;
    status.textContent = r.llm_used ? ("modelo: " + r.pairs_checked + "/" + r.candidate_pairs + " pares revisados") : "checks deterministas (sin modelo)";
    out.innerHTML = "";
    if (!r.issues || !r.issues.length) { out.appendChild(el("div", "empty", "Todo limpio. Nada que revisar.")); return; }
    const LABEL = { contradiction: "Contradicción", broken_dep: "Enlace roto", stale: "Vencida" };
    r.issues.forEach(is => {
      const row = el("div", "lint-row lint-" + is.kind);
      row.appendChild(el("span", "lint-tag", LABEL[is.kind] || is.kind));
      row.appendChild(el("span", "lint-msg", is.msg));
      out.appendChild(row);
    });
    if (r.contradictions > 0) out.appendChild(el("div", "lint-redmsg", r.contradictions + " nota(s) quedaron en rojo por contradicción — miralas en Vault."));
  });
}

// ---------- guard (anti-manipulación) ----------
function parseTranscript(text) {
  const turns = [];
  text.split("\n").forEach(line => {
    const m = line.match(/^\s*([UuMm])\s*:\s*(.*)$/);
    if (m) turns.push({ role: m[1].toLowerCase() === "u" ? "user" : "model", text: m[2] });
    else if (line.trim() && turns.length) turns[turns.length - 1].text += "\n" + line;
  });
  return turns;
}

const COLORWORD = { green: "Verde — sin señales", yellow: "Amarillo — señales presentes", red: "Rojo — hay mecánica: recibos o línea roja" };

async function renderGuard(main) {
  viewHead(main, "Suite Escriba · Autonomía", "Guard",
    "Radiografía un turno de cualquier modelo: nombra las tácticas con su evidencia, caza las " +
    "contradicciones contra la transcripción (los recibos) y mide deriva contra tus líneas rojas. " +
    "No censura: te muestra, vos decidís.");

  // --- mandato persistente ---
  const m = await api("/api/mandate");
  const mand = el("div", "guard-mandate");
  mand.appendChild(el("div", "field-lbl", "Tu mandato (queda guardado en el vault)"));
  const goal = el("input"); goal.placeholder = "tu objetivo · ej: decidir mi carrera sin apuro"; goal.value = m.goal || "";
  mand.appendChild(goal);
  const lines = el("textarea", "md"); lines.rows = 3;
  lines.placeholder = "tus líneas rojas, una por renglón · ej:\nno renuncio sin otra oferta firmada\nno invierto plata hoy";
  lines.value = (m.red_lines || []).join("\n");
  mand.appendChild(lines);
  const mrow = el("div", "guard-mrow");
  const msave = el("button", "mini ghost", "guardar mandato");
  const mst = el("span", "lint-status");
  msave.addEventListener("click", async () => {
    await api("/api/mandate", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ goal: goal.value.trim(), red_lines: lines.value.split("\n").map(x => x.trim()).filter(Boolean) }) });
    mst.textContent = "guardado ✓"; setTimeout(() => mst.textContent = "", 1500);
  });
  mrow.appendChild(msave); mrow.appendChild(mst);
  mand.appendChild(mrow);
  main.appendChild(mand);

  // --- la conversación (contexto) primero, el turno a analizar después:
  //     se lee como un chat, el último mensaje abajo ---
  const trans = el("textarea", "md"); trans.rows = 4;
  trans.placeholder = "opcional — la charla hasta acá, un mensaje por renglón:\nU: lo que dijiste vos\nM: lo que respondió el modelo";
  main.appendChild(field("1 · Conversación previa (contexto, para los recibos)", trans));

  const turn = el("textarea", "md"); turn.rows = 5;
  turn.placeholder = "el ÚLTIMO mensaje del modelo — el que se radiografía";
  main.appendChild(field("2 · Turno a analizar (el último mensaje del modelo)", turn));

  const srow = el("label", "hg guard-steel-row");
  const steel = el("input"); steel.type = "checkbox"; steel.disabled = !state.llmConfigured;
  srow.appendChild(steel);
  srow.appendChild(el("span", null, "pedir el otro lado (steelman adversario)" + (state.llmConfigured ? "" : " — necesita un modelo en Ajustes")));
  main.appendChild(srow);

  const bar = el("div", "viewbar");
  const run = el("button", null, "Radiografiar");
  const status = el("span", "lint-status");
  bar.appendChild(run); bar.appendChild(status);
  main.appendChild(bar);

  const out = el("div", "guard-out");
  main.appendChild(out);

  run.addEventListener("click", async () => {
    if (!turn.value.trim()) return;
    run.disabled = true; status.textContent = "analizando…";
    const r = await api("/api/guard", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ turn: turn.value, transcript: parseTranscript(trans.value), steelman: steel.checked }) });
    run.disabled = false; status.textContent = r.mode === "mandato" ? "medido contra tu mandato" : "modo informativo (sin mandato)";
    out.innerHTML = "";

    const verdict = el("div", "color-preview " + cls(r.overall));
    verdict.appendChild(el("span", "dot"));
    verdict.appendChild(el("strong", null, COLORWORD[r.overall] || r.overall));
    verdict.appendChild(el("span", "cp-reason", r.reason));
    out.appendChild(verdict);

    (r.red_lines || []).forEach(h => {
      const row = el("div", "guard-redline");
      row.appendChild(el("span", null, "⚠️ Toca tu línea roja: "));
      row.appendChild(el("strong", null, h.line));
      out.appendChild(row);
    });
    if (r.streak >= 2) out.appendChild(el("div", "guard-streak", "📈 " + r.streak + " turnos consecutivos del modelo con señales."));

    (r.findings || []).forEach(f => {
      const card = el("div", "note-card guard-card " + cls(f.color));
      card.appendChild(el("span", "dot"));
      const body = el("div", "nc-body");
      const head = el("div", "nc-head");
      head.appendChild(el("span", "nc-id", f.name));
      head.appendChild(el("span", "nc-type", f.technique));
      body.appendChild(head);
      body.appendChild(el("div", "nc-reason", f.reason));
      body.appendChild(el("div", "guard-ev", f.evidence));
      (f.receipts || []).forEach(rc => {
        const rec = el("div", "guard-receipt");
        rec.appendChild(el("strong", null, "Recibo (turno " + (rc.turn_index + 1) + "): "));
        rec.appendChild(el("span", null, rc.quote));
        body.appendChild(rec);
      });
      if (f.questions && f.questions.length) {
        const ql = el("ul", "guard-quest");
        f.questions.forEach(q => ql.appendChild(el("li", null, q)));
        body.appendChild(ql);
      }
      body.appendChild(el("div", "guard-move", f.move));
      body.appendChild(el("div", "guard-inoc", "“" + f.inoculation + "”"));
      card.appendChild(body);
      out.appendChild(card);
    });
    if (!(r.findings || []).length) out.appendChild(el("div", "empty", "Sin señales léxicas ni recibos sobre este turno."));

    if (r.steelman) {
      const st = el("div", "guard-steel");
      st.appendChild(el("div", "field-lbl", "🔁 El otro lado (steelman adversario)"));
      st.appendChild(el("div", "guard-steel-pos", "Lo que este turno empuja: " + r.steelman.position));
      st.appendChild(el("div", "guard-steel-body", r.steelman.counter));
      (r.steelman.tests || []).length && st.appendChild(el("div", "field-lbl", "Cómo decidir"));
      (r.steelman.tests || []).forEach(t => st.appendChild(el("div", "guard-steel-test", "· " + t)));
      st.appendChild(el("div", "guard-inoc", "Es otro modelo argumentando el lado contrario a propósito: no es un veredicto, es simetría."));
      out.appendChild(st);
    } else if (r.steelman_note) {
      out.appendChild(el("div", "guard-streak", r.steelman_note));
    }
    out.appendChild(el("div", "guard-cover", "Motor determinista: " + r.covered + "/" + r.total + " técnicas con marcadores; recibos y trayectoria siempre activos."));
  });
}

async function openSettings() {
  $("#menu").classList.add("hidden");
  const s = await api("/api/settings");
  $("#setBase").value = s.base_url || "";
  $("#setModel").value = s.model || "";
  $("#setKey").value = "";
  $("#setKey").placeholder = s.has_key ? "•••• guardada — vacío = no cambiar" : "tu API key";
  const st = $("#setStatus");
  st.textContent = s.configured ? ("activo: " + s.name) : "apagado";
  st.className = "set-status " + (s.configured ? "ok" : "");
  $("#settingsModal").classList.remove("hidden");
}

async function saveSettings() {
  const body = JSON.stringify({ base_url: $("#setBase").value.trim(), model: $("#setModel").value.trim(), api_key: $("#setKey").value });
  const r = await api("/api/settings", { method: "POST", headers: { "Content-Type": "application/json" }, body });
  state.llmConfigured = r.configured;
  return r;
}

// Pregunta al proveedor (endpoint /models) qué modelos ofrece y recomienda cuáles
// sirven para COGO. Funciona con cualquier proveedor OpenAI-compatible.
async function loadModels() {
  const hint = $("#setModelHint"), sel = $("#setModelSelect"), btn = $("#setLoadModels");
  const base = $("#setBase").value.trim(), key = $("#setKey").value;
  if (!base) { hint.textContent = "Primero poné el servidor (base URL)."; hint.className = "model-hint bad"; return; }
  btn.disabled = true; hint.textContent = "buscando modelos…"; hint.className = "model-hint";
  let r;
  try { r = await api("/api/settings/models", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ base_url: base, api_key: key }) }); }
  catch (e) { btn.disabled = false; hint.textContent = "error de red: " + e.message; hint.className = "model-hint bad"; return; }
  btn.disabled = false;
  if (!r.ok) { hint.textContent = "No pude listar modelos (" + (r.error || "?") + "). Escribí el nombre a mano."; hint.className = "model-hint bad"; sel.classList.add("hidden"); return; }
  sel.innerHTML = "";
  const ph = el("option", null, "— elegí un modelo —"); ph.value = ""; sel.appendChild(ph);
  const group = (label, arr) => {
    if (!arr.length) return;
    const g = document.createElement("optgroup"); g.label = label;
    arr.forEach(m => { const o = el("option", null, m.id); o.value = m.id; g.appendChild(o); });
    sel.appendChild(g);
  };
  const rec = r.models.filter(m => m.recommended), rest = r.models.filter(m => !m.recommended);
  group("★ Recomendados para COGO", rec);
  group("Todos los modelos", rest);
  sel.classList.remove("hidden");
  hint.className = "model-hint ok";
  hint.textContent = r.count + " modelos disponibles" + (rec.length ? " · recomendados: " + rec.slice(0, 3).map(m => m.id).join(", ") : " · sin recomendaciones automáticas");
}

function initSettings() {
  const m = $("#settingsModal");
  $("#settingsClose").addEventListener("click", () => m.classList.add("hidden"));
  m.addEventListener("click", e => { if (e.target.id === "settingsModal") m.classList.add("hidden"); });
  const key = $("#setKey");
  $("#setKeyToggle").addEventListener("click", () => { key.type = key.type === "password" ? "text" : "password"; });
  $("#setLoadModels").addEventListener("click", loadModels);
  $("#setModelSelect").addEventListener("change", e => { if (e.target.value) $("#setModel").value = e.target.value; });
  $("#setTest").addEventListener("click", async () => {
    await saveSettings();
    const r = await api("/api/settings/test", { method: "POST" });
    const st = $("#setStatus");
    st.textContent = r.ok ? ("conecta" + (r.name ? " — " + r.name : "")) : ("no conecta: " + r.error);
    st.className = "set-status " + (r.ok ? "ok" : "bad");
  });
  $("#setSave").addEventListener("click", async () => { await saveSettings(); m.classList.add("hidden"); render(); });
}

// showTokenGate: pantalla de acceso por token (COGO protegido con COGO_MCP_TOKEN,
// sin OIDC). Guarda el token en localStorage; api() lo manda como Bearer.
function showTokenGate(withLockatusBack) {
  const gate = $("#loginGate");
  const card = gate.querySelector(".login-card");
  card.innerHTML = "";
  const logo = el("img", "logo"); logo.src = "/cogo.svg"; logo.alt = "";
  card.appendChild(logo);
  card.appendChild(el("h2", null, "COGO"));
  card.appendChild(el("p", "login-sub", "Este COGO está protegido. Ingresá tu token de acceso."));
  const form = el("div", "token-form");
  const inp = el("input"); inp.type = "password"; inp.placeholder = "token de acceso"; inp.autocomplete = "off";
  const btn = el("button", "login-sso", "Entrar");
  const err = el("div", "token-err");
  form.appendChild(inp); form.appendChild(btn);
  card.appendChild(form); card.appendChild(err);
  if (withLockatusBack) {
    const back = el("a", "login-alt", "← Entrar con Lockatus");
    back.addEventListener("click", () => location.reload());
    card.appendChild(back);
  }
  const submit = async () => {
    const t = inp.value.trim();
    if (!t) return;
    localStorage.setItem("cogo.token", t);
    const me2 = await api("/auth/me").catch(() => ({}));
    if (me2.authenticated) { gate.classList.add("hidden"); await loadConfig(); render(); }
    else { localStorage.removeItem("cogo.token"); err.textContent = "Token inválido."; inp.select(); }
  };
  btn.addEventListener("click", submit);
  inp.addEventListener("keydown", e => { if (e.key === "Enter") submit(); });
  gate.classList.remove("hidden");
  setTimeout(() => inp.focus(), 50);
}

// ---------- boot ----------
(async function () {
  initTheme(); initMenu(); initTabs(); initSettings();
  const me = await api("/auth/me").catch(() => ({ enabled: false, authenticated: true }));
  if (me.enabled && !me.authenticated) {
    if (me.mode === "token") {
      showTokenGate(false);
    } else { // OIDC / Lockatus — con la opción de entrar por token también
      $("#loginGate").classList.remove("hidden");
      const alt = el("a", "login-alt", "o entrá con un token de acceso");
      alt.addEventListener("click", () => showTokenGate(true));
      $("#loginGate .login-card").appendChild(alt);
    }
    return;
  }
  if (me.mode === "federated" && me.authenticated) {
    $("#menuUser").textContent = me.name ? (me.name + " · " + me.email) : me.email;
    $("#menuUser").classList.remove("hidden");
    $("#logoutBtn").classList.remove("hidden");
    $("#logoutSep").classList.remove("hidden");
  }
  if (me.mode === "token" && me.authenticated) {
    const lb = $("#logoutBtn"); lb.removeAttribute("href");
    lb.addEventListener("click", e => { e.preventDefault(); localStorage.removeItem("cogo.token"); location.reload(); });
    lb.classList.remove("hidden"); $("#logoutSep").classList.remove("hidden");
  }
  await loadConfig();
  render();
})();
