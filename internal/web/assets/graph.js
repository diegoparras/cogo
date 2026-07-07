"use strict";
/* COGO — motor de grafos en Canvas. Sin dependencias, sin CDN: 2D con física y
   glow, 3D "constelación" proyectada. El color sale del semáforo (--ok/--warn/
   --err); todo lo demás es grafito. Expone window.CogoGraph.mount(el, data, opts). */
(function () {
  const TAU = Math.PI * 2;

  function hexToRgb(h) {
    h = (h || "").trim();
    if (h[0] === "#") h = h.slice(1);
    if (h.length === 3) h = h.split("").map(c => c + c).join("");
    const n = parseInt(h, 16);
    if (isNaN(n) || h.length < 6) return { r: 140, g: 140, b: 150 };
    return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
  }
  const rgba = (c, a) => `rgba(${c.r},${c.g},${c.b},${a})`;
  const mix = (a, b, t) => ({ r: Math.round(a.r + (b.r - a.r) * t), g: Math.round(a.g + (b.g - a.g) * t), b: Math.round(a.b + (b.b - a.b) * t) });
  const WHITE = { r: 255, g: 255, b: 255 }, BLACK = { r: 0, g: 0, b: 0 };

  function readTokens() {
    // The graph panel is always a deep-graphite constellation, so the palette is
    // fixed to the vivid (dark-theme) semáforo regardless of the app theme —
    // vivid nodes + light labels pop over the dark field, and the glow is additive.
    return {
      dark: true,
      green: "#3fb950", yellow: "#d6a01a", red: "#f85149", ungraded: "#8b8b96",
      text: "#c7cedb", muted: "#8b8b96",
      hair: "rgba(255,255,255,.14)", accent: "#9aa4b2",
    };
  }
  const colorFor = (T, c) => ({ green: T.green, yellow: T.yellow, red: T.red, ungraded: T.ungraded }[c] || T.ungraded);

  // Deterministic pseudo-random from a string, so a vault lays out the same each load.
  function seed(str) {
    let h = 2166136261 >>> 0;
    for (let i = 0; i < str.length; i++) { h ^= str.charCodeAt(i); h = Math.imul(h, 16777619); }
    return () => { h += 0x6D2B79F5; let t = h; t = Math.imul(t ^ (t >>> 15), t | 1); t ^= t + Math.imul(t ^ (t >>> 7), t | 61); return ((t ^ (t >>> 14)) >>> 0) / 4294967296; };
  }

  // Cada relación: color PROPIO + estilo de línea distinto, para leerlas de un
  // vistazo. Azul=dependencia dura · ámbar=reemplaza · violeta=causa · gris=wiki.
  const KIND = {
    depends_on: { dash: [], w: 2.6, dir: true, color: "#5aa9e6", label: "depende de" },
    supersedes: { dash: [10, 6], w: 2.6, dir: true, color: "#e0913a", label: "reemplaza a" },
    caused_by: { dash: [4, 5], w: 2.3, dir: true, color: "#c07ad6", label: "causada por" },
    wikilink: { dash: [1.5, 5], w: 1.7, dir: false, color: "#8b93a3", label: "se relaciona" },
  };
  window.CogoGraphKinds = KIND; // la leyenda de aristas lo lee

  function mount(container, data, opts) {
    opts = opts || {};
    const canvas = document.createElement("canvas");
    canvas.className = "graph-canvas";
    container.appendChild(canvas);
    const tip = document.createElement("div");
    tip.className = "graph-tip hidden";
    container.appendChild(tip);
    const ctx = canvas.getContext("2d");

    let T = readTokens();
    let glow = {};
    function buildGlow() {
      glow = {};
      for (const key of ["green", "yellow", "red", "ungraded"]) {
        const S = 128, c = document.createElement("canvas"); c.width = c.height = S;
        const x = c.getContext("2d"), r = S / 2, col = hexToRgb(colorFor(T, key));
        const grd = x.createRadialGradient(r, r, 0, r, r, r);
        grd.addColorStop(0, rgba(col, .95)); grd.addColorStop(.18, rgba(col, .55));
        grd.addColorStop(.5, rgba(col, .16)); grd.addColorStop(1, rgba(col, 0));
        x.fillStyle = grd; x.fillRect(0, 0, S, S);
        glow[key] = c;
      }
    }
    buildGlow();

    // --- model ---
    const rnd = seed(data.nodes.map(n => n.id).join("|") || "cogo");
    const nodes = data.nodes.map(n => {
      const a = rnd() * TAU, b = Math.acos(2 * rnd() - 1), R = 120 + rnd() * 80;
      return {
        id: n.id, type: n.type, color: n.color, project: n.project, claim: n.claim, deg: 0,
        x: R * Math.sin(b) * Math.cos(a), y: R * Math.sin(b) * Math.sin(a), z: R * Math.cos(b),
        vx: 0, vy: 0, vz: 0,
      };
    });
    const byId = {}; nodes.forEach(n => byId[n.id] = n);
    const edges = data.edges.filter(e => byId[e.from] && byId[e.to]).map(e => ({ a: byId[e.from], b: byId[e.to], kind: e.kind }));
    edges.forEach(e => { e.a.deg++; e.b.deg++; });
    const maxDeg = Math.max(1, ...nodes.map(n => n.deg));
    const nbr = new Map(); nodes.forEach(n => nbr.set(n, new Set([n])));
    edges.forEach(e => { nbr.get(e.a).add(e.b); nbr.get(e.b).add(e.a); });
    const radius = n => 6 + 7 * Math.sqrt(n.deg / maxDeg);

    // --- view state ---
    let mode = opts.mode === "3d" ? "3d" : "2d";
    let alpha = 1;                         // simulation cooling
    let zoom = 1, panX = 0, panY = 0;      // 2D
    let yaw = 0.5, pitch = -0.35, spinY = 0, spinX = 0, autoSpin = 0.0016; // 3D
    let hovered = null, dragging = false, dragMoved = false, lastX = 0, lastY = 0;
    let W = 0, H = 0, dpr = 1;
    let colorFilter = null; // Set of colors to show; null/empty = all

    function resize() {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      W = container.clientWidth; H = container.clientHeight;
      canvas.width = Math.round(W * dpr); canvas.height = Math.round(H * dpr);
      canvas.style.width = W + "px"; canvas.style.height = H + "px";
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }
    const ro = new ResizeObserver(resize); ro.observe(container); resize();

    // --- physics (generic 2D/3D) ---
    function step() {
      if (alpha < 0.02) return;
      const K = alpha, rep = 1500, spring = 0.035, len = 78, damp = 0.9, centre = 0.012;
      for (let i = 0; i < nodes.length; i++) {
        const a = nodes[i];
        for (let j = i + 1; j < nodes.length; j++) {
          const b = nodes[j];
          let dx = a.x - b.x, dy = a.y - b.y, dz = a.z - b.z;
          let d2 = dx * dx + dy * dy + dz * dz + 0.01, d = Math.sqrt(d2);
          const f = (rep * K) / d2 / d;
          dx *= f; dy *= f; dz *= f;
          a.vx += dx; a.vy += dy; a.vz += dz; b.vx -= dx; b.vy -= dy; b.vz -= dz;
        }
        a.vx -= a.x * centre * K; a.vy -= a.y * centre * K; a.vz -= a.z * centre * K;
      }
      for (const e of edges) {
        let dx = e.b.x - e.a.x, dy = e.b.y - e.a.y, dz = e.b.z - e.a.z;
        const d = Math.sqrt(dx * dx + dy * dy + dz * dz) + 0.01;
        const f = spring * K * (d - len) / d;
        dx *= f; dy *= f; dz *= f;
        e.a.vx += dx; e.a.vy += dy; e.a.vz += dz; e.b.vx -= dx; e.b.vy -= dy; e.b.vz -= dz;
      }
      for (const n of nodes) {
        n.vx *= damp; n.vy *= damp; n.vz *= damp;
        n.x += n.vx; n.y += n.vy; n.z += n.vz;
      }
      alpha *= 0.985;
    }

    // --- projection ---
    function bounds3() {
      let r = 1; for (const n of nodes) r = Math.max(r, Math.hypot(n.x, n.y, n.z));
      return r;
    }
    function project() {
      // shared fit
      if (mode === "2d") {
        let minx = 1e9, miny = 1e9, maxx = -1e9, maxy = -1e9;
        for (const n of nodes) { if (n.x < minx) minx = n.x; if (n.x > maxx) maxx = n.x; if (n.y < miny) miny = n.y; if (n.y > maxy) maxy = n.y; }
        const gw = Math.max(1, maxx - minx), gh = Math.max(1, maxy - miny);
        const s = Math.min((W - 120) / gw, (H - 120) / gh) * zoom;
        const cx = (minx + maxx) / 2, cy = (miny + maxy) / 2;
        for (const n of nodes) { n.sx = W / 2 + (n.x - cx) * s + panX; n.sy = H / 2 + (n.y - cy) * s + panY; n.ss = s; n.depth = 1; }
      } else {
        const cy_ = Math.cos(pitch), sy_ = Math.sin(pitch), cx_ = Math.cos(yaw), sx_ = Math.sin(yaw);
        const R = bounds3(), cam = R * 2.5, focal = R * 2.4;
        const fit = Math.min(W, H) * 0.44 / R * zoom;
        for (const n of nodes) {
          // rotate: yaw around Y, then pitch around X
          const x1 = n.x * cx_ + n.z * sx_, z1 = -n.x * sx_ + n.z * cx_;
          const y2 = n.y * cy_ - z1 * sy_, z2 = n.y * sy_ + z1 * cy_;
          const persp = focal / Math.max(1, cam - z2);
          n.sx = W / 2 + x1 * fit * persp + panX;
          n.sy = H / 2 - y2 * fit * persp + panY;
          n.ss = fit * persp; n.depth = z2;
        }
      }
    }

    function edgeStyle(e) {
      const k = KIND[e.kind] || KIND.wikilink;
      return { stroke: k.color, w: k.w, dash: k.dash, dir: k.dir };
    }

    function draw() {
      ctx.clearRect(0, 0, W, H);
      project();
      const order = mode === "3d" ? nodes.slice().sort((a, b) => a.depth - b.depth) : nodes;
      const focusSet = hovered ? nbr.get(hovered) : null;
      const dimEdge = e => focusSet && !(focusSet.has(e.a) && (e.a === hovered || e.b === hovered) || focusSet.has(e.b) && (e.b === hovered || e.a === hovered));

      // edges
      ctx.lineCap = "round";
      for (const e of edges) {
        const st = edgeStyle(e);
        const touches = e.a === hovered || e.b === hovered;
        const eDim = colorFilter && (!colorFilter.has(e.a.color) || !colorFilter.has(e.b.color));
        let ea = focusSet ? (touches ? 1 : 0.07) : (mode === "3d" ? 0.72 : 0.92);
        if (eDim) ea = Math.min(ea, 0.05);
        ctx.globalAlpha = ea;
        ctx.strokeStyle = st.stroke; ctx.lineWidth = st.w; ctx.setLineDash(st.dash);
        const ax = e.a.sx, ay = e.a.sy, bx = e.b.sx, by = e.b.sy;
        ctx.beginPath();
        if (mode === "2d") {
          const mx = (ax + bx) / 2, my = (ay + by) / 2, dx = bx - ax, dy = by - ay;
          const nx = -dy, ny = dx, L = Math.hypot(dx, dy) || 1;
          const cxp = mx + (nx / L) * L * 0.12, cyp = my + (ny / L) * L * 0.12;
          ctx.moveTo(ax, ay); ctx.quadraticCurveTo(cxp, cyp, bx, by); ctx.stroke();
          if (st.dir) arrow(bx, by, cxp, cyp, e.b, st.stroke);
        } else {
          ctx.moveTo(ax, ay); ctx.lineTo(bx, by); ctx.stroke();
        }
      }
      ctx.setLineDash([]); ctx.globalAlpha = 1;

      // nodes
      const additive = T.dark;
      for (const n of order) {
        const r = radius(n) * (mode === "3d" ? Math.max(.4, n.ss / (n.ssBase || n.ss)) : 1);
        const rr = mode === "3d" ? radius(n) * Math.max(.45, Math.min(1.7, n.ss)) : radius(n);
        const dim = (focusSet && !focusSet.has(n)) || (colorFilter && !colorFilter.has(n.color));
        const near = mode === "3d" ? Math.max(.35, Math.min(1, (n.depth + bounds3()) / (2 * bounds3()))) : 1;
        // halo
        ctx.globalAlpha = (dim ? 0.12 : 0.9) * near;
        if (additive) ctx.globalCompositeOperation = "lighter";
        const g = glow[({ green: "green", yellow: "yellow", red: "red" }[n.color]) || "ungraded"];
        const hs = rr * (additive ? 5.5 : 4.2);
        ctx.drawImage(g, n.sx - hs / 2, n.sy - hs / 2, hs, hs);
        ctx.globalCompositeOperation = "source-over";
        // core — disco plano en 2D, ESFERA sombreada en 3D
        ctx.globalAlpha = dim ? 0.25 : 1;
        const rgb = hexToRgb(colorFor(T, n.color));
        if (mode === "3d") {
          const gx = n.sx - rr * 0.34, gy = n.sy - rr * 0.4;
          const grd = ctx.createRadialGradient(gx, gy, rr * 0.08, n.sx, n.sy, rr * 1.08);
          grd.addColorStop(0, rgba(mix(rgb, WHITE, 0.62), 1));
          grd.addColorStop(0.42, rgba(rgb, 1));
          grd.addColorStop(1, rgba(mix(rgb, BLACK, 0.5), 1));
          ctx.beginPath(); ctx.arc(n.sx, n.sy, rr, 0, TAU); ctx.fillStyle = grd; ctx.fill();
          ctx.lineWidth = 1; ctx.strokeStyle = rgba(BLACK, 0.32); ctx.stroke();
        } else {
          ctx.beginPath(); ctx.arc(n.sx, n.sy, rr, 0, TAU); ctx.fillStyle = rgba(rgb, 1); ctx.fill();
          ctx.lineWidth = 1.5; ctx.strokeStyle = rgba(BLACK, .55); ctx.stroke();
        }
        if (n === hovered) { ctx.beginPath(); ctx.arc(n.sx, n.sy, rr + 5, 0, TAU); ctx.strokeStyle = colorFor(T, n.color); ctx.globalAlpha = .8; ctx.lineWidth = 2; ctx.stroke(); }
      }
      ctx.globalAlpha = 1;

      // labels
      const showAll = nodes.length <= 42;
      ctx.font = "600 11.5px ui-monospace, Consolas, monospace"; ctx.textAlign = "center";
      for (const n of order) {
        if (colorFilter && !colorFilter.has(n.color)) continue; // filtered out → no label
        const show = showAll || n === hovered || (focusSet && focusSet.has(n));
        if (!show) continue;
        const dim = focusSet && !focusSet.has(n);
        ctx.globalAlpha = dim ? 0.3 : 0.92;
        const label = n.id.length > 22 ? n.id.slice(0, 21) + "…" : n.id;
        ctx.fillStyle = T.text;
        ctx.fillText(label, n.sx, n.sy - radius(n) - 8);
      }
      ctx.globalAlpha = 1;
    }

    function arrow(bx, by, cx, cy, target, color) {
      const r = radius(target) + 3;
      let dx = bx - cx, dy = by - cy, L = Math.hypot(dx, dy) || 1; dx /= L; dy /= L;
      const tx = bx - dx * r, ty = by - dy * r, s = 8.5;
      ctx.setLineDash([]); ctx.fillStyle = color; ctx.globalAlpha = ctx.globalAlpha;
      ctx.beginPath();
      ctx.moveTo(tx, ty);
      ctx.lineTo(tx - dx * s - dy * s * .6, ty - dy * s + dx * s * .6);
      ctx.lineTo(tx - dx * s + dy * s * .6, ty - dy * s - dx * s * .6);
      ctx.closePath(); ctx.fill();
    }

    // --- loop ---
    let raf = 0;
    function frame() {
      if (!canvas.isConnected) { stop(); return; }
      step();
      if (mode === "3d" && !dragging) { yaw += spinY + autoSpin; pitch += spinX; spinY *= 0.94; spinX *= 0.94; pitch = Math.max(-1.4, Math.min(1.4, pitch)); }
      draw();
      raf = requestAnimationFrame(frame);
    }
    function stop() { if (raf) cancelAnimationFrame(raf); raf = 0; ro.disconnect(); window.removeEventListener("cogo-theme", onTheme); }
    raf = requestAnimationFrame(frame);

    // --- interaction ---
    function hit(mx, my) {
      let best = null, bd = 22 * 22;
      for (const n of nodes) { const dx = n.sx - mx, dy = n.sy - my, d = dx * dx + dy * dy; if (d < bd) { bd = d; best = n; } }
      return best;
    }
    canvas.addEventListener("mousedown", e => { dragging = true; dragMoved = false; lastX = e.offsetX; lastY = e.offsetY; });
    window.addEventListener("mouseup", () => { dragging = false; });
    canvas.addEventListener("mousemove", e => {
      const mx = e.offsetX, my = e.offsetY;
      if (dragging) {
        const dx = mx - lastX, dy = my - lastY; lastX = mx; lastY = my;
        if (Math.abs(dx) + Math.abs(dy) > 2) dragMoved = true;
        if (mode === "3d") { yaw += dx * 0.008; pitch += dy * 0.008; spinY = dx * 0.008; spinX = dy * 0.008; }
        else { panX += dx; panY += dy; }
        return;
      }
      const n = hit(mx, my);
      if (n !== hovered) { hovered = n; canvas.style.cursor = n ? "pointer" : "grab"; }
      if (n) {
        tip.classList.remove("hidden");
        tip.innerHTML = '<span class="gt-id">' + esc(n.id) + '</span><span class="gt-type">' + esc(n.type) + (n.project ? " · " + esc(n.project) : "") + '</span>' + (n.claim ? '<span class="gt-claim">' + esc(n.claim) + '</span>' : "");
        const tw = tip.offsetWidth, th = tip.offsetHeight;
        tip.style.left = Math.min(W - tw - 8, Math.max(8, mx + 14)) + "px";
        tip.style.top = Math.max(8, my - th - 12) + "px";
      } else tip.classList.add("hidden");
    });
    canvas.addEventListener("mouseleave", () => { hovered = null; tip.classList.add("hidden"); });
    canvas.addEventListener("click", e => { if (dragMoved) return; const n = hit(e.offsetX, e.offsetY); if (n && opts.onSelect) opts.onSelect(n.id); });
    canvas.addEventListener("wheel", e => { e.preventDefault(); zoom = Math.max(0.3, Math.min(4, zoom * (e.deltaY < 0 ? 1.12 : 0.89))); }, { passive: false });
    function onTheme() { T = readTokens(); buildGlow(); }
    window.addEventListener("cogo-theme", onTheme);

    function esc(s) { return (s || "").replace(/[&<>"]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c])); }

    return {
      setMode(m) { mode = m === "3d" ? "3d" : "2d"; zoom = 1; panX = panY = 0; alpha = Math.max(alpha, 0.5); },
      setColorFilter(set) { colorFilter = (set && set.size) ? new Set(set) : null; },
      resetView() { zoom = 1; panX = panY = 0; yaw = 0.5; pitch = -0.35; spinY = spinX = 0; alpha = 1; },
      reheat() { alpha = 1; },
      destroy: stop,
    };
  }

  window.CogoGraph = { mount };
})();
