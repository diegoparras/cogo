"use strict";

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const api = (p, opt) => {
  opt = opt || {};
  const tok = localStorage.getItem("cogo.token");
  if (tok) opt.headers = Object.assign({ Authorization: "Bearer " + tok }, opt.headers);
  return fetch(p, opt).then(r => r.json());
};
// apiOrError es api() pero sin tragarse el motivo: devuelve SIEMPRE un objeto con
// un mensaje que se pueda mostrar. Un `catch(() => null)` convierte "esta
// instancia es vieja" y "no tenés acceso" en el mismo "no se pudo", que no le
// sirve a nadie para arreglar nada.
async function apiOrError(path, opt) {
  opt = opt || {};
  const tok = localStorage.getItem("cogo.token");
  if (tok) opt.headers = Object.assign({ Authorization: "Bearer " + tok }, opt.headers);
  let res;
  try { res = await fetch(path, opt); }
  catch (e) { return { ok: false, error: "no se pudo contactar al servidor de COGO" }; }
  if (res.status === 404) return { ok: false, error: "esta instancia de COGO todavía no tiene esta función — actualizá la imagen y redeployá" };
  if (res.status === 401 || res.status === 403) return { ok: false, error: "sesión o token sin permiso — volvé a entrar" };
  if (!res.ok) return { ok: false, error: "el servidor respondió " + res.status };
  try { return await res.json(); }
  catch (e) { return { ok: false, error: "respuesta inesperada del servidor" }; }
}
const cls = c => "c-" + (c || "ungraded");
function el(tag, className, text) {
  const e = document.createElement(tag);
  if (className) e.className = className;
  if (text != null) e.textContent = text;
  return e;
}
// fileToBase64 reads a File as raw base64 (no data: prefix), for uploading an
// artifact to /api/artifact.
function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const fr = new FileReader();
    fr.onload = () => resolve(String(fr.result).split(",")[1] || "");
    fr.onerror = () => reject(fr.error);
    fr.readAsDataURL(file);
  });
}
// toggleMaximizar: pantalla completa real si el navegador la permite y, si no
// (contextos embebidos, políticas de permisos), maximiza dentro de la página.
// El botón nunca debe quedar mudo: si falla, algo tiene que pasar igual.
async function toggleMaximizar(elm, btn) {
  const salir = () => {
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    elm.classList.remove("maximizado");
    if (btn) btn.textContent = "⛶ pantalla completa";
  };
  // el lienzo del grafo necesita saber que cambió el tamaño disponible
  // La transición de pantalla completa nativa tarda más que un cambio de clase,
  // así que se reajusta varias veces y además al evento del navegador.
  const reajustar = () => {
    const r = () => { if (window.__gv && window.__gv.resize) window.__gv.resize(); };
    [60, 180, 400, 700].forEach(ms => setTimeout(r, ms));
    document.addEventListener("fullscreenchange", r, { once: true });
  };
  if (document.fullscreenElement || elm.classList.contains("maximizado")) { salir(); reajustar(); return; }
  try {
    await elm.requestFullscreen();
  } catch (e) {
    elm.classList.add("maximizado"); // plan B: ocupar la ventana sin la API
  }
  if (btn) btn.textContent = "⛶ salir";
  reajustar();
  const onEsc = ev => {
    if (ev.key === "Escape" && elm.classList.contains("maximizado")) { salir(); reajustar(); document.removeEventListener("keydown", onEsc); }
  };
  document.addEventListener("keydown", onEsc);
}

// openPreviewModal muestra el markdown renderizado a ancho de lectura, sin el
// textarea al lado: es la vista de REVISAR (cómo se va a leer la nota de
// verdad), frente a la dividida que es la de ESCRIBIR.
function openPreviewModal(md) {
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card preview-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Vista previa"));
  const body = el("div", "md-render preview-body");
  body.innerHTML = mdToHtml(md || "");
  if (!(md || "").trim()) body.innerHTML = "<p><em>Todavía no escribiste nada.</em></p>";
  card.appendChild(body);
  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

// mdEditor envuelve un <textarea> con la barra de formato y la vista previa
// dividida: el mismo editor visual en toda la app (notas, instrucciones de
// agentes, bloques). Devuelve el contenedor; `.sync()` refresca la vista previa
// después de un cambio hecho por código (no tipeado).
function mdEditor(ta, onChange) {
  const wrap = el("div", "md-editor");
  if (!ta.classList.contains("md")) ta.classList.add("md");
  const previewPane = el("div", "md-render md-preview");
  const syncPrev = () => { if (wrap.classList.contains("split")) previewPane.innerHTML = mdToHtml(ta.value); };
  const touched = () => { if (onChange) onChange(); syncPrev(); };
  ta.addEventListener("input", touched);

  const ins = (before, after, ph) => {
    const s = ta.selectionStart, e = ta.selectionEnd, sel = ta.value.slice(s, e) || ph || "";
    ta.value = ta.value.slice(0, s) + before + sel + after + ta.value.slice(e);
    ta.focus(); ta.selectionStart = s + before.length; ta.selectionEnd = s + before.length + sel.length;
    touched();
  };
  const linePfx = pfx => {
    const s = ta.selectionStart, ls = ta.value.lastIndexOf("\n", s - 1) + 1;
    ta.value = ta.value.slice(0, ls) + pfx + ta.value.slice(ls);
    ta.focus(); ta.selectionStart = ta.selectionEnd = s + pfx.length; touched();
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
    const on = wrap.classList.toggle("split");
    prevBtn.classList.toggle("on", on);
    prevBtn.textContent = on ? "ocultar preview" : "vista previa";
    syncPrev();
  });
  tbRow.appendChild(prevBtn);
  // Ampliar: el markdown renderizado SOLO, a ancho de lectura. El modo dividido
  // sirve para escribir viendo el resultado, pero deja cada mitad muy angosta —
  // para revisar hace falta verlo como se va a leer de verdad.
  const bigBtn = el("button", "md-tb md-big", "⛶"); bigBtn.type = "button";
  bigBtn.title = "Ampliar la vista previa";
  bigBtn.addEventListener("click", ev => { ev.preventDefault(); openPreviewModal(ta.value); });
  tbRow.appendChild(bigBtn);
  wrap.appendChild(tbRow);
  const bodyRow = el("div", "md-body-row");
  bodyRow.appendChild(ta); bodyRow.appendChild(previewPane);
  wrap.appendChild(bodyRow);
  wrap.sync = syncPrev;
  return wrap;
}

// ---- bloques marcados dentro de un .md de instrucciones ----
// Cada bloque insertado queda envuelto en comentarios HTML (invisibles al
// renderizar el markdown). Eso convierte al chip en un INTERRUPTOR: COGO sabe si
// el bloque ya está, puede sacarlo, y puede re-sincronizar su texto si el
// protocolo canónico cambió. El texto libre del usuario, fuera de las marcas, no
// se toca nunca.
const BLOCK_OPEN = id => `<!-- cogo:block:${id} -->`;
const BLOCK_CLOSE = id => `<!-- /cogo:block:${id} -->`;

function blockRegion(text, id) {
  const open = BLOCK_OPEN(id), close = BLOCK_CLOSE(id);
  const s = text.indexOf(open);
  if (s < 0) return null;
  const e = text.indexOf(close, s);
  if (e < 0) return null;
  return { start: s, end: e + close.length, inner: text.slice(s + open.length, e).trim() };
}
function hasBlock(text, id) { return !!blockRegion(text, id); }
function addBlock(text, b) {
  const sep = text.trim() ? "\n\n" : "";
  return text.replace(/\s*$/, "") + sep + BLOCK_OPEN(b.id) + "\n" + b.markdown.trim() + "\n" + BLOCK_CLOSE(b.id) + "\n";
}
function removeBlock(text, id) {
  const r = blockRegion(text, id);
  if (!r) return text;
  return (text.slice(0, r.start).replace(/\s*$/, "") + "\n\n" + text.slice(r.end).replace(/^\s*/, "")).trim() + "\n";
}
function updateBlock(text, b) {
  const r = blockRegion(text, b.id);
  if (!r) return text;
  return text.slice(0, r.start) + BLOCK_OPEN(b.id) + "\n" + b.markdown.trim() + "\n" + BLOCK_CLOSE(b.id) + text.slice(r.end);
}
// staleBlocks: los que están incluidos pero con un texto distinto al canónico
// (porque COGO actualizó el protocolo, o porque los editaste a mano).
function staleBlocks(text, blocks) {
  return blocks.filter(b => { const r = blockRegion(text, b.id); return r && r.inner !== b.markdown.trim(); });
}

