"use strict";

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const api = (p, opt) => fetch(p, opt).then(r => r.json());
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

const state = { view: "vault", project: "", hideGreen: false, editing: null, llmConfigured: false, scrubEnabled: false };

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
  $("#kebab").addEventListener("click", e => { e.stopPropagation(); menu.classList.toggle("hidden"); });
  menu.addEventListener("click", e => e.stopPropagation());
  document.addEventListener("click", () => menu.classList.add("hidden"));
  $("#settingsBtn").addEventListener("click", openSettings);
  $("#aboutBtn").addEventListener("click", () => { $("#aboutModal").classList.remove("hidden"); menu.classList.add("hidden"); });
  $("#aboutClose").addEventListener("click", () => $("#aboutModal").classList.add("hidden"));
  $("#aboutModal").addEventListener("click", e => { if (e.target.id === "aboutModal") $("#aboutModal").classList.add("hidden"); });
}

function initTabs() {
  $$(".tab").forEach(b => b.addEventListener("click", () => {
    state.view = b.dataset.view;
    $$(".tab").forEach(x => x.classList.toggle("active", x === b));
    render();
  }));
}

async function loadConfig() {
  const c = await api("/api/config");
  state.llmConfigured = !!c.llm_configured;
  state.scrubEnabled = !!c.scrub_enabled;
  $("#aboutVersion").textContent = c.version;
  $("#aboutCount").textContent = c.count;
  const sel = $("#projsel");
  (c.projects || []).forEach(p => { const o = el("option", null, p); o.value = p; sel.appendChild(o); });
  sel.addEventListener("change", () => { state.project = sel.value; render(); });
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

async function renderVault(main) {
  const notes = (await api("/api/notes")).filter(matchesProject);
  if (!notes.length && !state.project) { renderWelcome(main); return; }
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
  main.appendChild(bar);

  const shown = notes.filter(n => !(state.hideGreen && n.color === "green"));
  if (!shown.length) { main.appendChild(el("div", "empty", "Sin notas para mostrar.")); return; }

  const list = el("div", "note-list");
  shown.forEach(n => {
    const card = el("div", "note-card " + cls(n.color));
    card.addEventListener("click", () => openEditor(n.id));
    card.appendChild(el("span", "dot"));
    const body = el("div", "nc-body");
    const head = el("div", "nc-head");
    head.appendChild(el("span", "nc-id", n.id));
    head.appendChild(el("span", "nc-type", n.type + (n.project ? " · " + n.project : "")));
    if (n.stale_at) head.appendChild(el("span", "nc-stale", "↻ " + n.stale_at));
    body.appendChild(head);
    body.appendChild(el("div", "nc-claim", n.claim || "—"));
    body.appendChild(el("div", "nc-reason", n.reason));
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
  const bar = el("div", "viewbar graph-bar");
  bar.appendChild(legend(nodes));
  bar.appendChild(el("span", "gb-sp"));
  const seg = el("div", "seg");
  const b2 = el("button", "seg-btn", "2D"), b3 = el("button", "seg-btn", "3D");
  seg.appendChild(b2); seg.appendChild(b3);
  const reset = el("button", "mini ghost", "recentrar");
  bar.appendChild(seg); bar.appendChild(reset);
  main.appendChild(bar);

  const wrap = el("div", "graph-wrap");
  main.appendChild(wrap);

  const mode = window.__graphMode || "2d";
  const setActive = m => { b2.classList.toggle("on", m === "2d"); b3.classList.toggle("on", m === "3d"); };
  setActive(mode);
  const gv = CogoGraph.mount(wrap, { nodes, edges }, { mode, onSelect: id => openEditor(id) });
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

async function openEditor(id) {
  let d = { id: "", type: "bug", project: state.project || "", body: "## Claim\n", evidence: [], check_test: "" };
  if (id) {
    const n = await api("/api/note?id=" + encodeURIComponent(id));
    d = { id: n.id, type: n.type, project: n.project || "", body: n.body || "## Claim\n", evidence: (n.evidence || []).map(e => ({ kind: e.kind, ref: e.ref })), check_test: n.check_test || "" };
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
    }, 300);
  }

  const row1 = el("div", "form-row");
  row1.appendChild(field("Tipo", select(TYPES, d.type, v => { d.type = v; preview(); })));
  const proj = el("input"); proj.value = d.project; proj.placeholder = "proyecto";
  proj.addEventListener("input", () => { d.project = proj.value; preview(); });
  row1.appendChild(field("Proyecto", proj));
  form.appendChild(row1);

  const body = el("textarea", "md"); body.value = d.body; body.setAttribute("rows", "8");
  body.addEventListener("input", () => { d.body = body.value; preview(); });
  form.appendChild(field("Nota (markdown) — empezá con ## Claim", body));

  const evWrap = el("div", "ev-wrap");
  function renderEv() {
    evWrap.innerHTML = "";
    if (!d.evidence.length) evWrap.appendChild(el("div", "ev-empty", "Sin evidencia → la nota nace roja (suposición)."));
    d.evidence.forEach((e, i) => {
      const row = el("div", "ev-row");
      row.appendChild(select(KINDS, e.kind, v => { d.evidence[i].kind = v; preview(); }));
      const ref = el("input"); ref.value = e.ref; ref.placeholder = "ref real: commit+línea, log+hora, url+fecha";
      ref.addEventListener("input", () => { d.evidence[i].ref = ref.value; preview(); });
      row.appendChild(ref);
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

function initSettings() {
  const m = $("#settingsModal");
  $("#settingsClose").addEventListener("click", () => m.classList.add("hidden"));
  m.addEventListener("click", e => { if (e.target.id === "settingsModal") m.classList.add("hidden"); });
  const key = $("#setKey");
  $("#setKeyToggle").addEventListener("click", () => { key.type = key.type === "password" ? "text" : "password"; });
  $("#setTest").addEventListener("click", async () => {
    await saveSettings();
    const r = await api("/api/settings/test", { method: "POST" });
    const st = $("#setStatus");
    st.textContent = r.ok ? ("conecta" + (r.name ? " — " + r.name : "")) : ("no conecta: " + r.error);
    st.className = "set-status " + (r.ok ? "ok" : "bad");
  });
  $("#setSave").addEventListener("click", async () => { await saveSettings(); m.classList.add("hidden"); render(); });
}

// ---------- boot ----------
(async function () {
  initTheme(); initMenu(); initTabs(); initSettings();
  const me = await api("/auth/me");
  if (me.enabled && !me.authenticated) { $("#loginGate").classList.remove("hidden"); return; }
  if (me.authenticated) {
    $("#menuUser").textContent = me.name ? (me.name + " · " + me.email) : me.email;
    $("#menuUser").classList.remove("hidden");
    $("#logoutBtn").classList.remove("hidden");
    $("#logoutSep").classList.remove("hidden");
  }
  await loadConfig();
  render();
})();
