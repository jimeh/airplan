(() => {
  function at(n) {
    return n === "system" || n === "light" || n === "dark";
  }
  function fe(n, m) {
    try {
      return n?.getItem(m) ?? null;
    } catch {
      return null;
    }
  }
  function Y(n, m, w) {
    try {
      if (w === null)
        n?.removeItem(m);
      else
        n?.setItem(m, w);
    } catch {}
  }
  function He(n, m, w) {
    let y = fe(w, "airplan-color-mode");
    if (y === null) {
      let T = fe(w, "airplan-theme");
      if (y = T === "light" || T === "dark" ? T : "system", y !== "system")
        Y(w, "airplan-color-mode", y);
    }
    let S = at(y) ? y : "system", L = new Set(n.themes.map((T) => T.id)), h = fe(w, "airplan-light-theme"), g = fe(w, "airplan-dark-theme"), d = h !== null && L.has(h) ? h : n.defaultLight, C = g !== null && L.has(g) ? g : n.defaultDark;
    return Le(n, S, d, C, m);
  }
  function Le(n, m, w, y, S) {
    let L = new Map(n.themes.map((U) => [U.id, U])), h = L.has(w) ? w : n.defaultLight, g = L.has(y) ? y : n.defaultDark, d = m === "system" ? S ? "dark" : "light" : m, C = d === "light" ? h : g, T = L.get(C)?.variant ?? d;
    return { mode: m, resolvedMode: d, lightTheme: h, darkTheme: g, theme: C, variant: T };
  }
  function Ne(n, m) {
    if (m === "system")
      Y(n, "airplan-color-mode", null), Y(n, "airplan-theme", null);
    else
      Y(n, "airplan-color-mode", m), Y(n, "airplan-theme", m);
  }
  function qe(n, m, w) {
    Y(n, m === "light" ? "airplan-light-theme" : "airplan-dark-theme", w);
  }
  function De(n) {
    return {
      mode: n.mode,
      resolvedMode: n.resolvedMode,
      theme: n.theme,
      variant: n.variant
    };
  }

  (function() {
    let n = document, m = n.documentElement;
    n.querySelectorAll(".js-only").forEach((s) => {
      s.hidden = !1;
    });
    let w = window.__AIRPLAN_THEME_CATALOG__;
    if (!w)
      return;
    let y = w, S = window.matchMedia("(prefers-color-scheme: dark)"), L;
    try {
      L = window.localStorage;
    } catch {}
    let h = window.__airplanThemeState ?? He(y, S.matches, L), g = n.querySelector("[data-airplan-appearance-trigger]"), d = n.querySelector("[data-airplan-appearance-panel]"), C = n.querySelector('select[data-airplan-theme-slot="light"]'), T = n.querySelector('select[data-airplan-theme-slot="dark"]'), U = Array.from(n.querySelectorAll("[data-airplan-color-mode]"));
    if (d)
      n.body.appendChild(d);
    function Q(s) {
      if (!s || s.options.length > 0)
        return;
      for (let [f, x] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let k = n.createElement("optgroup");
        k.label = x;
        for (let _ of y.themes) {
          if (_.variant !== f)
            continue;
          let R = n.createElement("option");
          R.value = _.id, R.textContent = _.name, k.append(R);
        }
        if (k.children.length > 0)
          s.append(k);
      }
    }
    Q(C), Q(T);
    function P(s, f = !0) {
      if (h = s, window.__airplanThemeState = h, m.dataset.airplanMode = h.mode, m.dataset.airplanResolvedMode = h.resolvedMode, m.dataset.airplanTheme = h.theme, m.dataset.airplanThemeVariant = h.variant, U.forEach((x) => {
        let k = x.dataset.airplanColorMode === h.mode;
        x.classList.toggle("active", k), x.setAttribute("aria-pressed", String(k));
      }), C)
        C.value = h.lightTheme;
      if (T)
        T.value = h.darkTheme;
      if (f)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: De(h) }));
    }
    function V(s = {}) {
      P(Le(y, s.mode ?? h.mode, s.lightTheme ?? h.lightTheme, s.darkTheme ?? h.darkTheme, S.matches));
    }
    function X(s, f = !1) {
      if (!d || !g)
        return;
      if (s)
        ee();
      if (d.hidden = !s, g.setAttribute("aria-expanded", String(s)), s)
        d.querySelector("button,select")?.focus();
      else if (f)
        g.focus();
    }
    function ee() {
      if (!d || !g)
        return;
      let s = g.getBoundingClientRect(), f = g.closest(".toolbar")?.getBoundingClientRect(), x = n.documentElement.clientWidth, k = Math.min(304, x - 32), _ = Math.max(16, x - s.right);
      d.style.setProperty("--airplan-appearance-top", `${(f?.bottom ?? s.bottom) + 8}px`), d.style.setProperty("--airplan-appearance-right", `${Math.min(_, Math.max(16, x - k - 16))}px`);
    }
    g?.addEventListener("click", () => X(Boolean(d?.hidden ?? !0))), U.forEach((s) => s.addEventListener("click", () => {
      let f = s.dataset.airplanColorMode;
      if (!f)
        return;
      Ne(L, f), V({ mode: f });
    }));
    function ie(s, f) {
      qe(L, s, f.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: f.value } })), V(s === "light" ? { lightTheme: f.value } : { darkTheme: f.value });
    }
    C?.addEventListener("change", () => ie("light", C)), T?.addEventListener("change", () => ie("dark", T)), S.addEventListener("change", () => {
      if (h.mode === "system")
        V();
    }), n.addEventListener("keydown", (s) => {
      if (s.key === "Escape" && d && !d.hidden)
        s.preventDefault(), X(!1, !0);
    }), n.addEventListener("pointerdown", (s) => {
      if (!d || d.hidden || !g)
        return;
      let f = s.target;
      if (!(f instanceof Node) || d.contains(f) || g.contains(f))
        return;
      let k = (f instanceof Element ? f : f.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), _ = d.contains(n.activeElement) && !k;
      if (X(!1), _)
        setTimeout(() => {
          if (n.activeElement === n.body || d.contains(n.activeElement))
            g.focus();
        });
    }), window.addEventListener("resize", () => {
      if (d && !d.hidden)
        ee();
    }), window.addEventListener("scroll", () => {
      if (d && !d.hidden)
        ee();
    }), P(h, !1);
  })();

  var Ue = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', Pe = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', Ie = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>';
  var Oe = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';
  var $e = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>';

  (function() {
    var n = document, m = 262144;
    let w = n.getElementById("rendered");
    if (!w)
      return;
    let y = w;
    var S = n.querySelector('meta[name="airplan-versions"]'), L = n.querySelector('meta[name="airplan-revision-chain"]'), h = n.querySelector('meta[name="airplan-page-path"]'), g = n.querySelector('meta[name="airplan-entrypoint"]'), d = S ? new URL(S.content, window.location.href) : null, C = d ? new URL("./", d) : null, T = C ? C.pathname.split("/").filter(Boolean) : [], U = T.slice(0, -1);
    function Q(e, t) {
      if (typeof e !== "string")
        return null;
      try {
        var r = new URL(e);
        if (r.origin !== window.location.origin || r.username || r.password || r.search || r.hash)
          return null;
        var i = r.pathname.split("/").filter(Boolean);
        if (i.length !== U.length + 2 || !U.every(function(l, v) {
          return i[v] === l;
        }) || !/^[a-z2-7]{26}$/.test(i[i.length - 2]))
          return null;
        var a = i[i.length - 1];
        if (t ? a !== ".airplan-changes.diff" : !a.endsWith(".html"))
          return null;
        return r.href;
      } catch {
        return null;
      }
    }
    function P(e) {
      if (typeof e !== "string" || e === "" || e.startsWith("/") || e.includes("\\"))
        return !1;
      var t = e.split("/");
      return t.every(function(r) {
        var i = r.toLowerCase(), a = Array.from(r).some(function(l) {
          var v = l.codePointAt(0) || 0;
          return v < 32 || v === 127;
        });
        if (!r || r === "." || r === ".." || i.startsWith(".airplan-") || i === ".airplan.json" || a || /[. ]$/.test(r) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(r))
          return !1;
        return !0;
      });
    }
    function V(e, t) {
      if (!P(t))
        return null;
      var r = String(t).split("/").map(function(a) {
        return encodeURIComponent(a);
      }).join("/"), i = new URL(r, e);
      if (i.origin !== e.origin || i.username || i.password || i.search || i.hash || !i.pathname.startsWith(e.pathname))
        return null;
      return i.href;
    }
    async function X(e) {
      if (!e.ok)
        throw Error("marker request failed");
      var t = e.headers.get("content-length");
      if (t && /^\d+$/.test(t) && Number(t) > m) {
        if (e.body)
          await e.body.cancel("marker is too large");
        throw Error("marker is too large");
      }
      if (!e.body || typeof e.body.getReader !== "function")
        throw Error("bounded marker stream is unavailable");
      var r = e.body.getReader(), i = [], a = 0;
      try {
        for (;; ) {
          var l = await r.read();
          if (l.done)
            break;
          if (a += l.value.byteLength, a > m)
            throw await r.cancel("marker is too large"), Error("marker is too large");
          i.push(l.value);
        }
      } finally {
        r.releaseLock();
      }
      var v = new Uint8Array(a), o = 0;
      i.forEach(function(Z) {
        v.set(Z, o), o += Z.byteLength;
      });
      var b = new TextDecoder("utf-8", { fatal: !0 }).decode(v);
      return ee(b), JSON.parse(b);
    }
    function ee(e) {
      var t = 0;
      function r() {
        while (/\s/.test(e[t] || ""))
          t += 1;
      }
      function i() {
        if (e[t] !== '"')
          throw Error("JSON string is invalid");
        var l = t++;
        while (t < e.length) {
          var v = e[t++];
          if (v === '"')
            return JSON.parse(e.slice(l, t));
          if (v === "\\")
            t += 1;
        }
        throw Error("JSON string is incomplete");
      }
      function a() {
        if (r(), e[t] === "{") {
          t += 1, r();
          var l = new Set;
          if (e[t] === "}") {
            t += 1;
            return;
          }
          for (;; ) {
            r();
            var v = i();
            if (l.has(v))
              throw Error("JSON object has a duplicate field");
            if (l.add(v), r(), e[t++] !== ":")
              throw Error("JSON object is invalid");
            a(), r();
            var o = e[t++];
            if (o === "}")
              return;
            if (o !== ",")
              throw Error("JSON object is invalid");
          }
        }
        if (e[t] === "[") {
          if (t += 1, r(), e[t] === "]") {
            t += 1;
            return;
          }
          for (;; ) {
            a(), r();
            var o = e[t++];
            if (o === "]")
              return;
            if (o !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (e[t] === '"') {
          i();
          return;
        }
        var b = e.slice(t).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!b)
          throw Error("JSON value is invalid");
        t += b[0].length;
      }
      if (a(), r(), t !== e.length)
        throw Error("JSON has trailing content");
    }
    function ie(e, t, r, i) {
      if (!R(e))
        throw Error("marker is invalid");
      var a = e, l = t.pathname.split("/").filter(Boolean), v = l[l.length - 1] || "";
      if (a.schema !== "airplan-upload" || a.version !== 6 || a.kind !== "document" || a.directory !== v || !/^[a-z2-7]{26}$/.test(a.directory) || !ze(a.created_at) || a.format !== "md" || !Ve(a.slug) || a.entrypoint !== a.slug + ".html" || !R(a.producer) || a.producer.name !== "airplan" || typeof a.producer.version !== "string" || a.producer.version.trim() !== a.producer.version || a.producer.version === "" || !f(a.render) || !R(a.revision) || a.revision.number !== r.number || a.revision.chain_id !== i || (a.revision.number === 1 ? a.revision.previous_url !== void 0 : typeof a.revision.previous_url !== "string" || !Je(a.revision.previous_url)) || !Array.isArray(a.objects) || !Array.isArray(a.pages) || a.pages.length === 0)
        throw Error("marker identity is invalid");
      var o = V(t, a.entrypoint);
      if (o !== r.safeURL)
        throw Error("marker entrypoint is invalid");
      if (a.title !== void 0 && typeof a.title !== "string" || a.repo !== void 0 && !Ke(a.repo) || a.objects.length === 0 || a.pages.length > 100)
        throw Error("marker shape is invalid");
      var b = Fe(a), Z = new Set, K = new Set, O = new Set, W = new Map;
      if (a.pages.forEach(function(c, u) {
        if (!R(c) || !P(c.path) || Z.has(c.path) || K.has(c.path.toLowerCase()) || c.format !== "md" && c.format !== "txt" || typeof c.lang !== "string" || c.title !== void 0 && typeof c.title !== "string" || !P(c.page) || !P(c.source))
          throw Error("marker page descriptor is invalid");
        var N = s(c.path, c.format), D = c.path;
        if (u === 0) {
          if (N = a.entrypoint, D = a.slug + ".md", c.format !== a.format)
            throw Error("marker entry format is invalid");
        }
        if (c.page !== N || c.source !== D)
          throw Error("marker generated page mapping is invalid");
        var A = V(t, c.page);
        if (!A || O.has(A))
          throw Error("marker page object is invalid");
        if (!V(t, c.source))
          throw Error("marker source object is invalid");
        if (b.get(c.page) !== "page" || b.get(c.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var H = a.objects.find(function(J) {
          return J.name === c.source;
        }).content_type;
        if (c.format === "md" && H !== "text/markdown; charset=utf-8" || c.format === "txt" && H !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        Z.add(c.path), K.add(c.path.toLowerCase()), O.add(A), W.set(c.path, A);
      }), k(K))
        throw Error("marker page paths conflict");
      if (!Z.has(a.pages[0].path) || W.get(a.pages[0].path) !== o)
        throw Error("marker entry page is invalid");
      if (O.size !== a.pages.length || Array.from(b.values()).filter(function(c) {
        return c === "source";
      }).length !== a.pages.length)
        throw Error("marker page inventory is invalid");
      return W;
    }
    function s(e, t) {
      if (t !== "md")
        return e + ".html";
      var r = e.lastIndexOf("/"), i = e.lastIndexOf(".");
      return (i > r ? e.slice(0, i) : e) + ".html";
    }
    function f(e) {
      if (!R(e) || !R(e.template) || !R(e.themes) || !Number.isInteger(e.generation) || e.generation <= 0 || typeof e.indexable !== "boolean" || typeof e.no_external_assets !== "boolean" || !e.template || e.template.kind !== "builtin" && e.template.kind !== "custom" || e.mermaid_url !== void 0 && !je(e.mermaid_url) || !e.themes)
        return !1;
      if (e.template.kind === "builtin" && e.template.sha256 !== void 0 || e.template.kind === "custom" && !_(e.template.sha256))
        return !1;
      return x(e.themes.default_light) && x(e.themes.default_dark) && _(e.themes.catalog_sha256);
    }
    function x(e) {
      return typeof e === "string" && new TextEncoder().encode(e).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(e);
    }
    function k(e) {
      for (var t of e) {
        var r = t.indexOf("/");
        while (r >= 0) {
          if (e.has(t.slice(0, r)))
            return !0;
          r = t.indexOf("/", r + 1);
        }
      }
      return !1;
    }
    function _(e) {
      return typeof e === "string" && /^[0-9a-f]{64}$/.test(e);
    }
    function R(e) {
      return !!e && typeof e === "object" && !Array.isArray(e);
    }
    function Ve(e) {
      return typeof e === "string" && e.length <= 64 && /^[a-z0-9-]+$/.test(e);
    }
    function ze(e) {
      if (typeof e !== "string")
        return !1;
      var t = e.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/);
      if (!t)
        return !1;
      var r = Number(t[1]), i = Number(t[2]), a = Number(t[3]), l = Number(t[4]), v = Number(t[5]), o = Number(t[6]), b = r % 4 === 0 && (r % 100 !== 0 || r % 400 === 0), Z = [31, b ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return i >= 1 && i <= 12 && a >= 1 && a <= Z[i - 1] && l <= 23 && v <= 59 && o <= 59;
    }
    function Ke(e) {
      if (typeof e !== "string" || e === "" || e.trim() !== e)
        return !1;
      try {
        var t = new URL(e);
        if (t.protocol !== "https:" || t.username || t.password || t.port || t.search || t.hash)
          return !1;
        var r = t.pathname.replace(/^\/+|\/+$/g, "").split("/");
        if (r.length !== 2)
          return !1;
        var i = r[0], a = r[1].replace(/\.git$/, "");
        if (!i || !a || i === "." || i === ".." || a === "." || a === ".." || /[?#@:\\]/.test(i + a))
          return !1;
        return e === "https://" + t.hostname.toLowerCase() + "/" + i + "/" + a;
      } catch {
        return !1;
      }
    }
    function We(e) {
      return typeof e === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(e);
    }
    function Je(e) {
      try {
        var t = new URL(e);
        return (t.protocol === "https:" || t.protocol === "http:") && !t.username && !t.password && !t.search && !t.hash && t.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function je(e) {
      if (typeof e !== "string")
        return !1;
      try {
        var t = new URL(e);
        return t.protocol === "https:" && !!t.host && !t.username && !t.password && !t.hash;
      } catch {
        return !1;
      }
    }
    function Fe(e) {
      var t = new Map, r = new Set, i = 0, a = 0, l = 0, v = 0;
      if (e.objects.forEach(function(o) {
        if (!R(o) || !P(o.name) && o.name !== ".airplan-changes.diff" || t.has(o.name) || r.has(o.name.toLowerCase()) || !Number.isSafeInteger(o.bytes) || o.bytes < 0 || !_(o.sha256) || !We(o.content_type))
          throw Error("marker object inventory is invalid");
        if (o.role === "page") {
          if (a += 1, o.bytes <= 0 || o.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (o.role === "source") {
          if (l += 1, o.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (o.role === "asset")
          v += 1;
        else if (o.role === "diff") {
          if (i += 1, o.name !== ".airplan-changes.diff" || o.bytes <= 0 || o.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        t.set(o.name, o.role), r.add(o.name.toLowerCase());
      }), k(r))
        throw Error("marker object paths conflict");
      if (a !== e.pages.length || l !== e.pages.length || a + v > 100 || (e.revision.number === 1 ? i !== 0 : i !== 1))
        throw Error("marker object counts are invalid");
      return t;
    }
    function Ge(e, t) {
      var r = window.location.hash;
      if (r === "#airplan-all-changes")
        return e + r;
      if (!t)
        return e;
      return t + (r && r !== "#airplan-all-changes" ? r : "");
    }
    function Ye(e) {
      var t = n.querySelector('meta[name="airplan-revision"]'), r = t ? Number(t.content) : Number(e.current_revision);
      if (!Number.isInteger(r) || r <= 0 || e.current_revision !== r || !Number.isInteger(e.latest_revision) || !Number.isInteger(e.last_assigned_revision) || !Array.isArray(e.revisions) || e.revisions.length === 0 || e.last_assigned_revision !== e.revisions.length || !/^[a-z2-7]{26}$/.test(e.chain_id) || L && L.content !== e.chain_id)
        throw Error("revision identity is invalid");
      var i = !1, a = 0, l = e.revisions.filter(function(u) {
        if (!u || !Number.isInteger(u.number) || u.number !== a + 1)
          return i = !0, !1;
        if (a = u.number, u.deleted)
          return !1;
        if (u.safeURL = Q(u.url, !1), !u.safeURL)
          return i = !0, !1;
        if (u.number > 1) {
          var N = Q(u.diff_url, !0);
          if (!N || new URL(N).pathname.replace(/[^/]+$/, "") !== new URL(u.safeURL).pathname.replace(/[^/]+$/, ""))
            return i = !0, !1;
        }
        return !0;
      });
      if (i || e.revisions[0].number !== 1 || !l.some(function(u) {
        return u.number === r;
      }))
        throw Error("revision entries are invalid");
      var v = l.find(function(u) {
        return u.number === r;
      }), o = new URL(window.location.href);
      if (o.search = "", o.hash = "", !v || !C || new URL(v.safeURL || "").pathname.replace(/[^/]+$/, "") !== C.pathname || !o.pathname.startsWith(C.pathname))
        throw Error("current revision URL is invalid");
      var b = Math.max.apply(null, l.map(function(u) {
        return u.number;
      }));
      if (b !== e.latest_revision)
        throw Error("latest is invalid");
      var Z = Array.from(n.querySelectorAll("[data-revision-controls]")), K = Array.from(n.querySelectorAll("[data-revision-heading]"));
      if (K.length === 0) {
        if (Z.length === 0)
          throw Error("revision controls are unavailable");
        var O = n.createElement("p");
        O.className = "revision-heading", O.setAttribute("data-revision-heading", ""), Z[0].appendChild(O), K.push(O);
      }
      Z.forEach(function(u) {
        u.hidden = !1;
      });
      var W = r < b, c = W ? "Revision " + r + " of " + b : "Revision " + r + " (Latest)";
      K.forEach(function(u) {
        var N = n.createElement("span");
        N.className = "revision-picker-label", N.textContent = c, N.setAttribute("aria-hidden", "true");
        var D = n.createElement("select");
        D.setAttribute("aria-label", "Document revision"), l.forEach(function(A) {
          var H = n.createElement("option");
          H.value = A.safeURL || "", H.textContent = A.number === b ? "Revision " + A.number + " (Latest)" : "Revision " + A.number + " of " + b, H.selected = A.number === r, D.appendChild(H);
        }), D.addEventListener("change", function() {
          var A = D.selectedIndex;
          if (A < 0 || A >= l.length)
            return;
          var H = l[A], J = H.safeURL || "";
          if (window.location.hash === "#airplan-all-changes") {
            window.location.assign(J + (H.number > 1 ? "#airplan-all-changes" : ""));
            return;
          }
          var tt = g ? new URL(g.content, window.location.href).href : "";
          if (!h || o.href === tt || !L) {
            window.location.assign(J);
            return;
          }
          u.setAttribute("aria-busy", "true"), D.disabled = !0;
          var _e = new URL("./", J), Be = new URL(".airplan.json", _e);
          Be.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(Be, { cache: "no-store", credentials: "same-origin" }).then(X).then(function(rt) {
            var nt = ie(rt, _e, H, L.content);
            window.location.assign(Ge(J, nt.get(h.content) || null));
          }).catch(function() {
            console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(J);
          });
        }), u.replaceChildren(N, D), u.classList.add("is-picker"), u.classList.toggle("is-stale", W);
      }), n.body.classList.toggle("airplan-stale-revision", W);
    }
    if (S) {
      var Ce = new URL(S.content, window.location.href);
      Ce.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(Ce, { cache: "no-store", credentials: "same-origin" }).then(function(e) {
        if (e.status === 404)
          return null;
        if (!e.ok)
          throw Error("metadata request failed");
        return e.json();
      }).then(function(e) {
        if (e === null)
          return;
        if (!e || e.schema !== "airplan-versions" || e.version !== 1 || !Array.isArray(e.revisions) || e.revisions.length < 2)
          throw Error("metadata is invalid");
        Ye(e), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: e
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var oe = n.createElement("div");
    oe.className = "sr-status", oe.setAttribute("aria-live", "polite"), n.body.appendChild(oe);
    var j = null;
    function Qe() {
      if (j !== null)
        return;
      j = Array.from(n.querySelectorAll("details:not([open])")), j.forEach(function(e) {
        e.open = !0;
      });
    }
    function Xe() {
      if (j === null)
        return;
      j.forEach(function(e) {
        e.open = !1;
      }), j = null;
    }
    window.addEventListener("beforeprint", Qe), window.addEventListener("afterprint", Xe);
    function me(e, t, r) {
      oe.textContent = t;
      var i = e.querySelector(".action-label"), a = i ? i.textContent : "";
      if (i)
        i.textContent = r ? "Copied" : "Failed";
      e.classList.add(r ? "is-copied" : "is-failed"), e.disabled = !0, setTimeout(function() {
        if (e.classList.remove("is-copied", "is-failed"), e.disabled = !1, i)
          i.textContent = a;
      }, 1200);
    }
    function Te(e, t) {
      if (!navigator.clipboard) {
        me(t, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(e).then(function() {
        me(t, "Copied!", !0);
      }, function() {
        me(t, "Copy failed", !1);
      });
    }
    var ke = n.getElementById("pages"), B = n.querySelector(".pages-trigger"), E = null, ve = window.matchMedia("(max-width: 78rem)"), q = function() {};
    function pe() {
      return E ? E.matches(":popover-open") : !1;
    }
    function te(e) {
      if (!E || !pe())
        return;
      if (E.hidePopover(), e && B && ve.matches)
        setTimeout(function() {
          B.focus();
        }, 0);
    }
    if (ke && B) {
      var Ae = ke.querySelector(".pages-list");
      if (Ae) {
        var ge = n.createElement("div");
        if ("popover" in ge && typeof ge.showPopover === "function") {
          let e = function() {
            if (!B || !E)
              return;
            var t = B.getBoundingClientRect(), r = B.closest(".toolbar"), i = r ? r.getBoundingClientRect().bottom : t.bottom;
            E.style.setProperty("--pages-left", Math.max(16, t.left) + "px"), E.style.setProperty("--pages-top", i + "px"), E.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, t.left) - 16) + "px");
          };
          E = ge, E.className = "pages-popover", E.id = "pages-popover", E.setAttribute("popover", "auto");
          var re = n.createElement("nav");
          re.className = "pages-popover-nav", re.setAttribute("aria-label", "Pages"), re.appendChild(Ae.cloneNode(!0)), E.appendChild(re), B.setAttribute("popovertarget", E.id), B.popoverTargetElement = E, E.addEventListener("beforetoggle", function(t) {
            if (t.newState !== "open")
              return;
            q(), e();
          }), E.addEventListener("toggle", function(t) {
            var r = t.newState === "open";
            if (B.setAttribute("aria-expanded", r ? "true" : "false"), n.body.classList.toggle("pages-popover-open", r), r) {
              var i = E.querySelector('[aria-current="page"]');
              if (i)
                i.scrollIntoView({ block: "nearest" });
            }
            z();
          }), re.querySelectorAll("a").forEach(function(t) {
            t.addEventListener("click", function() {
              te(!1);
            });
          }), ve.addEventListener("change", function() {
            if (!ve.matches)
              te(!1);
          }), window.addEventListener("resize", function() {
            if (pe())
              e();
          }), B.hidden = !1, B.setAttribute("aria-expanded", "false"), n.body.appendChild(E), n.body.classList.add("pages-popover-ready");
        }
      }
    }
    var F = n.getElementById("source"), se = n.getElementById("changes"), le = n.querySelector("[data-airplan-all-changes]"), I = n.getElementById("toc"), M = null, p = null, xe = window.matchMedia("(max-width: 78rem)");
    q = function() {
      if (p && p.open)
        p.close();
    };
    function z() {
      if (!I || !M || !p)
        return;
      var e = xe.matches && !y.hidden && !p.open && !pe();
      if (M.classList.toggle("is-visible", e), M.tabIndex = e ? 0 : -1, M.setAttribute("aria-hidden", e ? "false" : "true"), p.open && (!xe.matches || y.hidden))
        q();
    }
    function Se(e) {
      if (te(!1), q(), y.hidden = e !== "rendered", F)
        F.hidden = e !== "source";
      if (se)
        se.hidden = e !== "changes";
      if (I)
        I.hidden = e !== "rendered";
      n.querySelectorAll(".viewtoggle button").forEach(function(t) {
        var r = t.dataset.view === e;
        t.classList.toggle("active", r), t.setAttribute("aria-pressed", r ? "true" : "false");
      }), z();
    }
    n.querySelectorAll(".viewtoggle button").forEach(function(e) {
      e.addEventListener("click", function() {
        Se(e.dataset.view || "rendered");
      });
    });
    var we = !1;
    n.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(e) {
      e.addEventListener("click", function() {
        we = new URL(e.href).pathname === window.location.pathname;
      });
    });
    function Re() {
      var e = window.location.hash === "#airplan-all-changes" && !!le;
      if (te(!1), q(), n.body.classList.toggle("all-changes-active", e), le)
        le.hidden = !e;
      if (e) {
        if (y.hidden = !0, F)
          F.hidden = !0;
        if (se)
          se.hidden = !0;
        if (I)
          I.hidden = !0;
        if (we)
          le.querySelector("h1")?.focus();
      } else
        Se("rendered");
      we = !1, z();
    }
    if (window.addEventListener("hashchange", Re), Re(), I) {
      let e = function() {
        if (ne.length === 0) {
          z();
          return;
        }
        var r = 0;
        if (et.forEach(function(a, l) {
          if (a && a.getBoundingClientRect().top <= 128)
            r = l;
        }), window.scrollY <= 128)
          r = 0;
        else if (window.innerHeight + window.scrollY >= n.documentElement.scrollHeight - 2)
          r = ne.length - 1;
        var i = ne[r].getAttribute("href");
        ye.forEach(function(a) {
          var l = a.getAttribute("href") === i;
          if (a.classList.toggle("active", l), l)
            a.setAttribute("aria-current", "location");
          else
            a.removeAttribute("aria-current");
        }), z();
      }, t = function() {
        if (Ee)
          return;
        Ee = !0, window.requestAnimationFrame(function() {
          Ee = !1, e();
        });
      };
      var ne = Array.from(I.querySelectorAll('a[href^="#"]')), Ze = I.querySelector(".toc-list");
      if (Ze)
        if (p = n.createElement("dialog"), typeof p.showModal === "function") {
          p.className = "toc-dialog", p.id = "toc-dialog", p.setAttribute("aria-labelledby", "toc-dialog-title");
          var ce = n.createElement("div");
          ce.className = "toc-dialog-panel";
          var de = n.createElement("div");
          de.className = "toc-dialog-header";
          var ue = n.createElement("h2");
          ue.className = "toc-dialog-title", ue.id = "toc-dialog-title", ue.textContent = "Contents";
          var G = n.createElement("button");
          G.className = "toc-dialog-close", G.type = "button", G.setAttribute("aria-label", "Close table of contents"), G.innerHTML = Ue, de.appendChild(ue), de.appendChild(G);
          var ae = n.createElement("nav");
          ae.className = "toc-dialog-nav", ae.setAttribute("aria-label", "Table of contents"), ae.appendChild(Ze.cloneNode(!0)), ce.appendChild(de), ce.appendChild(ae), p.appendChild(ce), M = n.createElement("button"), M.className = "toc-trigger", M.type = "button", M.tabIndex = -1, M.setAttribute("aria-label", "Open table of contents"), M.setAttribute("aria-controls", "toc-dialog"), M.setAttribute("aria-haspopup", "dialog"), M.setAttribute("aria-hidden", "true"), M.innerHTML = $e, n.body.appendChild(M), n.body.appendChild(p), n.body.classList.add("toc-dialog-ready"), M.addEventListener("click", function() {
            te(!1), p.showModal(), n.body.classList.add("toc-dialog-open"), z();
            var r = p.querySelector("a.active");
            if (r)
              r.scrollIntoView({ block: "nearest" });
          }), G.addEventListener("click", q), p.addEventListener("click", function(r) {
            if (r.target === p)
              q();
          }), p.addEventListener("keydown", function(r) {
            if (r.key === "Escape")
              r.preventDefault(), q();
          }), p.addEventListener("close", function() {
            if (n.body.classList.remove("toc-dialog-open"), z(), M.classList.contains("is-visible"))
              setTimeout(function() {
                M.focus();
              }, 50);
          }), ae.querySelectorAll("a").forEach(function(r) {
            r.addEventListener("click", q);
          });
        } else
          p = null;
      var ye = ne.slice();
      if (p)
        ye = ye.concat(Array.from(p.querySelectorAll('a[href^="#"]')));
      var et = ne.map(function(r) {
        return n.getElementById((r.getAttribute("href") || "").slice(1));
      }), Ee = !1;
      n.addEventListener("scroll", t, { passive: !0 }), window.addEventListener("resize", e), e();
    }
    var he = n.querySelector(".top-controls");
    function Me() {
      var e = he ? he.getBoundingClientRect().height : 0;
      n.documentElement.style.setProperty("--airplan-sticky-height", e + "px");
    }
    if (he) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(Me).observe(he);
      window.addEventListener("resize", Me), Me();
    }
    let be = n.querySelector(".copy-source");
    if (be && F)
      be.addEventListener("click", function() {
        var e = F.querySelector("pre");
        Te(e ? e.textContent : "", be);
      });
    y.querySelectorAll("pre").forEach(function(e) {
      if (e.classList.contains("mermaid"))
        return;
      var t = n.createElement("div");
      t.className = "codewrap", e.parentNode?.insertBefore(t, e), t.appendChild(e);
      var r = n.createElement("button");
      r.className = "codecopy", r.type = "button", r.setAttribute("aria-label", "Copy code"), r.title = "Copy code", r.innerHTML = Ie + Pe + Oe, r.addEventListener("click", function() {
        var i = e.querySelector("code");
        Te((i || e).textContent, r);
      }), t.appendChild(r);
    });
  })();
})();