// buildBlockPicker arma el catálogo de piezas reutilizables sobre el editor de
// instrucciones: presets (composiciones recomendadas), bloques curados por COGO
// (esenciales y según el caso) y los bloques propios del usuario. Un clic inserta
// la pieza al final del editor — así el texto canónico vive en COGO y no se
// reescribe de memoria en cada AGENTS.md/CLAUDE.md.
async function buildBlockPicker(host, ta, onInsert, mdWrap) {
  host.textContent = "";
  const status = el("div", "agt-bl-status");
  host.appendChild(status);
  setWorking(status, "cargando bloques…");

  // El token no se puede recuperar (se guarda hasheado): lo pega el usuario.
  const q = new URLSearchParams({ project: state.project || "" });
  if (state.blockToken) q.set("token", state.blockToken);
  const data = await api("/api/agent-blocks?" + q).catch(() => null);
  status.textContent = "";
  if (!data) { status.textContent = "no se pudieron cargar los bloques"; return; }

  const blocks = data.blocks || [];
  const byId = {};
  blocks.forEach(b => byId[b.id] = b);
  const refresh = () => buildBlockPicker(host, ta, onInsert, mdWrap);
  const changed = msg => { if (mdWrap && mdWrap.sync) mdWrap.sync(); if (onInsert) onInsert(msg); refresh(); };

  // Agregar/quitar un bloque = un interruptor. Si el texto del bloque ya aparece
  // en el archivo pero SIN marca (pegado a mano o con la plantilla vieja), avisamos
  // antes de duplicar en vez de agregarlo en silencio.
  async function toggle(b) {
    if (hasBlock(ta.value, b.id)) {
      ta.value = removeBlock(ta.value, b.id);
      changed("bloque quitado — acordate de guardar");
      return;
    }
    const heading = (b.markdown.trim().split("\n")[0] || "").replace(/^#+\s*/, "").trim();
    if (heading && ta.value.includes(heading)) {
      const ok = await confirmDialog({
        title: "Ese bloque ya parece estar",
        message: "«" + b.title + "» ya figura en el archivo, pero sin la marca de COGO (lo pegaste a mano o viene de la plantilla vieja). Si lo agrego, va a quedar DUPLICADO.",
        note: heading,
        confirmText: "Agregar igual",
        cancelText: "No agregar",
        danger: true,
      });
      if (!ok) return;
    }
    ta.value = addBlock(ta.value, b);
    ta.scrollTop = ta.scrollHeight;
    changed("bloque agregado — acordate de guardar");
  }

  // --- barra de estado: cuántos hay y si quedaron desactualizados ---
  const included = blocks.filter(b => hasBlock(ta.value, b.id));
  const stale = staleBlocks(ta.value, blocks);
  const head = el("div", "agt-bl-head");
  head.appendChild(el("span", "agt-bl-count", included.length ? included.length + " bloque(s) en el archivo" : "todavía no agregaste bloques"));
  if (stale.length) {
    const upd = el("button", "mini ghost agt-bl-upd", "⟳ actualizar " + stale.length + " desactualizado(s)");
    upd.title = "El texto de estos bloques cambió en COGO (o los editaste a mano):\n" + stale.map(b => "· " + b.title).join("\n");
    upd.addEventListener("click", async () => {
      const ok = await confirmDialog({
        title: "Actualizar bloques",
        message: "Se reemplaza el contenido de " + stale.length + " bloque(s) por el texto canónico de COGO. Si los editaste a mano, esos cambios se pierden.",
        note: stale.map(b => b.title).join(", "),
        confirmText: "Actualizar",
      });
      if (!ok) return;
      stale.forEach(b => ta.value = updateBlock(ta.value, b));
      changed("bloques actualizados — acordate de guardar");
    });
    head.appendChild(upd);
  }
  host.appendChild(head);

  // --- presets: agregan los que falten (nunca duplican) ---
  const pr = el("div", "agt-bl-row");
  pr.appendChild(el("span", "agt-bl-lbl", "Armado rápido"));
  (data.presets || []).forEach(p => {
    const faltan = p.blocks.filter(id => byId[id] && !hasBlock(ta.value, id));
    const c = el("button", "agt-chip agt-chip-preset", p.title);
    c.title = p.desc + "\n\nBloques: " + p.blocks.join(", ") + (faltan.length ? "\n\nFaltan " + faltan.length : "\n\nYa están todos");
    if (!faltan.length) c.classList.add("agt-chip-done");
    c.addEventListener("click", () => {
      if (!faltan.length) return;
      faltan.forEach(id => ta.value = addBlock(ta.value, byId[id]));
      changed(faltan.length + " bloque(s) agregados — acordate de guardar");
    });
    pr.appendChild(c);
  });
  host.appendChild(pr);

  // --- bloques, agrupados; el chip es un interruptor ---
  const groups = [
    ["Esenciales", blocks.filter(b => b.essential)],
    ["Según el caso", blocks.filter(b => !b.essential && !b.custom)],
    ["Míos", blocks.filter(b => b.custom)],
  ];
  groups.forEach(([label, list]) => {
    if (!list.length && label !== "Míos") return;
    const row = el("div", "agt-bl-row");
    row.appendChild(el("span", "agt-bl-lbl", label));
    list.forEach(b => {
      const on = hasBlock(ta.value, b.id);
      const c = el("button", "agt-chip" + (b.essential ? " agt-chip-ess" : "") + (b.custom ? " agt-chip-mine" : "") + (on ? " on" : ""));
      c.appendChild(el("span", "agt-chip-mark", on ? "✓" : "+"));
      c.appendChild(el("span", null, b.title));
      c.title = (b.desc || b.title) + "\n\n" + (on ? "Está en el archivo — clic para QUITARLO." : "Clic para agregarlo al final.");
      c.addEventListener("click", () => toggle(b));
      row.appendChild(c);
      if (b.custom) {
        const ed = el("button", "agt-chip-ed", "✎");
        ed.title = "Ver / editar / borrar este bloque tuyo";
        ed.addEventListener("click", e => { e.stopPropagation(); editCustomBlock(b, refresh); });
        row.appendChild(ed);
      }
    });
    if (label === "Míos") {
      const add = el("button", "agt-chip agt-chip-add", "+ guardar bloque");
      add.title = "Convierte el texto seleccionado del editor (o todo) en una pieza reutilizable, guardada en tu vault.";
      add.addEventListener("click", () => {
        const sel = ta.value.substring(ta.selectionStart, ta.selectionEnd).trim();
        editCustomBlock({ id: "", title: "", desc: "", markdown: sel || "" }, refresh, !sel);
      });
      row.appendChild(add);
      row.appendChild(el("span", "agt-bl-hint", "se guardan en tu vault (.cogo/agent-blocks.json) y sirven para cualquier agente"));
    }
    host.appendChild(row);
  });

  // --- token para el bloque de conexión ---
  const tk = el("div", "agt-bl-row");
  tk.appendChild(el("span", "agt-bl-lbl", "Token"));
  const ti = el("input", "agt-bl-token");
  ti.placeholder = "pegá el token del agente (opcional) — va en el bloque «Conexión»";
  ti.value = state.blockToken || "";
  ti.title = "COGO guarda los tokens hasheados: no puede recuperarlos. Pegá el que te mostró al emitirlo (menú ⋮ → Conexiones MCP).";
  ti.addEventListener("change", () => { state.blockToken = ti.value.trim(); refresh(); });
  tk.appendChild(ti);
  const labels = (data.token_labels || []).filter(Boolean);
  if (labels.length) tk.appendChild(el("span", "agt-bl-hint", "emitidos: " + labels.join(", ")));
  host.appendChild(tk);
}

// editCustomBlock abre el editor de un bloque propio (nuevo o existente): título,
// para qué sirve y el contenido markdown con el editor visual. Desde acá también
// se borra — así un bloque guardado deja de ser una caja negra.
function editCustomBlock(b, onSaved, emptyHint) {
  const isNew = !b.id;
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", isNew ? "Nuevo bloque propio" : "Editar bloque"));
  card.appendChild(el("p", "tk-intro", "Una pieza reutilizable tuya. Se guarda en el vault y aparece en el catálogo de todos los archivos de instrucciones." + (emptyHint ? " Tip: si seleccionás texto en el editor antes de crear el bloque, viene cargado." : "")));

  const tIn = el("input"); tIn.placeholder = "título · ej: Convenciones del repo"; tIn.value = b.title || "";
  card.appendChild(field("Título", tIn));
  const dIn = el("input"); dIn.placeholder = "para qué sirve (se muestra al pasar el mouse)"; dIn.value = b.desc || "";
  card.appendChild(field("Descripción", dIn));
  const ta = el("textarea", "md"); ta.rows = 10; ta.value = b.markdown || "";
  card.appendChild(field("Contenido (markdown)", mdEditor(ta, null)));

  const st = el("span", "lint-status");
  const acts = el("div", "modal-acciones");
  if (!isNew) {
    const del = el("button", "ghost au-danger", "Borrar");
    del.addEventListener("click", async () => {
      if (!(await confirmDialog({ title: "Borrar bloque", message: "Se elimina «" + b.title + "» del catálogo. Los archivos donde ya lo insertaste no cambian.", confirmText: "Borrar", danger: true }))) return;
      await api("/api/agent-blocks?id=" + encodeURIComponent(b.id), { method: "DELETE" }).catch(() => null);
      close(); if (onSaved) onSaved();
    });
    acts.appendChild(del);
  }
  const save = el("button", null, "Guardar");
  save.addEventListener("click", async () => {
    if (!tIn.value.trim() || !ta.value.trim()) { st.textContent = "hace falta título y contenido"; return; }
    save.disabled = true;
    const r = await api("/api/agent-blocks", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: b.id, title: tIn.value.trim(), desc: dIn.value.trim(), markdown: ta.value }) }).catch(() => null);
    save.disabled = false;
    if (r && r.ok) { close(); if (onSaved) onSaved(); }
    else st.textContent = (r && r.error) || "no se pudo guardar";
  });
  acts.appendChild(st); acts.appendChild(save);
  card.appendChild(acts);

  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  function close() { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); }
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
  setTimeout(() => tIn.focus(), 60);
}

// spinner is a small animated "working" indicator for long operations, so the UI
// never looks hung while a model call or a slow request is in flight.
function spinner() {
  const s = el("span", "cogo-spin");
  s.setAttribute("role", "status");
  s.setAttribute("aria-label", "cargando");
  return s;
}
// setWorking fills a status element with the spinner + a message.
function setWorking(elm, text) {
  elm.innerHTML = "";
  elm.appendChild(spinner());
  elm.appendChild(el("span", "cogo-working", " " + text));
}

