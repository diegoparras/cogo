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

const state = { view: "vault", project: "", hideGreen: false, editing: null, llmConfigured: false };

// ---------- chrome ----------
function initTheme() {
  const t = $("#themeToggle");
  t.checked = document.documentElement.dataset.theme === "dark";
  t.addEventListener("change", () => {
    if (t.checked) { document.documentElement.dataset.theme = "dark"; localStorage.setItem("cogo.theme", "dark"); }
    else { delete document.documentElement.dataset.theme; localStorage.setItem("cogo.theme", "light"); }
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
  $("#aboutVersion").textContent = c.version;
  $("#aboutCount").textContent = c.count;
  $("#vaultCount").textContent = c.count + " notas";
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
  ({ vault: renderVault, fresh: renderFresh, pack: renderPack, graph: renderGraph, lint: renderLint }[state.view])(main);
}

// ---------- vault ----------
async function renderVault(main) {
  const notes = (await api("/api/notes")).filter(matchesProject);
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

  main.appendChild(el("div", "viewbar")).appendChild(
    el("div", null, "Notas vencidas o que vencen en ≤30 días. Revalidá una que ya chequeaste.")
  );
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

// ---------- graph ----------
const NS = "http://www.w3.org/2000/svg";
function svgEl(tag, attrs) { const e = document.createElementNS(NS, tag); for (const k in attrs) e.setAttribute(k, attrs[k]); return e; }

async function renderGraph(main) {
  const g = await api("/api/graph");
  const nodes = g.nodes.filter(matchesProject);
  const keep = new Set(nodes.map(n => n.id));
  const edges = g.edges.filter(e => keep.has(e.from) && keep.has(e.to));
  if (!nodes.length) { main.appendChild(el("div", "empty", "Sin notas para graficar.")); return; }

  const W = 760, H = 520, cx = W / 2, cy = H / 2, R = Math.min(W, H) / 2 - 80, N = nodes.length;
  const pos = {};
  nodes.forEach((n, i) => { const a = 2 * Math.PI * i / N - Math.PI / 2; pos[n.id] = { x: cx + R * Math.cos(a), y: cy + R * Math.sin(a) }; });

  const svg = svgEl("svg", { viewBox: `0 0 ${W} ${H}`, class: "graph" });
  edges.forEach(e => {
    const a = pos[e.from], b = pos[e.to];
    svg.appendChild(svgEl("line", { x1: a.x, y1: a.y, x2: b.x, y2: b.y, class: "edge edge-" + e.kind }));
  });
  nodes.forEach(n => {
    const p = pos[n.id];
    svg.appendChild(svgEl("circle", { cx: p.x, cy: p.y, r: 8, class: "gnode " + cls(n.color) }));
    const t = svgEl("text", { x: p.x, y: p.y - 13, class: "glabel" });
    t.textContent = n.id.length > 22 ? n.id.slice(0, 21) + "…" : n.id;
    svg.appendChild(t);
  });

  main.appendChild(legend(nodes));
  const wrap = el("div", "graph-wrap"); wrap.appendChild(svg); main.appendChild(wrap);

  const lg = el("div", "edge-legend");
  [["depends_on", "depende de"], ["supersedes", "reemplaza"], ["caused_by", "causado por"], ["wikilink", "relaciona"]].forEach(([k, label]) => {
    const s = el("span"); const i = el("i", k === "wikilink" ? "wiki" : null); s.appendChild(i); s.appendChild(el("span", null, label)); lg.appendChild(s);
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
  main.appendChild(el("p", "lint-intro", "Revisa el vault: enlaces rotos, notas vencidas y —si conectaste un modelo— contradicciones entre notas. Lo que encuentre como contradicción pinta esa nota de rojo en todo el visor."));

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
  await loadConfig();
  render();
})();