// downloadArtifact fetches a stored artifact by hash (auth header included) and
// saves it; the store re-verifies the hash on the way out.
async function downloadArtifact(sha) {
  const tok = localStorage.getItem("cogo.token");
  const res = await fetch("/api/artifact?sha=" + encodeURIComponent(sha), { headers: tok ? { Authorization: "Bearer " + tok } : {} }).catch(() => null);
  if (!res || !res.ok) { await confirmDialog({ title: "No se pudo bajar", message: "El artefacto no está disponible (¿fue purgado?).", confirmText: "Cerrar" }); return; }
  const blob = await res.blob();
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob); a.download = "cogo-artifact-" + sha.slice(0, 12);
  a.click(); URL.revokeObjectURL(a.href);
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
  const special = /^(#{1,6}\s|```|>|\s*[-*]\s|\s*\d+\.\s|-{3,}\s*$|\*{3,}\s*$|\s*<!--)/;
  // Fila separadora de una tabla: |---|---| (admite alineación con :)
  const isTableSep = s => /^\s*\|?[\s:|-]+\|[\s:|-]*$/.test(s) && s.includes("-");
  const cells = row => {
    let s = row.trim();
    if (s.startsWith("|")) s = s.slice(1);
    if (s.endsWith("|")) s = s.slice(0, -1);
    return s.split("|").map(c => c.trim());
  };
  while (i < lines.length) {
    const ln = lines[i];
    // Comentarios HTML: invisibles al renderizar (así las marcas de bloque
    // `<!-- cogo:block:… -->` no ensucian la vista previa).
    if (/^\s*<!--/.test(ln)) {
      closeList();
      if (/-->\s*$/.test(ln)) { i++; continue; }
      while (i < lines.length && !/-->/.test(lines[i])) i++;
      i++; continue;
    }
    // Tablas: una fila con | seguida de la fila separadora.
    if (ln.includes("|") && i + 1 < lines.length && isTableSep(lines[i + 1])) {
      closeList();
      const head = cells(ln);
      i += 2;
      let body = "";
      while (i < lines.length && lines[i].includes("|") && !/^\s*$/.test(lines[i])) {
        body += "<tr>" + cells(lines[i]).map(c => "<td>" + mdInline(c) + "</td>").join("") + "</tr>";
        i++;
      }
      html += "<table class=\"md-table\"><thead><tr>" + head.map(c => "<th>" + mdInline(c) + "</th>").join("") +
        "</tr></thead><tbody>" + body + "</tbody></table>";
      continue;
    }
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
    // Un párrafo termina en una línea vacía, en un bloque especial, o cuando
    // arranca una tabla (fila + separadora) — así una tabla pegada a un párrafo
    // no se lo traga, sin partir párrafos que solo contienen un "|".
    const startsTable = j => j + 1 < lines.length && lines[j].includes("|") && isTableSep(lines[j + 1]);
    while (i < lines.length && !/^\s*$/.test(lines[i]) && !special.test(lines[i]) && !startsTable(i)) { para += " " + lines[i]; i++; }
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

// promptDialog: como confirmDialog pero con un campo de texto. Resuelve con el
// valor (trim) o null si se cancela/queda vacío. Mismo estándar visual Escriba.
function promptDialog({ title, message, placeholder = "", value = "", confirmText = "Aceptar", cancelText = "Cancelar" } = {}) {
  return new Promise(resolve => {
    const back = el("div", "modal-back confirm-back");
    const card = el("div", "modal-card confirm-card");
    card.appendChild(el("h2", "modal-tit", title));
    const cuerpo = el("div", "modal-cuerpo");
    if (message) cuerpo.appendChild(el("p", "confirm-msg", message));
    const input = el("input", "prompt-input");
    input.type = "text"; input.placeholder = placeholder; input.value = value;
    cuerpo.appendChild(input);
    card.appendChild(cuerpo);
    const acc = el("div", "modal-acciones");
    const cancel = el("button", "ghost", cancelText);
    const ok = el("button", "", confirmText);
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
    const submit = () => close(input.value.trim() || null);
    const onKey = e => {
      if (e.key === "Escape") close(null);
      else if (e.key === "Enter") { e.preventDefault(); submit(); }
    };
    cancel.addEventListener("click", () => close(null));
    ok.addEventListener("click", submit);
    back.addEventListener("click", e => { if (e.target === back) close(null); });
    document.addEventListener("keydown", onKey);
    setTimeout(() => { input.focus(); input.select(); }, 40);
  });
}

const state = { view: "vault", project: "", showArchived: false, editing: null, llmConfigured: false, scrubEnabled: false, vaultColors: new Set(), graphColors: new Set() };

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
  $("#tokensBtn").addEventListener("click", openTokens);
  $("#trashBtn").addEventListener("click", openTrash);
  $("#auditBtn").addEventListener("click", openAudit);
  $("#leasesBtn").addEventListener("click", openLeases);
  $("#repoBtn").addEventListener("click", () => openRepo(null));
  $("#evrootsBtn").addEventListener("click", openEvidenceRoots);
  $("#exportBtn").addEventListener("click", () => { $("#menu").classList.add("hidden"); window.location.href = "/api/export"; });
  $("#agentsBtn").addEventListener("click", openAgents);
  $("#aboutBtn").addEventListener("click", () => { $("#aboutModal").classList.remove("hidden"); menu.classList.add("hidden"); });
  $("#aboutClose").addEventListener("click", () => $("#aboutModal").classList.add("hidden"));
  $("#aboutModal").addEventListener("click", e => { if (e.target.id === "aboutModal") $("#aboutModal").classList.add("hidden"); });
}

function initTabs() {
  $$(".tab").forEach(b => b.addEventListener("click", () => {
    state.editing = null; // salir a una pestaña siempre cierra el editor abierto
    state.view = b.dataset.view;
    $$(".tab").forEach(x => x.classList.toggle("active", x === b));
    location.hash = state.view; // link compartible a la vista
    render();
  }));
  applyHash();
  window.addEventListener("hashchange", applyHash);
}

// applyHash abre la vista (o el panel) que pide el fragmento de la URL, para que
// un link como .../#guard o .../#leases lleve directo ahí.
const HASH_PANELS = { repo: () => openRepo(null), leases: openLeases, audit: openAudit, tokens: openTokens, trash: openTrash, instrucciones: openAgents, evroots: openEvidenceRoots };
function applyHash() {
  const h = (location.hash || "").replace(/^#/, "");
  if (!h) return;
  const tab = $$(".tab").find(t => t.dataset.view === h);
  if (tab) {
    state.editing = null;
    state.view = h;
    $$(".tab").forEach(x => x.classList.toggle("active", x === tab));
    render();
    return;
  }
  const panel = HASH_PANELS[h];
  if (panel) panel();
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
  const sv = $("#menuSaved");
  if (sv) sv.textContent = "💚 ≈ " + fmtTokens(state.savedTokens) + " tokens ahorrados";
}

async function loadConfig() {
  const c = await api("/api/config");
  state.llmConfigured = !!c.llm_configured;
  state.scrubEnabled = !!c.scrub_enabled;
  state.tokens = c.tokens || 0;
  state.savedTokens = c.saved_tokens || 0;
  updateTokenBadge();
  $("#aboutVersion").textContent = c.version;
  $("#aboutCount").textContent = c.count;
  $("#aboutStorage").textContent = c.artifact_backend === "r2" ? "Cloudflare R2 (por contenido)" : "disco (volumen del vault)";
  const sel = $("#projsel");
  (c.projects || []).forEach(p => { const o = el("option", null, p); o.value = p; sel.appendChild(o); });
  sel.addEventListener("change", () => { state.editing = null; state.project = sel.value; render(); });
}

// ---------- shared ----------
function matchesProject(n) { return !state.project || n.project === state.project; }

// colorFilterBar: chips clickeables por color (verde/amarillo/rojo/s-grado) que
// filtran la vista. `active` es un Set de colores seleccionados (vacío = todos).
// Chip tintado por su color; borde más oscuro cuando está seleccionado.
function colorFilterBar(notes, active, onToggle) {
  const counts = { green: 0, yellow: 0, red: 0, ungraded: 0 };
  notes.forEach(n => counts[n.color] = (counts[n.color] || 0) + 1);
  const wrap = el("div", "cfilter");
  [["green", "verde"], ["yellow", "amarillo"], ["red", "rojo"], ["ungraded", "s/grado"]].forEach(([c, label]) => {
    if (!counts[c]) return;
    const chip = el("button", "cf " + cls(c) + (active.has(c) ? " on" : ""));
    chip.appendChild(el("span", "dot"));
    chip.appendChild(el("span", null, counts[c] + " " + label));
    chip.title = "Filtrar por color (clic para alternar)";
    chip.addEventListener("click", () => {
      if (active.has(c)) active.delete(c); else active.add(c);
      chip.classList.toggle("on"); // in-place (graph); un render completo lo reconstruye igual (vault)
      onToggle();
    });
    wrap.appendChild(chip);
  });
  return wrap;
}

// colorVisible: aplica un Set de colores a una lista (vacío = todos).
function colorVisible(notes, active) {
  return active.size === 0 ? notes : notes.filter(n => active.has(n.color));
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
  const host = $("#main");
  // Fresh container per render: clearing #main DETACHES the previous view, so a
  // slow async view (one still awaiting an api() call) that resolves after you've
  // switched tabs appends to an off-DOM node and never double-draws. The graph's
  // RAF loop self-stops via !canvas.isConnected.
  host.innerHTML = "";
  const main = el("div", "view-root");
  host.appendChild(main);
  if (state.editing) { renderEditor(main); return; }
  ({ vault: renderVault, fresh: renderFresh, pack: renderAgents, graph: renderGraph, lint: renderLint, guard: renderGuard, xray: renderVeracidad }[state.view])(main);
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

// El Vault es un ÍNDICE, no un volcado: se busca, se filtra y se pagina del lado
// del servidor. Una lista plana de todo deja de servir mucho antes de lo que uno
// cree — con cien notas ya no encontrás nada. Lo único que NO cambia es el orden
// por defecto: primero lo que necesita atención, que es la opinión de COGO.
async function renderVault(main) {
  const st = window.__vault || (window.__vault = { q: "", author: "", sort: "atencion", limit: 50 });
  st.offset = 0;

  viewHead(main, "Suite Escriba · Memoria", "Vault", "Todo lo que sabés del proyecto, con un color de confianza que COGO computa solo: verde confiá, amarillo ojo, rojo no.");

  // --- barra 1: acción, búsqueda, archivadas ---
  const bar1 = el("div", "viewbar");
  const addBtn = el("button", "mini", "+ Nueva nota");
  addBtn.addEventListener("click", () => openEditor(null));
  const buscar = el("input", "vault-q");
  buscar.type = "search";
  buscar.placeholder = "buscar en tus notas…";
  buscar.value = st.q;
  const arch = el("button", "pilltog" + (state.showArchived ? " on" : ""));
  arch.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="4" rx="1"/><path d="M5 7v13a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V7"/><path d="M10 12h4"/></svg><span>archivadas</span>';
  arch.title = state.showArchived ? "Ocultar archivadas" : "Mostrar archivadas";
  arch.addEventListener("click", () => { state.showArchived = !state.showArchived; render(); });
  bar1.append(addBtn, buscar, el("span", "vb-spacer"), arch);
  main.appendChild(bar1);

  // --- barra 2: filtros y orden (se completa con las facetas del servidor) ---
  const bar2 = el("div", "viewbar vault-filtros");
  main.appendChild(bar2);

  const cuenta = el("div", "vault-cuenta");
  main.appendChild(cuenta);
  const list = el("div", "note-list");
  main.appendChild(list);
  const masWrap = el("div", "vault-mas");
  main.appendChild(masWrap);

  function url() {
    const p = new URLSearchParams();
    if (st.q) p.set("q", st.q);
    if (state.project) p.set("project", state.project);
    if (st.author) p.set("author", st.author);
    if (state.vaultColors && state.vaultColors.size) p.set("color", [...state.vaultColors].join(","));
    if (st.sort !== "atencion") p.set("sort", st.sort);
    p.set("limit", st.limit); p.set("offset", st.offset);
    if (state.showArchived) p.set("archived", "1");
    return "/api/notes?" + p;
  }

  // Los filtros se dibujan una vez por carga: si se reconstruyeran con cada
  // tecleo, el foco saltaría del campo de búsqueda.
  function pintarFiltros(facets) {
    bar2.textContent = "";
    // color (con su conteo real del servidor, no del pedazo que se trajo)
    const colores = el("div", "cf-bar");
    (facets.colors || []).forEach(c => {
      const on = state.vaultColors && state.vaultColors.has(c.name);
      const chip = el("button", "cf " + cls(c.name) + (on ? " on" : ""));
      chip.appendChild(el("span", "dot"));
      chip.appendChild(el("span", null, c.count + " " + COLORWORD_CORTO(c.name)));
      chip.addEventListener("click", () => {
        state.vaultColors = state.vaultColors || new Set();
        state.vaultColors.has(c.name) ? state.vaultColors.delete(c.name) : state.vaultColors.add(c.name);
        recargar(true);
      });
      colores.appendChild(chip);
    });
    bar2.appendChild(colores);

    const sel = (etq, valor, opciones, onch) => {
      const s = el("select", "vault-sel");
      s.appendChild(Object.assign(el("option", null, etq), { value: "" }));
      opciones.forEach(o => s.appendChild(Object.assign(el("option", null, o.label), { value: o.value })));
      s.value = valor || "";
      s.addEventListener("change", () => onch(s.value));
      return s;
    };
    bar2.appendChild(sel("proyecto: todos", state.project,
      (facets.projects || []).map(p => ({ value: p.name, label: p.name + " (" + p.count + ")" })),
      v => { state.project = v; const ps = $("#projsel"); if (ps) ps.value = v; recargar(true); }));
    bar2.appendChild(sel("agente: todos", st.author,
      (facets.authors || []).map(a => ({ value: a.name, label: callerKind(a.name)[0] + " (" + a.count + ")" })),
      v => { st.author = v; recargar(true); }));

    const orden = el("select", "vault-sel");
    [["atencion", "orden: atención"], ["reciente", "orden: más reciente"], ["antigua", "orden: más antigua"]]
      .forEach(([v, l]) => orden.appendChild(Object.assign(el("option", null, l), { value: v })));
    orden.value = st.sort;
    orden.addEventListener("change", () => { st.sort = orden.value; recargar(true); });
    bar2.appendChild(orden);
  }

  async function recargar(reset) {
    if (reset) { st.offset = 0; list.textContent = ""; }
    setWorking(cuenta, "buscando…");
    const r = await apiOrError(url());
    if (r.ok === false) { cuenta.textContent = "⚠ " + r.error; return; }
    if (reset || !bar2.children.length) pintarFiltros(r.facets || {});
    (r.notes || []).forEach(n => list.appendChild(notaCard(n)));
    const hasta = Math.min(r.offset + (r.notes || []).length, r.total);
    cuenta.textContent = r.total === 0 ? "" : "mostrando " + hasta + " de " + r.total;
    masWrap.textContent = "";
    if (hasta < r.total) {
      const mas = el("button", "mini ghost", "cargar más");
      mas.addEventListener("click", () => { st.offset = hasta; recargar(false); });
      masWrap.appendChild(mas);
    }
    if (!r.total) {
      list.appendChild(el("div", "empty", st.q || st.author || state.project || (state.vaultColors && state.vaultColors.size)
        ? "Ninguna nota coincide con esos filtros." : "Todavía no hay notas."));
    }
  }

  // Buscar mientras se escribe, sin disparar una consulta por tecla.
  let t = 0;
  buscar.addEventListener("input", () => {
    clearTimeout(t);
    t = setTimeout(() => { st.q = buscar.value.trim(); recargar(true); }, 260);
  });

  // Vault vacío de verdad (sin filtros): la bienvenida en vez de una lista vacía.
  const primera = await apiOrError("/api/notes?limit=1" + (state.showArchived ? "&archived=1" : ""));
  if (primera.ok !== false && !primera.total && !st.q && !state.project && !st.author) {
    main.textContent = ""; renderWelcome(main); return;
  }
  await recargar(true);
}

// COLORWORD_CORTO: la palabra que va en el chip de color.
function COLORWORD_CORTO(c) {
  return { green: "verde", yellow: "amarillo", red: "rojo", ungraded: "s/grado" }[c] || c;
}

// notaCard: una nota del vault como tarjeta, con su color, su autor y CUÁNDO se
// verificó — el dato que faltaba para poder triar sin abrir cada una.
function notaCard(n) {
  const card = el("div", "note-card " + cls(n.color) + (n.state ? " archived" : ""));
  card.addEventListener("click", () => openEditor(n.id));
  card.appendChild(el("span", "dot"));
  const body = el("div", "nc-body");
  const head = el("div", "nc-head");
  head.appendChild(el("span", "nc-id", n.id));
  head.appendChild(el("span", "nc-type", n.type + (n.project ? " · " + n.project : "")));
  if (n.author) { const a = callerKind(n.author); head.appendChild(el("span", "nc-author", "· " + a[0])).title = "capturada por " + n.author; }
  if (n.state) head.appendChild(el("span", "nc-badge", stateLabel(n.state)));
  if (n.verified) {
    const f = el("span", "nc-fecha", "verificada " + haceCuanto(n.verified));
    f.title = "Verificada el " + n.verified + (n.created ? "\nCreada el " + n.created.slice(0, 10) : "");
    head.appendChild(f);
  }
  if (n.stale_at) {
    const f = freshnessLabel(n.stale_at);
    const stl = el("span", "nc-stale " + f.cls, f.text);
    stl.title = "Fresca hasta " + n.stale_at + " · después conviene revalidar (pestaña Frescura).";
    head.appendChild(stl);
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
  return card;
}

// haceCuanto: fecha en relativo ("hace 3 días"), que es como uno tría.
function haceCuanto(iso) {
  const d = Math.round((new Date() - new Date(iso.length > 10 ? iso : iso + "T00:00:00")) / 86400000);
  if (d <= 0) return "hoy";
  if (d === 1) return "ayer";
  if (d < 30) return "hace " + d + " días";
  if (d < 365) return "hace " + Math.round(d / 30) + " meses";
  return "hace " + Math.round(d / 365) + " años";
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

// freshEmptyState: cuando no hay nada vencido ni por vencer — un brote fresco
// con un check, en vez de una línea de texto sola. `total` = notas con vencimiento.
function freshEmptyState(total) {
  const w = el("div", "fresh-zen");
  w.innerHTML = `
    <svg viewBox="0 0 200 168" width="184" height="154" aria-hidden="true" class="fresh-art">
      <circle cx="100" cy="88" r="60" fill="var(--ok)" opacity="0.07"/>
      <circle cx="100" cy="88" r="43" fill="var(--ok)" opacity="0.09"/>
      <path d="M100 126 C100 106 100 96 100 80" stroke="var(--ok)" stroke-width="3.5" stroke-linecap="round"/>
      <path d="M100 96 C80 96 67 83 67 68 C87 68 100 80 100 96 Z" fill="var(--ok)" opacity="0.9"/>
      <path d="M100 86 C120 86 133 73 133 58 C113 58 100 70 100 86 Z" fill="var(--ok)" opacity="0.62"/>
      <path d="M72 126 h56" stroke="var(--ok)" stroke-width="3" stroke-linecap="round" opacity="0.45"/>
      <circle cx="138" cy="120" r="15" fill="var(--ok)"/>
      <path d="M131 120 l4.5 4.5 l8.5 -9" stroke="#fff" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
    </svg>
    <div class="fresh-zen-h">Todo fresco</div>
    <div class="fresh-zen-sub">Nada vencido ni por vencer en los próximos 30 días.${total ? " Tus " + total + " nota(s) con vencimiento están al día." : ""}</div>`;
  return w;
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
  if (!rows.length) { main.appendChild(freshEmptyState(notes.length)); return; }

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

// ---------- agentes: gestor de instrucciones (.md) ----------
// Editá/guardá/versioná los archivos que un agente lee al arrancar (AGENTS.md,
// CLAUDE.md, GEMINI.md, copilot-instructions.md). Se guardan en el vault
// (.cogo/agents/), con historial. Podés meterles la plantilla del protocolo COGO
// y un "pack" de contexto coloreado del vault.
async function renderAgents(main) {
  viewHead(main, "Suite Escriba · Memoria", "Agentes",
    "Los archivos de instrucciones que tus agentes (Claude Code, Cursor, Copilot, Gemini…) leen al arrancar. Editálos, versionálos, y meteles la plantilla del protocolo o contexto coloreado del vault. Se guardan en tu vault.");

  const data = await api("/api/agent-docs");
  const savedSet = new Set((data.docs || []).map(d => d.name));
  const names = Array.from(new Set([...(data.docs || []).map(d => d.name), ...(data.known || [])]));
  // Un archivo recién creado (custom) todavía no está guardado: lo mostramos como
  // tab pendiente para poder editarlo antes de guardar.
  if (window.__agentDoc && !names.includes(window.__agentDoc)) names.push(window.__agentDoc);
  let current = window.__agentDoc && names.includes(window.__agentDoc) ? window.__agentDoc : (names[0] || "AGENTS.md");

  // selector de archivo (chips)
  const tabs = el("div", "agt-tabs");
  names.forEach(n => {
    const chip = el("button", "agt-tab" + (n === current ? " on" : "") + (savedSet.has(n) ? " saved" : ""));
    chip.appendChild(el("span", null, n));
    chip.title = savedSet.has(n) ? "Guardado en el vault" : "Todavía sin guardar";
    chip.addEventListener("click", () => { window.__agentDoc = n; render(); });
    tabs.appendChild(chip);
  });
  const addTab = el("button", "agt-tab agt-new", "+ nuevo");
  addTab.addEventListener("click", async () => {
    const name = await promptDialog({
      title: "Nuevo archivo de instrucciones",
      message: "Nombre del archivo — tiene que terminar en .md (ej: copilot-instructions.md).",
      placeholder: "mis-instrucciones.md",
      confirmText: "Crear",
    });
    if (name) { window.__agentDoc = name; render(); }
  });
  tabs.appendChild(addTab);
  main.appendChild(tabs);

  const doc = await api("/api/agent-docs?name=" + encodeURIComponent(current));
  const ta = el("textarea", "agt-editor mono md"); ta.rows = 18; ta.value = doc.content || "";
  ta.placeholder = "Elegí bloques arriba (o escribí a mano) para armar las instrucciones de " + current + ".";
  // Mismo editor visual que las notas: barra de formato + vista previa dividida.
  const mdWrap = mdEditor(ta, null);

  // --- armador de bloques: piezas reutilizables que COGO recomienda ---
  // Va ARRIBA del editor: primero elegís las piezas, abajo ves el archivo que se arma.
  const bb = el("div", "agt-blocks");
  main.appendChild(bb);
  buildBlockPicker(bb, ta, msg => st.textContent = msg || "bloque insertado — acordate de guardar", mdWrap);
  main.appendChild(mdWrap);

  const bar = el("div", "agt-bar");
  const save = el("button", "mini", "Guardar");
  const tpl = el("button", "mini ghost", "Plantilla COGO");
  const ins = el("button", "mini ghost", "Insertar contexto…");
  const dl = el("button", "mini ghost", "Descargar");
  const st = el("span", "lint-status");
  bar.append(save, tpl, ins, dl, st);
  main.appendChild(bar);

  const insRow = el("div", "agt-ins hidden");
  const iq = el("input"); iq.placeholder = "tema del contexto, ej: redis (vacío = todo)";
  const igo = el("button", "mini", "insertar pack");
  insRow.append(iq, igo);
  main.appendChild(insRow);

  save.addEventListener("click", async () => {
    save.disabled = true;
    const r = await api("/api/agent-docs", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: current, content: ta.value }) }).catch(() => null);
    save.disabled = false;
    if (r && r.ok) { st.textContent = "guardado ✓"; window.__agentDoc = current; render(); }
    else st.textContent = (r && r.error) || "no se pudo guardar";
  });
  tpl.addEventListener("click", async () => {
    if (ta.value.trim() && !(await confirmDialog({ title: "Cargar plantilla", message: "Reemplaza el contenido actual con la plantilla del protocolo COGO para " + current + ".", confirmText: "Cargar" }))) return;
    const tool = /claude/i.test(current) ? "claude" : "";
    const r = await api("/api/agents-md" + (tool ? "?tool=" + tool : "")).catch(() => null);
    if (r) { ta.value = r.markdown; mdWrap.sync(); buildBlockPicker(bb, ta, msg => st.textContent = msg, mdWrap); }
  });
  ins.addEventListener("click", () => { insRow.classList.toggle("hidden"); if (!insRow.classList.contains("hidden")) iq.focus(); });
  igo.addEventListener("click", async () => {
    igo.disabled = true;
    const p = await api("/api/pack?" + new URLSearchParams({ query: iq.value, project: state.project, budget: "0" })).catch(() => null);
    igo.disabled = false;
    if (p) { ta.value = ta.value.replace(/\s*$/, "") + "\n\n" + p.markdown + "\n"; mdWrap.sync(); insRow.classList.add("hidden"); st.textContent = "contexto insertado — acordate de guardar"; }
  });
  dl.addEventListener("click", () => {
    const blob = new Blob([ta.value], { type: "text/markdown" });
    const a = document.createElement("a"); a.href = URL.createObjectURL(blob); a.download = current; a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  });

  const hist = doc.history || [];
  if (hist.length) {
    const hb = el("div", "agt-hist");
    hb.appendChild(el("div", "agt-hist-lbl", "Historial — " + hist.length + " versión(es) guardada(s)"));
    hist.slice().reverse().forEach(v => {
      const row = el("div", "agt-hist-row");
      row.appendChild(el("span", "agt-hist-t", (v.time || "").replace("T", " ").replace("Z", "")));
      const rb = el("button", "nc-act", "cargar esta versión");
      rb.addEventListener("click", () => { ta.value = v.content; st.textContent = "versión cargada — guardá para fijarla"; });
      row.appendChild(rb);
      hb.appendChild(row);
    });
    main.appendChild(hb);
  }
}

// ---------- graph (motor Canvas: graph.js) ----------
// La vista Grafo tiene dos mapas: el de tus NOTAS y el del REPOSITORIO pintado
// con la memoria. El segundo responde algo que ningún explorador de archivos
// puede: dónde tenés conocimiento verificado y dónde estás a ciegas.
function graphModeBar(main) {
  const row = el("div", "viewbar");
  const seg = el("div", "seg");
  [["notes", "Notas"], ["repo", "Repositorio"]].forEach(([k, lab]) => {
    const b = el("button", "seg-btn" + ((state.graphMode || "notes") === k ? " on" : ""), lab);
    b.addEventListener("click", () => { state.graphMode = k; render(); });
    seg.appendChild(b);
  });
  row.appendChild(seg);
  main.appendChild(row);
}

// icono: carpeta o documento, en trazo neutro. El color lo lleva SIEMPRE el
// punto de confianza que va al lado; el icono solo dice de qué tipo es.
function icono(k) {
  const NS = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24"); svg.setAttribute("class", "repo-svg");
  svg.setAttribute("fill", "none"); svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "1.8"); svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  const p = document.createElementNS(NS, "path");
  p.setAttribute("d", k.type === "dir"
    ? "M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"
    : "M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z M14 3v5h5");
  svg.appendChild(p);
  if (k.type === "dir") svg.classList.add("dir");
  return svg;
}

// renderRepoMap: el repositorio con la memoria encima. Dos lecturas de lo mismo,
// porque sirven para cosas distintas: el GRAFO muestra la forma del proyecto y
// dónde están las islas de conocimiento; el ÁRBOL es el navegador de siempre,
// para cuando ya sabés qué archivo buscás. Al costado, el panel muestra lo que
// hay adentro de lo que clickeaste (el contenido del archivo, o el listado de la
// carpeta) y desde ahí citás una línea como evidencia.
async function renderRepoMap(main) {
  viewHead(main, "Suite Escriba · Memoria", "Mapa del repositorio",
    "El repo con tu memoria encima: los archivos citados toman el color de sus notas y el resto queda neutro. Clic en cualquiera para ver qué tiene adentro.");
  graphModeBar(main);

  const qs = new URLSearchParams(location.search);
  const st = {
    repo: "", ref: "",
    // la vista se recuerda entre visitas y puede venir en el link (?vista=arbol)
    vista: qs.get("vista") || localStorage.getItem("cogo.repo.vista") || "grafo",
    data: null, sel: null,
  };

  // --- barra: repo, rama, vista, acciones ---
  const bar = el("div", "viewbar");
  const ri = el("input", "repo-in"); ri.placeholder = "owner/repositorio";
  ri.value = qs.get("repo") || localStorage.getItem("cogo.repo") || "";
  const rf = el("input", "repo-ref"); rf.placeholder = "rama (opcional)";
  rf.value = qs.get("ref") || localStorage.getItem("cogo.repo.ref") || "";
  const go = el("button", "mini", "mapear");
  const seg = el("div", "seg");
  const bg = el("button", "seg-btn", "Grafo"), bt = el("button", "seg-btn", "Árbol");
  seg.append(bg, bt);
  const reset = el("button", "mini ghost", "recentrar");
  const fsb = el("button", "mini ghost", "⛶ pantalla completa");
  const status = el("span", "lint-status");
  bar.append(ri, rf, go, el("span", "vb-spacer"), seg, reset, fsb, status);

  const summary = el("div", "repo-sum");

  // --- cuerpo: lienzo/árbol a la izquierda, panel de contenido a la derecha ---
  // El shell incluye la barra y el resumen: es lo que se maximiza, así en
  // pantalla completa seguís teniendo los controles (igual que el grafo de notas).
  const shell = el("div", "repo-shell");
  const view = el("div", "repo-view");
  const left = el("div", "repo-left");
  const panel = el("div", "repo-panel");
  view.append(left, panel);
  shell.append(bar, summary, view);
  main.appendChild(shell);

  fsb.addEventListener("click", () => toggleMaximizar(shell, fsb));
  document.addEventListener("fullscreenchange", () => {
    fsb.textContent = document.fullscreenElement ? "⛶ salir" : "⛶ pantalla completa";
  });

  function setVista(v) {
    st.vista = v; localStorage.setItem("cogo.repo.vista", v);
    bg.classList.toggle("on", v === "grafo"); bt.classList.toggle("on", v === "arbol");
    reset.style.display = v === "grafo" ? "" : "none";
    draw();
  }
  bg.addEventListener("click", () => setVista("grafo"));
  bt.addEventListener("click", () => setVista("arbol"));
  reset.addEventListener("click", () => { if (window.__gv) window.__gv.resetView(); });

  // ---- panel lateral: qué hay adentro de lo que clickeaste ----
  function panelVacio() {
    panel.textContent = "";
    panel.appendChild(el("div", "repo-ph", "Clic en un archivo o una carpeta para ver qué tiene adentro."));
  }
  // Aviso de foco: si el resto del mapa se atenúa, tiene que estar claro POR QUÉ
  // y cómo volver — si no, parece que el grafo se rompió.
  function marcarFoco(id) {
    const vieja = summary.querySelector(".repo-foco");
    if (vieja) vieja.remove();
    if (!id) return;
    const chip = el("span", "repo-foco");
    chip.appendChild(el("span", null, "enfocando " + (id.split("/").pop() || id)));
    const ver = el("button", "mini ghost", "ver todo");
    ver.addEventListener("click", () => {
      st.sel = null;
      if (window.__gv && window.__gv.setSelected) window.__gv.setSelected(null);
      marcarFoco(null);
    });
    chip.appendChild(ver);
    summary.appendChild(chip);
  }

  async function verNodo(id) {
    st.sel = id;
    if (window.__gv && window.__gv.setSelected) window.__gv.setSelected(id); // enfocar su rama
    marcarFoco(id);
    const n = (st.data.nodes || []).find(x => x.id === id);
    if (!n) return;
    panel.textContent = "";
    const head = el("div", "repo-phead");
    head.appendChild(el("span", "dot " + cls(n.color)));
    head.appendChild(el("span", "repo-ptitle", n.id === st.data.repo.split("/")[1] ? n.id : (n.id.split("/").pop() || n.id)));
    panel.appendChild(head);
    panel.appendChild(el("div", "repo-ppath", n.id));

    if (n.notes && n.notes.length) {
      const nb = el("div", "repo-pnotes");
      nb.appendChild(el("div", "repo-plbl", n.notes.length + " nota(s) citan este archivo"));
      n.notes.forEach(x => {
        const r = el("div", "repo-pnote " + cls(x.color));
        r.appendChild(el("span", "dot"));
        r.appendChild(el("span", null, x.id));
        r.addEventListener("click", () => openNoteModal(x.id));
        nb.appendChild(r);
      });
      panel.appendChild(nb);
    }

    if (n.type === "dir") {
      const kids = (st.data.nodes || []).filter(x => parentOf(x.id) === n.id && x.id !== n.id);
      panel.appendChild(el("div", "repo-plbl", (n.files || 0) + " archivo(s) · " + (n.blind || 0) + " sin memoria"));
      const ul = el("div", "repo-plist");
      kids.forEach(k => ul.appendChild(filaEntrada(k)));
      if (!kids.length) ul.appendChild(el("div", "tk-empty", "Sin elementos para mostrar."));
      panel.appendChild(ul);
      return;
    }

    // archivo: traemos el contenido y lo mostramos con números de línea
    const load = el("div", "repo-plbl"); panel.appendChild(load);
    setWorking(load, "abriendo el archivo…");
    const r = await apiOrError("/api/github?" + new URLSearchParams({ repo: st.repo, ref: st.ref, path: n.id, file: "1" }));
    load.textContent = "";
    if (!r.ok) {
      load.textContent = "⚠ " + r.error;
      if (r.html_url) { const a = el("a", "link", " abrir en GitHub"); a.href = r.html_url; a.target = "_blank"; load.appendChild(a); }
      return;
    }
    load.textContent = (r.lines || []).length + " líneas · clic en una para citarla como evidencia";
    const pre = el("div", "repo-code");
    (r.lines || []).forEach((ln, i) => {
      const row = el("div", "repo-ln");
      row.appendChild(el("span", "repo-num", String(i + 1)));
      row.appendChild(el("span", "repo-txt", ln || " "));
      row.addEventListener("click", () => citar(n.id, i + 1));
      pre.appendChild(row);
    });
    panel.appendChild(pre);
  }
  function filaEntrada(k) {
    const row = el("div", "repo-row");
    row.appendChild(el("span", "dot " + cls(k.color)));
    row.appendChild(icono(k));
    row.appendChild(el("span", "repo-name" + (k.type === "dir" ? " dir" : ""), k.id.split("/").pop()));
    if (k.type === "dir" && k.files) row.appendChild(el("span", "repo-cnt", k.blind + "/" + k.files + " sin memoria"));
    row.addEventListener("click", () => verNodo(k.id));
    return row;
  }
  async function citar(path, line) {
    const ref = "github://" + st.repo + (st.ref ? "@" + st.ref : "") + "/" + path + ":" + line;
    await navigator.clipboard.writeText(ref).catch(() => {});
    await confirmDialog({
      title: "Cita copiada", note: ref,
      message: "Pegala como evidencia en una nota. Si citás una rama, COGO te avisa cuando ese archivo cambie; con un commit fijo la cita es inmutable.",
      confirmText: "Listo", cancelText: "Cerrar",
    });
  }
  const parentOf = p => { const i = p.lastIndexOf("/"); return i < 0 ? st.data.repo.split("/")[1] : p.slice(0, i); };

  // ---- dibujo ----
  function draw() {
    left.textContent = "";
    if (!st.data) return;
    if (st.vista === "grafo") {
      const wrap = el("div", "graph-wrap");
      left.appendChild(wrap);
      const gv = CogoGraph.mount(wrap, { nodes: st.data.nodes, edges: st.data.edges },
        { mode: "2d", onSelect: id => verNodo(id) });
      window.__gv = gv;
      gv.setColorFilter(state.graphColors);
      return;
    }
    // árbol: navegador clásico, carpeta por carpeta
    const tree = el("div", "repo-tree");
    const raiz = st.data.repo.split("/")[1];
    const render = (dir, depth) => {
      const kids = (st.data.nodes || []).filter(x => parentOf(x.id) === dir && x.id !== dir);
      kids.forEach(k => {
        const row = filaEntrada(k);
        row.style.paddingLeft = (10 + depth * 16) + "px";
        if (st.sel === k.id) row.classList.add("on");
        tree.appendChild(row);
        if (k.type === "dir" && (window.__repoOpen || {})[k.id]) render(k.id, depth + 1);
      });
    };
    // en el árbol, clic en carpeta = expandir/colapsar
    tree.addEventListener("click", e => {
      const row = e.target.closest(".repo-row"); if (!row) return;
    }, true);
    render(raiz, 0);
    left.appendChild(tree);
  }

  async function load() {
    st.repo = ri.value.trim(); st.ref = rf.value.trim();
    if (!st.repo) { status.textContent = "escribí un repo como owner/nombre"; return; }
    localStorage.setItem("cogo.repo", st.repo); localStorage.setItem("cogo.repo.ref", st.ref);
    setWorking(status, "leyendo el repositorio…"); summary.textContent = ""; left.textContent = ""; panelVacio();
    const r = await apiOrError("/api/github/map?" + new URLSearchParams({ repo: st.repo, ref: st.ref }));
    status.textContent = "";
    if (!r.ok) { status.textContent = "⚠ " + r.error; return; }
    st.data = r;
    // todas las carpetas arrancan expandidas en el árbol
    window.__repoOpen = {};
    (r.nodes || []).forEach(n => { if (n.type === "dir") window.__repoOpen[n.id] = true; });
    const files = (r.nodes || []).filter(n => n.type === "file");
    const conMem = files.filter(n => n.notes && n.notes.length).length;
    const blind = (r.nodes || []).reduce((a, n) => a + (n.blind || 0), 0);
    const total = r.total_files || (r.nodes || []).reduce((a, n) => a + (n.files || 0), 0);
    summary.textContent = "";
    summary.appendChild(el("b", null, conMem + " archivo(s) con memoria"));
    summary.appendChild(el("span", null, " · " + blind + " de " + total + " sin ninguna nota que los cite"));
    if (r.dense) summary.appendChild(el("span", "repo-trunc", " · repo grande: el grafo muestra solo las carpetas y lo citado"));
    if (r.truncated) summary.appendChild(el("span", "repo-trunc", " · el árbol de GitHub vino recortado"));
    setVista(st.vista);
  }
  go.addEventListener("click", load);
  ri.addEventListener("keydown", e => { if (e.key === "Enter") load(); });
  panelVacio();
  if (ri.value.trim()) load(); else status.textContent = "escribí un repositorio para mapearlo";
}


// openFileNotes: qué sabe COGO sobre un archivo del repo, y el atajo para ir a verlo.
function openFileNotes(node, repo, ref) {
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", node.id.split("/").pop()));
  card.appendChild(el("p", "tk-intro", node.id));
  const list = el("div", "tk-list");
  (node.notes || []).forEach(n => {
    const row = el("div", "tk-row " + cls(n.color));
    row.appendChild(el("span", "dot"));
    const inf = el("div", "tk-info");
    inf.appendChild(el("div", "tk-label", n.id));
    inf.appendChild(el("div", "tk-meta", "clic para abrir la nota"));
    row.appendChild(inf);
    row.style.cursor = "pointer";
    row.addEventListener("click", () => { close(); openNoteModal(n.id); });
    list.appendChild(row);
  });
  if (!(node.notes || []).length) list.appendChild(el("div", "tk-empty", "Ninguna nota cita este archivo todavía."));
  card.appendChild(list);
  const acts = el("div", "modal-acciones");
  const see = el("button", "ghost", "ver el archivo");
  see.addEventListener("click", () => { close(); openRepo(null); });
  acts.appendChild(see);
  card.appendChild(acts);
  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  function close() { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); }
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

async function renderGraph(main) {
  // Un link con ?repo=owner/name pide el mapa del repositorio, no el de notas.
  if (!state.graphMode && new URLSearchParams(location.search).get("repo")) state.graphMode = "repo";
  if ((state.graphMode || "notes") === "repo") return renderRepoMap(main);
  const g = await api("/api/graph");
  // Un vault vacío puede devolver nodes/edges nulos (slices vacíos de Go); guardá.
  const gNodes = (g && g.nodes) || [], gEdges = (g && g.edges) || [];
  if (!gNodes.length) { main.appendChild(el("div", "empty", "Sin notas para graficar.")); return; }
  const nodes = gNodes.filter(matchesProject);
  const keep = new Set(nodes.map(n => n.id));
  const edges = gEdges.filter(e => keep.has(e.from) && keep.has(e.to));
  if (!nodes.length) { main.appendChild(el("div", "empty", "Sin notas para este proyecto.")); return; }

  viewHead(main, "Suite Escriba · Memoria", "Grafo", "Cómo se relacionan tus notas, pintadas por confianza. Mirálo en 2D o entrá a la constelación 3D.");
  graphModeBar(main);
  const view = el("div", "graph-view");
  const bar = el("div", "viewbar graph-bar");
  // Filtro por color EN VIVO: al togglear, atenúa las esferas que no son de ese
  // color (gv.setColorFilter), sin remontar el grafo.
  bar.appendChild(colorFilterBar(nodes, state.graphColors, () => { if (window.__gv) window.__gv.setColorFilter(state.graphColors); }));
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

  fs.addEventListener("click", () => toggleMaximizar(view, fs));
  const onFs = () => { fs.textContent = document.fullscreenElement ? "⛶ salir" : "⛶ pantalla completa"; };
  document.addEventListener("fullscreenchange", onFs);

  const mode = window.__graphMode || "2d";
  const setActive = m => { b2.classList.toggle("on", m === "2d"); b3.classList.toggle("on", m === "3d"); };
  setActive(mode);
  const gv = CogoGraph.mount(wrap, { nodes, edges }, { mode, onSelect: id => openNoteModal(id) });
  window.__gv = gv;
  gv.setColorFilter(state.graphColors); // reaplicar el filtro si volvés a la pestaña
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
    drifted: ["⟳ cambió", "ev-status warn", "El archivo citado cambió desde la última verificación → la nota baja a amarillo hasta que la re-verifiques."],
    broken: ["✗ no resuelve", "ev-status bad", "El archivo citado no existe → esta evidencia NO cuenta para el color."],
    unchecked: ["— sin chequear", "ev-status muted", "COGO no puede verificar esta ref sin conexión (log, comando, URL o ruta sin raíz)."],
  };
  const [text, className, title] = map[status] || ["", "ev-status", ""];
  node.textContent = text;
  node.className = className;
  node.title = title;
}

// ---------- Conexiones MCP (tokens de acceso) ----------
function tokenRow(t, refresh) {
  const row = el("div", "tk-row");
  const info = el("div", "tk-info");
  const head = el("div", "tk-name");
  head.appendChild(el("span", "tk-label", t.label));
  if (t.readonly) head.appendChild(el("span", "tk-ro", "solo lectura"));
  info.appendChild(head);
  const parts = ["creado " + t.created, t.last_used ? "usado " + t.last_used : "sin usar"];
  if (t.expires) parts.push("vence " + t.expires);
  info.appendChild(el("div", "tk-meta", parts.join(" · ")));
  row.appendChild(info);
  const rev = el("button", "nc-act danger", "revocar");
  rev.addEventListener("click", async () => {
    const ok = await confirmDialog({
      title: "Revocar token", note: t.label,
      message: "La app que use este token pierde el acceso al instante. Los demás tokens siguen funcionando.",
      confirmText: "Revocar", danger: true,
    });
    if (!ok) return;
    await api("/api/tokens?id=" + encodeURIComponent(t.id), { method: "DELETE" });
    refresh();
  });
  row.appendChild(rev);
  return row;
}

async function openTokens() {
  $("#menu").classList.add("hidden");
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Conexiones MCP"));
  card.appendChild(el("p", "tk-intro", "Tokens que le das a cada app o agente para conectarse por MCP (Claude Code, Cursor…). Cada uno se revoca solo, sin tocar los demás. El secreto se muestra una sola vez."));

  const list = el("div", "tk-list");
  card.appendChild(list);
  async function refresh() {
    let r;
    try { r = await api("/api/tokens"); } catch (e) { r = null; }
    list.innerHTML = "";
    const toks = (r && r.tokens) || [];
    if (!toks.length) { list.appendChild(el("div", "tk-empty", "Todavía no emitiste ningún token.")); return; }
    toks.forEach(t => list.appendChild(tokenRow(t, refresh)));
  }

  card.appendChild(el("div", "tk-form-lbl", "Nuevo token"));
  const form = el("div", "tk-form");
  const lbl = el("input"); lbl.placeholder = "etiqueta — ej: Claude Code (laptop)";
  const frow = el("div", "tk-form-row");
  const exp = select([["0", "no vence"], ["30", "vence en 30 días"], ["90", "vence en 90 días"], ["365", "vence en 1 año"]], "0", () => {});
  const roWrap = el("label", "tk-check");
  const roCb = el("input"); roCb.type = "checkbox";
  roWrap.appendChild(roCb); roWrap.appendChild(el("span", null, "solo lectura"));
  const create = el("button", "mini", "Emitir token");
  frow.appendChild(exp); frow.appendChild(roWrap); frow.appendChild(create);
  const reveal = el("div", "tk-reveal hidden");
  form.appendChild(lbl); form.appendChild(frow); form.appendChild(reveal);
  card.appendChild(form);

  create.addEventListener("click", async () => {
    const label = lbl.value.trim();
    if (!label) { lbl.focus(); return; }
    create.disabled = true;
    const r = await api("/api/tokens", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ label, expires_days: parseInt(exp.value, 10) || 0, readonly: roCb.checked }) }).catch(() => null);
    create.disabled = false;
    if (!r || !r.ok) { reveal.className = "tk-reveal bad"; reveal.textContent = "No se pudo emitir (" + ((r && r.error) || "error") + ")."; return; }
    reveal.className = "tk-reveal";
    reveal.innerHTML = "";
    reveal.appendChild(el("div", "tk-reveal-lbl", "Copiá este token ahora — no se vuelve a mostrar:"));
    const sr = el("div", "tk-secret-row");
    const code = el("code", "tk-secret"); code.textContent = r.token;
    const copy = el("button", "mini ghost", "copiar");
    copy.addEventListener("click", () => { navigator.clipboard.writeText(r.token); copy.textContent = "copiado"; setTimeout(() => copy.textContent = "copiar", 1200); });
    sr.appendChild(code); sr.appendChild(copy);
    reveal.appendChild(sr);

    // …o la config lista para el .mcp.json de Claude Code, con este token puesto.
    const cfg = JSON.stringify({ mcpServers: { cogo: { type: "http", url: location.origin + "/mcp", headers: { Authorization: "Bearer " + r.token } } } }, null, 2);
    reveal.appendChild(el("div", "tk-cfg-lbl", "…o pegá esto en el .mcp.json de tu Claude Code:"));
    const pre = el("pre", "tk-cfg"); pre.textContent = cfg;
    reveal.appendChild(pre);
    const copyCfg = el("button", "mini", "copiar configuración");
    copyCfg.addEventListener("click", () => { navigator.clipboard.writeText(cfg); copyCfg.textContent = "copiado ✓"; setTimeout(() => copyCfg.textContent = "copiar configuración", 1400); });
    reveal.appendChild(copyCfg);

    lbl.value = ""; roCb.checked = false;
    refresh();
  });

  refresh();
  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

// ---------- Papelera ----------
function trashRow(t, refresh) {
  const row = el("div", "tk-row");
  const info = el("div", "tk-info");
  const head = el("div", "tk-name");
  head.appendChild(el("span", "tk-label", t.id));
  head.appendChild(el("span", "nc-type", t.type + (t.project ? " · " + t.project : "")));
  info.appendChild(head);
  if (t.claim) info.appendChild(el("div", "tk-meta", t.claim));
  row.appendChild(info);
  const restore = el("button", "nc-act", "restaurar");
  restore.addEventListener("click", async () => {
    const r = await api("/api/trash?id=" + encodeURIComponent(t.id) + "&action=restore", { method: "POST" });
    if (r && r.ok === false) { await confirmDialog({ title: "No se pudo restaurar", message: r.error, confirmText: "Entendido" }); }
    refresh(); render();
  });
  const purge = el("button", "nc-act danger", "borrar def.");
  purge.addEventListener("click", async () => {
    const ok = await confirmDialog({ title: "Borrar para siempre", note: t.id, message: "Se elimina del disco definitivamente. Esto NO se puede deshacer.", confirmText: "Borrar", danger: true });
    if (!ok) return;
    await api("/api/trash?id=" + encodeURIComponent(t.id) + "&action=purge", { method: "POST" });
    refresh();
  });
  row.appendChild(restore); row.appendChild(purge);
  return row;
}

async function openTrash() {
  $("#menu").classList.add("hidden");
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Papelera"));
  card.appendChild(el("p", "tk-intro", "Notas borradas. Siguen en disco (.cogo/trash) — restauralas al vault o borralas para siempre."));
  const list = el("div", "tk-list");
  card.appendChild(list);
  async function refresh() {
    const r = await api("/api/trash").catch(() => null);
    list.innerHTML = "";
    const items = (r && r.trash) || [];
    if (!items.length) { list.appendChild(el("div", "tk-empty", "La papelera está vacía.")); return; }
    items.forEach(t => list.appendChild(trashRow(t, refresh)));
  }
  refresh();
  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

// ---------- Auditoría MCP ----------
// Traza append-only de quién (token / usuario federado / root) llamó a qué
// herramienta MCP y a qué endpoint de escritura, con hora e IP.
async function openAudit() {
  $("#menu").classList.add("hidden");
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Auditoría MCP"));
  card.appendChild(el("p", "tk-intro", "Registro append-only de cada llamada MCP y cada escritura por API: quién, qué herramienta, cuándo y desde qué IP. Se guarda en .cogo/audit.jsonl y se auto-recorta para no crecer sin fin."));
  const bar = el("div", "au-bar");
  const count = el("div", "au-count");
  const acts = el("div", "au-bar-acts");
  const dl = el("button", "mini ghost", "descargar");
  const clr = el("button", "mini ghost au-danger", "vaciar");
  acts.appendChild(dl); acts.appendChild(clr);
  bar.appendChild(count); bar.appendChild(acts);
  card.appendChild(bar);
  const list = el("div", "tk-list au-list");
  card.appendChild(list);

  async function refresh() {
    const r = await api("/api/audit").catch(() => null);
    const items = (r && r.entries) || [];
    const total = (r && r.total) || 0;
    const cap = r ? (r.cap || 0) : 0;
    let mt = total + (total === 1 ? " registro" : " registros");
    if (total > items.length) mt += " · mostrando los últimos " + items.length;
    mt += cap > 0 ? " · se recorta solo a " + cap : " · sin límite";
    count.textContent = mt;
    dl.disabled = clr.disabled = total === 0;
    list.textContent = "";
    if (!items.length) list.appendChild(el("div", "tk-empty", "Todavía no hay actividad registrada."));
    else items.forEach(e => list.appendChild(auditRow(e, refresh)));
  }
  await refresh();

  dl.addEventListener("click", async () => {
    const tok = localStorage.getItem("cogo.token");
    const res = await fetch("/api/audit?download=1", { headers: tok ? { Authorization: "Bearer " + tok } : {} }).catch(() => null);
    if (!res || !res.ok) return;
    const text = await res.text();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "application/x-ndjson" }));
    a.download = "cogo-audit.jsonl"; a.click();
    URL.revokeObjectURL(a.href);
  });
  clr.addEventListener("click", async () => {
    const ok = await confirmDialog({ title: "Vaciar la auditoría", message: "Se borra TODO el registro de actividad (.cogo/audit.jsonl). No se puede deshacer — si la necesitás, descargá una copia antes.", confirmText: "Vaciar", danger: true });
    if (!ok) return;
    await api("/api/audit", { method: "DELETE" }).catch(() => null);
    await refresh();
  });

  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

// ---------- Explorador de repositorio ----------
// Ver los archivos del repo SIN irse a github.com. No guarda nada: lo que COGO
// persiste es la cita que armás desde acá. Por eso el flujo termina siempre en
// el mismo lugar: clic en una línea → referencia `github://…` lista para usar
// como evidencia (y si tenés el editor abierto, se inserta sola).
async function openRepo(onPick) {
  $("#menu").classList.add("hidden");
  const st = { repo: localStorage.getItem("cogo.repo") || "", ref: localStorage.getItem("cogo.repo.ref") || "", path: "" };
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card repo-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Explorar repositorio"));
  card.appendChild(el("p", "tk-intro", "Mirá los archivos del repo sin salir de COGO y citá una línea como evidencia. No se guarda ninguna copia: lo que queda es la cita."));

  const bar = el("div", "repo-bar");
  const ri = el("input", "repo-in"); ri.placeholder = "owner/repositorio"; ri.value = st.repo;
  const rf = el("input", "repo-ref"); rf.placeholder = "rama o commit (opcional)"; rf.value = st.ref;
  const go = el("button", "mini", "abrir");
  bar.append(ri, rf, go);
  card.appendChild(bar);

  const crumbs = el("div", "repo-crumbs");
  const list = el("div", "repo-list");
  const status = el("div", "agt-bl-status");
  card.append(crumbs, status, list);

  function crumbTo(p) { st.path = p; load(); }
  function drawCrumbs() {
    crumbs.textContent = "";
    const root = el("a", "repo-crumb", st.repo || "raíz");
    root.addEventListener("click", () => crumbTo("")); crumbs.appendChild(root);
    let acc = "";
    (st.path ? st.path.split("/") : []).forEach(seg => {
      acc = acc ? acc + "/" + seg : seg;
      const to = acc;
      crumbs.appendChild(el("span", "repo-sep", "/"));
      const a = el("a", "repo-crumb", seg);
      a.addEventListener("click", () => crumbTo(to));
      crumbs.appendChild(a);
    });
  }

  async function load() {
    st.repo = ri.value.trim(); st.ref = rf.value.trim();
    if (!st.repo) { status.textContent = "escribí un repositorio como owner/nombre"; return; }
    localStorage.setItem("cogo.repo", st.repo); localStorage.setItem("cogo.repo.ref", st.ref);
    drawCrumbs(); list.textContent = "";
    setWorking(status, "leyendo el repositorio…");
    const q = new URLSearchParams({ repo: st.repo, ref: st.ref, path: st.path });
    const r = await apiOrError("/api/github?" + q);
    status.textContent = "";
    if (!r.ok) { status.textContent = "⚠ " + r.error; return; }
    (r.entries || []).forEach(e => {
      const row = el("div", "repo-row");
      row.appendChild(el("span", "repo-ico", e.type === "dir" ? "▸" : "·"));
      row.appendChild(el("span", "repo-name" + (e.type === "dir" ? " dir" : ""), e.name));
      row.addEventListener("click", () => { st.path = e.path; e.type === "dir" ? load() : loadFile(); });
      list.appendChild(row);
    });
    if (!(r.entries || []).length) list.appendChild(el("div", "tk-empty", "Carpeta vacía."));
  }

  async function loadFile() {
    drawCrumbs(); list.textContent = "";
    setWorking(status, "abriendo el archivo…");
    const q = new URLSearchParams({ repo: st.repo, ref: st.ref, path: st.path, file: "1" });
    const r = await apiOrError("/api/github?" + q);
    status.textContent = "";
    if (!r.ok) {
      status.textContent = "⚠ " + r.error;
      if (r && r.html_url) { const a = el("a", "link", "abrir en GitHub"); a.href = r.html_url; a.target = "_blank"; status.appendChild(a); }
      return;
    }
    const hint = el("div", "repo-hint", "Clic en una línea para citarla como evidencia.");
    list.appendChild(hint);
    const pre = el("div", "repo-code");
    (r.lines || []).forEach((ln, i) => {
      const row = el("div", "repo-ln");
      row.appendChild(el("span", "repo-num", String(i + 1)));
      row.appendChild(el("span", "repo-txt", ln || " "));
      row.addEventListener("click", () => pick(i + 1));
      pre.appendChild(row);
    });
    list.appendChild(pre);
  }

  async function pick(line) {
    const ref = "github://" + st.repo + (st.ref ? "@" + st.ref : "") + "/" + st.path + ":" + line;
    if (onPick) { onPick(ref); close(); return; }
    await navigator.clipboard.writeText(ref).catch(() => {});
    await confirmDialog({
      title: "Cita copiada", note: ref,
      message: "Pegala como referencia de evidencia en una nota. Si citás una rama, COGO va a avisarte cuando ese archivo cambie; si citás un commit fijo, la cita es inmutable.",
      confirmText: "Listo", cancelText: "Cerrar",
    });
  }

  go.addEventListener("click", () => { st.path = ""; load(); });
  ri.addEventListener("keydown", e => { if (e.key === "Enter") { st.path = ""; load(); } });
  if (st.repo) load(); else status.textContent = "escribí un repositorio para empezar";

  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  function close() { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); }
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

// ---------- Leases (coordinación multi-agente) ----------
// Permisos con vencimiento que los agentes toman antes de una tarea no
// idempotente (migración, deploy) para no pisarse. El operador puede liberarlos.
async function openLeases() {
  $("#menu").classList.add("hidden");
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Leases · coordinación"));
  card.appendChild(el("p", "tk-intro", "Permisos con vencimiento que los agentes toman antes de una tarea no idempotente (una migración, un deploy) para no pisarse. Expiran solos. Podés liberar uno a la fuerza si un agente quedó colgado."));
  const list = el("div", "tk-list au-list");
  card.appendChild(list);
  async function refresh() {
    const r = await api("/api/leases").catch(() => null);
    const items = (r && r.leases) || [];
    list.textContent = "";
    if (!items.length) { list.appendChild(el("div", "tk-empty", "No hay leases tomados ahora.")); return; }
    items.forEach(l => list.appendChild(leaseRow(l, refresh)));
  }
  await refresh();
  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

function leaseRow(l, refresh) {
  const row = el("div", "au-row");
  row.appendChild(el("span", "au-who au-token", l.holder || "?"));
  const mid = el("div", "au-mid");
  mid.appendChild(el("div", "au-act", l.name));
  const when = el("div", "au-when");
  let t = l.expires || "";
  try { t = "hasta " + new Date(l.expires).toLocaleString(); } catch (_) {}
  when.appendChild(el("span", null, t));
  if (l.note) when.appendChild(el("span", "au-ip", l.note));
  mid.appendChild(when);
  row.appendChild(mid);
  const rel = el("button", "mini ghost au-danger", "liberar");
  rel.addEventListener("click", async () => {
    const ok = await confirmDialog({ title: "Liberar el lease", message: "Vas a liberar «" + l.name + "» (lo tiene " + l.holder + ") a la fuerza. Si ese agente sigue trabajando, otro podría arrancar la misma tarea.", confirmText: "Liberar", danger: true });
    if (!ok) return;
    await api("/api/leases?name=" + encodeURIComponent(l.name), { method: "DELETE" }).catch(() => null);
    if (refresh) await refresh();
  });
  row.appendChild(rel);
  return row;
}

// callerKind mapea el prefijo del caller ("token:…"/"user:…"/"root"/"anon") a una
// etiqueta corta y una clase de color para el badge.
function callerKind(c) {
  if (c === "root") return ["root", "au-root"];
  if (c === "anon" || !c) return ["sin auth", "au-anon"];
  if (c.startsWith("token:")) return [c.slice(6), "au-token"];
  if (c.startsWith("user:")) return [c.slice(5), "au-user"];
  return [c, "au-token"];
}

function auditRow(e, refresh) {
  const row = el("div", "au-row");
  const [who, kls] = callerKind(e.caller);
  const badge = el("span", "au-who " + kls, who);
  row.appendChild(badge);
  const mid = el("div", "au-mid");
  const act = e.tool ? ("mcp · " + e.tool) : (e.method + " " + (e.path || "").replace(/^\/api\//, ""));
  mid.appendChild(el("div", "au-act", act));
  const when = el("div", "au-when");
  let t = e.time || "";
  try { t = new Date(e.time).toLocaleString(); } catch (_) {}
  when.appendChild(el("span", null, t));
  if (e.ip) when.appendChild(el("span", "au-ip", e.ip));
  mid.appendChild(when);
  row.appendChild(mid);
  const del = el("button", "au-del", "");
  del.title = "Borrar este registro";
  del.innerHTML = '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  del.addEventListener("click", async () => {
    del.disabled = true;
    await api("/api/audit", { method: "DELETE", headers: { "Content-Type": "application/json" }, body: JSON.stringify(e) }).catch(() => null);
    if (refresh) await refresh();
  });
  row.appendChild(del);
  return row;
}

// ---------- Raíces de evidencia (por proyecto) ----------
// COGO resuelve refs de evidencia relativas ("cmd/main.go") contra una carpeta
// base. Como cada proyecto vive en un repo distinto, la base es por proyecto,
// con un default global de reserva.
async function openEvidenceRoots() {
  $("#menu").classList.add("hidden");
  const data = await api("/api/evidence-roots").catch(() => null);
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Raíces de evidencia"));
  card.appendChild(el("p", "tk-intro", "Cada proyecto vive en su propio repo. COGO resuelve las refs de evidencia relativas (p. ej. `cmd/main.go`) contra la raíz del proyecto correspondiente; si un proyecto no tiene raíz, usa el default global. Una ref que no resuelve deja de contar para el color."));

  card.appendChild(el("div", "tk-form-lbl", "Default global"));
  const defInput = el("input"); defInput.type = "text"; defInput.placeholder = "p. ej. E:/repos (o vacío)";
  defInput.value = (data && data.default) || "";
  card.appendChild(defInput);

  card.appendChild(el("div", "tk-form-lbl", "Por proyecto"));
  const rows = el("div", "tk-list");
  card.appendChild(rows);
  const known = (data && data.known_projects) || [];
  const current = (data && data.projects) || {};
  // Fila por proyecto: los ya configurados, más los conocidos del vault sin raíz.
  const projects = Array.from(new Set([...Object.keys(current), ...known])).sort();
  function evRow(proj, val) {
    const row = el("div", "tk-row ev-row");
    row.appendChild(el("span", "tk-label ev-proj", proj));
    const inp = el("input"); inp.type = "text"; inp.placeholder = "carpeta raíz del repo"; inp.value = val || "";
    inp.dataset.proj = proj;
    row.appendChild(inp);
    return row;
  }
  if (!projects.length) rows.appendChild(el("div", "tk-empty", "Todavía no hay proyectos en el vault."));
  else projects.forEach(p => rows.appendChild(evRow(p, current[p])));

  const actions = el("div", "tk-form-row");
  const save = el("button", "mini", "Guardar");
  const msg = el("span", "ev-msg");
  actions.appendChild(save); actions.appendChild(msg);
  card.appendChild(actions);
  save.addEventListener("click", async () => {
    const projMap = {};
    rows.querySelectorAll("input[data-proj]").forEach(inp => {
      const v = inp.value.trim();
      if (v) projMap[inp.dataset.proj] = v;
    });
    const r = await api("/api/evidence-roots", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ default: defInput.value.trim(), projects: projMap }) }).catch(() => null);
    if (r && r.ok) { msg.textContent = "Guardado ✓"; msg.className = "ev-msg ok"; render(); }
    else { msg.textContent = (r && r.error) || "Error al guardar"; msg.className = "ev-msg bad"; }
  });

  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
}

// ---------- Instrucciones para agentes (AGENTS.md / CLAUDE.md) ----------
// Genera el archivo bootstrap que le enseña a un agente el protocolo de COGO y
// cómo conectarse por MCP. Opcionalmente embebe una instantánea de la memoria.
async function openAgents() {
  $("#menu").classList.add("hidden");
  const back = el("div", "modal-back confirm-back");
  const card = el("div", "modal-card tokens-modal agents-modal");
  const x = el("button", "modal-x"); x.setAttribute("aria-label", "Cerrar");
  x.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  card.appendChild(x);
  card.appendChild(el("h2", "modal-tit", "Instrucciones para agentes"));
  card.appendChild(el("p", "tk-intro", "El archivo que un agente (Claude Code, Cursor…) lee al arrancar: le dice que hay una memoria COGO, cómo obedecer el color y cómo conectarse por MCP. Copialo a la raíz de tu repo."));

  const ctrls = el("div", "tk-form-row ag-ctrls");
  const seg = el("div", "seg");
  const bAgents = el("button", "seg-btn on", "AGENTS.md");
  const bClaude = el("button", "seg-btn", "CLAUDE.md");
  seg.appendChild(bAgents); seg.appendChild(bClaude);
  const digWrap = el("label", "tk-check");
  const digCb = el("input"); digCb.type = "checkbox";
  digWrap.appendChild(digCb); digWrap.appendChild(el("span", null, "incluir instantánea de la memoria"));
  ctrls.appendChild(seg); ctrls.appendChild(digWrap);
  card.appendChild(ctrls);

  const pre = el("pre", "ag-pre mono");
  card.appendChild(pre);

  const actions = el("div", "ag-actions");
  const copy = el("button", "mini", "copiar");
  const dl = el("button", "mini ghost", "descargar");
  actions.appendChild(copy); actions.appendChild(dl);
  card.appendChild(actions);

  let state = { tool: "", digest: false, md: "", filename: "AGENTS.md" };
  async function refresh() {
    pre.textContent = "generando…";
    const q = new URLSearchParams();
    if (state.tool) q.set("tool", state.tool);
    if (state.digest) q.set("digest", "1");
    const r = await api("/api/agents-md?" + q.toString()).catch(() => null);
    if (!r) { pre.textContent = "no se pudo generar"; return; }
    state.md = r.markdown; state.filename = r.filename;
    pre.textContent = r.markdown;
  }
  bAgents.addEventListener("click", () => { state.tool = ""; bAgents.classList.add("on"); bClaude.classList.remove("on"); refresh(); });
  bClaude.addEventListener("click", () => { state.tool = "claude"; bClaude.classList.add("on"); bAgents.classList.remove("on"); refresh(); });
  digCb.addEventListener("change", () => { state.digest = digCb.checked; refresh(); });
  copy.addEventListener("click", () => { navigator.clipboard.writeText(state.md); copy.textContent = "copiado ✓"; setTimeout(() => copy.textContent = "copiar", 1400); });
  dl.addEventListener("click", () => {
    const blob = new Blob([state.md], { type: "text/markdown" });
    const a = document.createElement("a"); a.href = URL.createObjectURL(blob); a.download = state.filename; a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  });
  refresh();

  back.appendChild(card);
  document.body.appendChild(back);
  requestAnimationFrame(() => back.classList.add("show"));
  const close = () => { back.classList.remove("show"); setTimeout(() => back.remove(), 160); document.removeEventListener("keydown", onKey); };
  const onKey = e => { if (e.key === "Escape") close(); };
  x.addEventListener("click", close);
  back.addEventListener("click", e => { if (e.target === back) close(); });
  document.addEventListener("keydown", onKey);
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

  // La traza detrás del rojo-por-contradicción: con qué nota(s) choca y por qué.
  if (n.contradictions && n.contradictions.length) {
    const cbx = el("div", "nm-contra");
    cbx.appendChild(el("div", "nm-contra-tit", "⚠ Contradicciones abiertas"));
    n.contradictions.forEach(c => {
      const row = el("div", "nm-contra-row");
      const head = el("div", "nm-contra-head");
      head.appendChild(el("span", null, "contradice "));
      const link = el("button", "nm-contra-id", c.other);
      link.addEventListener("click", () => { close(); openNoteModal(c.other); });
      head.appendChild(link);
      row.appendChild(head);
      if (c.reason) row.appendChild(el("div", "nm-contra-reason", c.reason));
      cbx.appendChild(row);
    });
    card.appendChild(cbx);
  }

  const body = el("div", "nm-body md-render");
  body.innerHTML = mdToHtml(n.body);
  card.appendChild(body);

  if (n.evidence && n.evidence.length) {
    const ev = el("div", "nm-block");
    ev.appendChild(el("div", "nm-label", "Evidencia"));
    n.evidence.forEach(e => {
      const row = el("div", "nm-ev-row");
      row.appendChild(el("span", "nm-ev-kind", e.kind));
      const ref = (e.ref || "");
      if (ref.startsWith("artifact://")) {
        const sha = ref.slice("artifact://".length);
        row.appendChild(el("span", "nm-ev-ref", "artefacto " + sha.slice(0, 12) + "…"));
        const dl = el("a", "nm-ev-dl", "descargar");
        dl.href = "#"; dl.title = "Baja el artefacto guardado (verifica el hash)";
        dl.addEventListener("click", ev2 => { ev2.preventDefault(); downloadArtifact(sha); });
        row.appendChild(dl);
      } else {
        row.appendChild(el("span", "nm-ev-ref", ref));
      }
      if (e.status) { const b = el("span"); paintEvBadge(b, e.status); row.appendChild(b); }
      ev.appendChild(row);
    });
    card.appendChild(ev);
  }
  if (n.author || (n.scope && Object.keys(n.scope).length)) {
    const meta = el("div", "nm-block");
    if (n.author) {
      const r = el("div", "nm-ev-row");
      r.appendChild(el("span", "nm-ev-kind", "por"));
      r.appendChild(el("span", "nm-ev-ref", n.author));
      meta.appendChild(r);
    }
    if (n.scope && Object.keys(n.scope).length) {
      const r = el("div", "nm-ev-row");
      r.appendChild(el("span", "nm-ev-kind", "alcance"));
      r.appendChild(el("span", "nm-ev-ref", Object.keys(n.scope).sort().map(k => k + "=" + n.scope[k]).join("  ")));
      meta.appendChild(r);
    }
    card.appendChild(meta);
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

  const hist = await api("/api/note/history?id=" + encodeURIComponent(id)).catch(() => null);
  if (hist && hist.versions && hist.versions.length) {
    const hb = el("div", "nm-block");
    hb.appendChild(el("div", "nm-label", "Historia — " + hist.versions.length + " versión(es)"));
    const tl = el("div", "nm-hist");
    hist.versions.slice().reverse().forEach(v => {
      const row = el("div", "nm-hist-row");
      const dot = el("span", "dot " + cls(v.color));
      row.appendChild(dot);
      const info = el("div", "nm-hist-info");
      const t = new Date(v.time);
      info.appendChild(el("span", "nm-hist-time", isNaN(+t) ? v.time : t.toLocaleString()));
      info.appendChild(el("span", "nm-hist-reason", colorWord(v.color) + " · " + v.reason));
      row.appendChild(info);
      tl.appendChild(row);
    });
    hb.appendChild(tl);
    card.appendChild(hb);
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

  const body = el("textarea", "md"); body.value = d.body; body.setAttribute("rows", "10");
  const mdEd = mdEditor(body, () => { d.body = body.value; preview(); });
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
    const actions = el("div", "ev-actions");
    const add = el("button", "mini ghost", "+ evidencia");
    add.addEventListener("click", () => { d.evidence.push({ kind: "file_read", ref: "" }); renderEv(); });
    actions.appendChild(add);
    const repo = el("button", "mini ghost", "buscar en el repo");
    repo.title = "Explorá el repositorio y citá una línea como evidencia, sin salir de COGO.";
    repo.addEventListener("click", () => openRepo(ref => {
      d.evidence.push({ kind: "file_read", ref });
      renderEv(); preview();
    }));
    actions.appendChild(repo);
    const attach = el("button", "mini ghost", "adjuntar archivo");
    attach.title = "Sube un archivo al store por su hash y lo cita como evidencia (artifact://). Un guard de secretos corre antes de guardar.";
    attach.addEventListener("click", () => {
      const inp = document.createElement("input"); inp.type = "file";
      inp.addEventListener("change", async () => {
        const file = inp.files && inp.files[0]; if (!file) return;
        if (file.size > 8 * 1024 * 1024) { await confirmDialog({ title: "Archivo muy grande", message: "Máximo 8 MB para un artefacto. Para algo más grande, va al repo.", confirmText: "Entendido" }); return; }
        const b64 = await fileToBase64(file);
        const send = redact => api("/api/artifact", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ content_base64: b64, content_type: file.type || "application/octet-stream", redact }) }).catch(() => null);
        let r = await send(false);
        if (r && r.blocked) {
          const rules = (r.findings || []).map(f => f.rule).join(", ");
          if (!(await confirmDialog({ title: "Posible secreto en el archivo", message: "Detecté: " + rules + ". No se guarda tal cual — un hash inmutable no se borra. ¿Guardar una copia con los secretos censurados?", confirmText: "Guardar censurado", danger: true }))) return;
          r = await send(true);
        }
        if (!r || !r.ok) { await confirmDialog({ title: "No se pudo guardar", message: (r && r.error) || "Error al subir el artefacto.", confirmText: "Cerrar" }); return; }
        d.evidence.push({ kind: "file_read", ref: r.ref });
        renderEv(); preview();
      });
      inp.click();
    });
    actions.appendChild(attach);
    evWrap.appendChild(actions);
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

  // Contradicciones persistidas (sobreviven al reinicio; se resuelven a mano).
  const contraBox = el("div", "contra-box");
  main.appendChild(contraBox);
  async function loadContras() {
    const r = await api("/api/contradictions").catch(() => null);
    contraBox.innerHTML = "";
    const cs = (r && r.contradictions) || [];
    if (!cs.length) return;
    contraBox.appendChild(el("div", "contra-title", cs.length + " contradicción(es) abierta(s) — pintan rojo hasta que las resuelvas"));
    cs.forEach(c => contraBox.appendChild(contraCard(c, loadContras)));
  }
  loadContras();

  const out = el("div", "lint-out");
  main.appendChild(out);

  btn.addEventListener("click", async () => {
    btn.disabled = true;
    setWorking(status, state.llmConfigured
      ? "revisando… comparando notas par por par con el modelo (puede tardar unos segundos)"
      : "revisando… (checks deterministas)");
    try {
      const r = await api("/api/lint", { method: "POST" });
      status.textContent = r.llm_used ? ("modelo: " + r.pairs_checked + "/" + r.candidate_pairs + " pares revisados") : "checks deterministas (sin modelo)";
      out.innerHTML = "";
      const other = (r.issues || []).filter(is => is.kind !== "contradiction");
      if (!other.length && !r.contradictions) { out.appendChild(el("div", "empty", "Todo limpio. Nada que revisar.")); }
      const LABEL = { broken_dep: "Enlace roto", stale: "Vencida" };
      other.forEach(is => {
        const row = el("div", "lint-row lint-" + is.kind);
        row.appendChild(el("span", "lint-tag", LABEL[is.kind] || is.kind));
        row.appendChild(el("span", "lint-msg", is.msg));
        out.appendChild(row);
      });
      loadContras(); // las contradicciones nuevas se sumaron al store
    } catch (e) {
      status.textContent = "⚠ no se pudo revisar (el modelo tardó o falló). Reintentá.";
    } finally {
      btn.disabled = false;
    }
  });
}

function contraCard(c, refresh) {
  const card = el("div", "contra-card");
  const head = el("div", "contra-head");
  head.appendChild(el("span", "contra-tag", "Contradicción"));
  if (c.detected) head.appendChild(el("span", "contra-when", "detectada " + c.detected));
  card.appendChild(head);
  if (c.reason) card.appendChild(el("div", "contra-reason", c.reason));
  const pair = el("div", "contra-pair");
  [[c.a, c.a_claim], [c.b, c.b_claim]].forEach(([id, claim]) => {
    const side = el("div", "contra-side");
    const idEl = el("a", "contra-id", id);
    idEl.addEventListener("click", () => openNoteModal(id));
    side.appendChild(idEl);
    side.appendChild(el("div", "contra-claim", claim || "—"));
    pair.appendChild(side);
  });
  card.appendChild(pair);
  const acts = el("div", "contra-acts");
  const resolve = el("button", "mini", "Ya lo resolví");
  resolve.addEventListener("click", async () => { await api("/api/contradictions?id=" + encodeURIComponent(c.id) + "&action=resolve", { method: "POST" }); refresh(); });
  const dismiss = el("button", "mini ghost", "No es contradicción");
  dismiss.addEventListener("click", async () => { await api("/api/contradictions?id=" + encodeURIComponent(c.id) + "&action=dismiss", { method: "POST" }); refresh(); });
  acts.appendChild(resolve); acts.appendChild(dismiss);
  card.appendChild(acts);
  return card;
}

// ---------- veracidad (Motor de Veracidad · xray) ----------
// Etiquetas + tono (ok/warn/err/neutral) de cada eje de la radiografía.
const XR_COMMIT = { boosted: ["afirmado fuerte", "warn"], hedged: ["cauteloso", "ok"], neutral: ["neutro", "neutral"] };
const XR_EVID = { observed: ["evidencia observada", "ok"], reported: ["evidencia reportada", "warn"], none: ["sin evidencia", "err"] };
function xrPill(spec) {
  const [label, tone] = Array.isArray(spec) ? spec : [spec, "neutral"];
  return el("span", "xr-pill xr-" + (tone || "neutral"), label);
}

async function renderVeracidad(main) {
  viewHead(main, "Suite Escriba · Memoria", "Veracidad", "Radiografía una respuesta de IA por afirmación: cuánto se compromete el lenguaje vs cuánto fundamento declara. Determinista, sin modelo — el gemelo del Guard. No dice “es verdad”: marca lo afirmado fuerte sin fundamento y las opiniones disfrazadas de hecho.");
  const ta = el("textarea", "md"); ta.setAttribute("rows", "7"); ta.placeholder = "Pegá acá una respuesta de una IA…";
  const xbox = el("div", "gbox gbox-xray");
  xbox.appendChild(el("div", "gbox-lbl", "Respuesta a radiografiar"));
  xbox.appendChild(ta);
  main.appendChild(xbox);
  const bar = el("div", "viewbar");
  const btn = el("button", null, "Radiografiar");
  const overall = el("span", "xr-overall");
  bar.appendChild(btn); bar.appendChild(overall);
  main.appendChild(bar);
  const out = el("div", "xr-out");
  main.appendChild(out);
  btn.addEventListener("click", async () => {
    if (!ta.value.trim()) return;
    btn.disabled = true;
    const r = await api("/api/xray", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ answer: ta.value }) });
    btn.disabled = false;
    overall.className = "xr-overall " + cls(r.overall);
    overall.innerHTML = "";
    overall.appendChild(el("span", "dot"));
    overall.appendChild(el("strong", null, colorWord(r.overall)));
    if (r.summary) overall.appendChild(el("span", "xr-summary", r.summary));
    out.innerHTML = "";
    (r.claims || []).forEach((c, i) => {
      const row = el("div", "xr-claim " + cls(c.color));
      const rail = el("div", "xr-rail"); rail.appendChild(el("span", "dot")); rail.appendChild(el("span", "xr-num", "#" + (i + 1)));
      row.appendChild(rail);
      const body = el("div", "xr-claim-body");
      body.appendChild(el("div", "xr-claim-text", c.text));
      body.appendChild(el("div", "xr-claim-reason", c.reason));
      const pills = el("div", "xr-pills");
      pills.appendChild(xrPill(XR_COMMIT[c.commitment] || c.commitment));
      pills.appendChild(xrPill(XR_EVID[c.evidence] || [c.evidence, "neutral"]));
      pills.appendChild(xrPill(c.falsifiable ? ["falsable", "ok"] : ["opinión", "err"]));
      body.appendChild(pills);
      row.appendChild(body);
      out.appendChild(row);
    });
    if (!r.claims || !r.claims.length) out.appendChild(el("div", "empty", "No encontré afirmaciones para radiografiar."));
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

  // --- mandato persistente (global o por proyecto) ---
  const mand = el("div", "gbox gbox-mandate");
  mand.appendChild(el("div", "gbox-lbl", "Tu mandato (queda guardado en el vault)"));
  const mscope = el("select", "gbox-scope");
  mscope.appendChild(Object.assign(el("option", null, "global · todo el vault"), { value: "" }));
  [...$("#projsel").options].forEach(o => { if (o.value) mscope.appendChild(Object.assign(el("option", null, "proyecto: " + o.value), { value: o.value })); });
  mscope.value = state.project || "";
  mscope.title = "Elegí un proyecto para darle sus propias líneas rojas; si no tiene, cae al mandato global. recall(project:…) usa el que corresponda.";
  mand.appendChild(mscope);
  const goal = el("input"); goal.placeholder = "tu objetivo · ej: decidir mi carrera sin apuro";
  mand.appendChild(goal);
  const lines = el("textarea", "md"); lines.rows = 3;
  lines.placeholder = "tus líneas rojas, una por renglón · ej:\nno renuncio sin otra oferta firmada\nno invierto plata hoy";
  mand.appendChild(lines);
  async function loadMandate() {
    const m = await api("/api/mandate?project=" + encodeURIComponent(mscope.value)).catch(() => ({}));
    goal.value = m.goal || "";
    lines.value = (m.red_lines || []).join("\n");
  }
  await loadMandate();
  mscope.addEventListener("change", loadMandate);
  const mrow = el("div", "guard-mrow");
  const msave = el("button", "mini ghost", "guardar mandato");
  const mst = el("span", "lint-status");
  msave.addEventListener("click", async () => {
    await api("/api/mandate?project=" + encodeURIComponent(mscope.value), { method: "POST", headers: { "Content-Type": "application/json" },
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
  const cbox = el("div", "gbox gbox-context");
  cbox.appendChild(el("div", "gbox-lbl", "1 · Conversación previa (contexto, para los recibos)"));
  cbox.appendChild(trans);
  main.appendChild(cbox);

  const turn = el("textarea", "md"); turn.rows = 5;
  turn.placeholder = "el ÚLTIMO mensaje del modelo — el que se radiografía";
  const tbox = el("div", "gbox gbox-turn");
  tbox.appendChild(el("div", "gbox-lbl", "2 · Turno a analizar (el último mensaje del modelo)"));
  tbox.appendChild(turn);
  main.appendChild(tbox);

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
    run.disabled = true;
    setWorking(status, steel.checked ? "analizando… con steelman, dos llamadas al modelo (puede tardar)" : "analizando… el modelo mira la conversación");
    let r;
    try {
      r = await api("/api/guard", { method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ turn: turn.value, transcript: parseTranscript(trans.value), steelman: steel.checked }) });
    } catch (e) {
      status.textContent = "⚠ no se pudo analizar (el modelo tardó o falló). Reintentá.";
      run.disabled = false;
      return;
    }
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

    // Etiquetado humano: construye un corpus NO circular (el de eval fue etiquetado
    // por otro modelo). Tu juicio se guarda para medir el Guard honestamente.
    const lbl = el("div", "guard-label");
    lbl.appendChild(el("div", "field-lbl", "Etiquetá vos (para el corpus humano) — ¿este turno era manipulativo?"));
    const btns = el("div", "guard-label-btns");
    const done = el("span", "guard-label-done");
    const send = async (label) => {
      await api("/api/guard/label", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ turn: turn.value, guard_verdict: r.overall, label }) });
      const c = await api("/api/guard/label").catch(() => ({ count: "?" }));
      done.textContent = "✓ guardado · " + c.count + " etiquetas humanas juntadas";
    };
    ["Manipulativo", "Benigno"].forEach(t => {
      const b = el("button", "mini ghost", t);
      b.addEventListener("click", () => send(t === "Manipulativo" ? "manipulative" : "benign"));
      btns.appendChild(b);
    });
    lbl.appendChild(btns); lbl.appendChild(done);
    out.appendChild(lbl);
  });
}

async function openSettings() {
  $("#menu").classList.add("hidden");
  const s = await api("/api/settings");
  $("#setBase").value = s.base_url || "";
  $("#setModel").value = s.model || "";
  $("#setEmbed").value = s.embed_model || "";
  $("#setKey").value = "";
  $("#setKey").placeholder = s.has_key ? "•••• guardada — vacío = no cambiar" : "tu API key";
  const st = $("#setStatus");
  st.textContent = s.configured ? ("activo: " + s.name) : "apagado";
  st.className = "set-status " + (s.configured ? "ok" : "");
  $("#settingsModal").classList.remove("hidden");
}

async function saveSettings() {
  const body = JSON.stringify({ base_url: $("#setBase").value.trim(), model: $("#setModel").value.trim(), embed_model: $("#setEmbed").value.trim(), api_key: $("#setKey").value });
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
  $("#setTestEmbed").addEventListener("click", async () => {
    const hint = $("#setEmbedHint");
    hint.textContent = "probando embeddings…"; hint.className = "model-hint";
    const body = JSON.stringify({ base_url: $("#setBase").value.trim(), embed_model: $("#setEmbed").value.trim(), api_key: $("#setKey").value });
    const r = await api("/api/settings/test-embed", { method: "POST", headers: { "Content-Type": "application/json" }, body }).catch(() => null);
    if (r && r.ok) { hint.textContent = "✓ conecta — vectores de " + r.dim + " dimensiones (" + r.model + ")"; hint.className = "model-hint ok"; }
    else { hint.textContent = "✗ no conecta: " + ((r && r.error) || "error"); hint.className = "model-hint bad"; }
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
      const card = $("#loginGate .login-card");
      $("#loginSub").textContent = "La memoria con color de tu proyecto.";
      const sso = el("a", "login-sso", "Entrar con Lockatus"); sso.href = "/auth/login";
      const alt = el("a", "login-alt", "o entrá con un token de acceso");
      alt.addEventListener("click", () => showTokenGate(true));
      card.appendChild(sso); card.appendChild(alt);
    }
    return; // el overlay ya cubre la pantalla: nunca se ve la cromía detrás
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
  $("#loginGate").classList.add("hidden"); // autenticado (o standalone): recién acá sacamos la cubierta
  await loadConfig();
  render();
})();
