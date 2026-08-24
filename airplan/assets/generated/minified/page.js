(() => {
  function ot(n) {
    return n === "system" || n === "light" || n === "dark";
  }
  function oe(n, u) {
    try {
      return n?.getItem(u) ?? null;
    } catch {
      return null;
    }
  }
  function F(n, u, w) {
    try {
      if (w === null)
        n?.removeItem(u);
      else
        n?.setItem(u, w);
    } catch {}
  }
  function Ne(n, u, w) {
    let y = oe(w, "airplan-color-mode");
    if (y === null) {
      let T = oe(w, "airplan-theme");
      if (y = T === "light" || T === "dark" ? T : "system", y !== "system")
        F(w, "airplan-color-mode", y);
    }
    let R = ot(y) ? y : "system", E = new Set(n.themes.map((T) => T.id)), v = oe(w, "airplan-light-theme"), g = oe(w, "airplan-dark-theme"), d = v !== null && E.has(v) ? v : n.defaultLight, C = g !== null && E.has(g) ? g : n.defaultDark;
    return Ce(n, R, d, C, u);
  }
  function Ce(n, u, w, y, R) {
    let E = new Map(n.themes.map((H) => [H.id, H])), v = E.has(w) ? w : n.defaultLight, g = E.has(y) ? y : n.defaultDark, d = u === "system" ? R ? "dark" : "light" : u, C = d === "light" ? v : g, T = E.get(C)?.variant ?? d;
    return { mode: u, resolvedMode: d, lightTheme: v, darkTheme: g, theme: C, variant: T };
  }
  function qe(n, u) {
    if (u === "system")
      F(n, "airplan-color-mode", null), F(n, "airplan-theme", null);
    else
      F(n, "airplan-color-mode", u), F(n, "airplan-theme", u);
  }
  function De(n, u, w) {
    F(n, u === "light" ? "airplan-light-theme" : "airplan-dark-theme", w);
  }
  function Ue(n) {
    return oe(n, "airplan-fixed-navbar") !== "false";
  }
  function Ie(n, u) {
    F(n, "airplan-fixed-navbar", u ? null : "false");
  }
  function Pe(n) {
    return {
      mode: n.mode,
      resolvedMode: n.resolvedMode,
      theme: n.theme,
      variant: n.variant
    };
  }

  (function() {
    let n = document, u = n.documentElement;
    n.querySelectorAll(".js-only").forEach((s) => {
      s.hidden = !1;
    });
    let w = window.__AIRPLAN_THEME_CATALOG__;
    if (!w)
      return;
    let y = w, R = window.matchMedia("(prefers-color-scheme: dark)"), E;
    try {
      E = window.localStorage;
    } catch {}
    let v = window.__airplanThemeState ?? Ne(y, R.matches, E), g = n.querySelector("[data-airplan-appearance-trigger]"), d = n.querySelector("[data-airplan-appearance-panel]"), C = n.querySelector('select[data-airplan-theme-slot="light"]'), T = n.querySelector('select[data-airplan-theme-slot="dark"]'), H = n.querySelector("input[data-airplan-fixed-navbar]"), X = Array.from(n.querySelectorAll("[data-airplan-color-mode]"));
    if (d)
      n.body.appendChild(d);
    function U(s) {
      if (!s || s.options.length > 0)
        return;
      for (let [h, S] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let k = n.createElement("optgroup");
        k.label = S;
        for (let x of y.themes) {
          if (x.variant !== h)
            continue;
          let O = n.createElement("option");
          O.value = x.id, O.textContent = x.name, k.append(O);
        }
        if (k.children.length > 0)
          s.append(k);
      }
    }
    U(C), U(T);
    function J(s, h = !0) {
      if (v = s, window.__airplanThemeState = v, u.dataset.airplanMode = v.mode, u.dataset.airplanResolvedMode = v.resolvedMode, u.dataset.airplanTheme = v.theme, u.dataset.airplanThemeVariant = v.variant, X.forEach((S) => {
        let k = S.dataset.airplanColorMode === v.mode;
        S.classList.toggle("active", k), S.setAttribute("aria-pressed", String(k));
      }), C)
        C.value = v.lightTheme;
      if (T)
        T.value = v.darkTheme;
      if (h)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: Pe(v) }));
    }
    function ee(s = {}) {
      J(Ce(y, s.mode ?? v.mode, s.lightTheme ?? v.lightTheme, s.darkTheme ?? v.darkTheme, R.matches));
    }
    function te(s, h = !1) {
      if (!d || !g)
        return;
      if (s)
        j();
      if (d.hidden = !s, g.setAttribute("aria-expanded", String(s)), s)
        d.querySelector("button,select,input")?.focus();
      else if (h)
        g.focus();
    }
    function j() {
      if (!d || !g)
        return;
      let s = g.getBoundingClientRect(), h = g.closest(".toolbar")?.getBoundingClientRect(), S = n.documentElement.clientWidth, k = Math.min(304, S - 32), x = Math.max(16, S - s.right), O = (h?.bottom ?? s.bottom) + 8;
      d.style.setProperty("--airplan-appearance-top", `${Math.max(16, O)}px`), d.style.setProperty("--airplan-appearance-right", `${Math.min(x, Math.max(16, S - k - 16))}px`);
    }
    g?.addEventListener("click", () => te(Boolean(d?.hidden ?? !0))), X.forEach((s) => s.addEventListener("click", () => {
      let h = s.dataset.airplanColorMode;
      if (!h)
        return;
      qe(E, h), ee({ mode: h });
    }));
    function se(s, h) {
      De(E, s, h.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: h.value } })), ee(s === "light" ? { lightTheme: h.value } : { darkTheme: h.value });
    }
    if (C?.addEventListener("change", () => se("light", C)), T?.addEventListener("change", () => se("dark", T)), H?.addEventListener("change", () => {
      let s = H.checked;
      Ie(E, s), u.dataset.airplanFixedNavbar = String(s), window.dispatchEvent(new CustomEvent("airplan:navbarchange", { detail: { fixed: s } })), j();
    }), R.addEventListener("change", () => {
      if (v.mode === "system")
        ee();
    }), n.addEventListener("keydown", (s) => {
      if (s.key === "Escape" && d && !d.hidden)
        s.preventDefault(), te(!1, !0);
    }), n.addEventListener("pointerdown", (s) => {
      if (!d || d.hidden || !g)
        return;
      let h = s.target;
      if (!(h instanceof Node) || d.contains(h) || g.contains(h))
        return;
      let k = (h instanceof Element ? h : h.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), x = d.contains(n.activeElement) && !k;
      if (te(!1), x)
        setTimeout(() => {
          if (n.activeElement === n.body || d.contains(n.activeElement))
            g.focus();
        });
    }), window.addEventListener("resize", () => {
      if (d && !d.hidden)
        j();
    }), window.addEventListener("scroll", () => {
      if (d && !d.hidden)
        j();
    }), H)
      H.checked = Ue(E);
    J(v, !1);
  })();

  var Oe = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', $e = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', Ve = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>';
  var ze = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';
  var Ke = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>';

  (function() {
    var n = document, u = 262144;
    let w = n.getElementById("rendered");
    if (!w)
      return;
    let y = w;
    var R = n.querySelector('meta[name="airplan-versions"]'), E = n.querySelector('meta[name="airplan-revision-chain"]'), v = n.querySelector('meta[name="airplan-page-path"]'), g = n.querySelector('meta[name="airplan-entrypoint"]'), d = R ? new URL(R.content, window.location.href) : null, C = d ? new URL("./", d) : null, T = C ? C.pathname.split("/").filter(Boolean) : [], H = T.slice(0, -1);
    function X(e, t) {
      if (typeof e !== "string")
        return null;
      try {
        var r = new URL(e);
        if (r.origin !== window.location.origin || r.username || r.password || r.search || r.hash)
          return null;
        var i = r.pathname.split("/").filter(Boolean);
        if (i.length !== H.length + 2 || !H.every(function(l, m) {
          return i[m] === l;
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
    function U(e) {
      if (typeof e !== "string" || e === "" || e.startsWith("/") || e.includes("\\"))
        return !1;
      var t = e.split("/");
      return t.every(function(r) {
        var i = r.toLowerCase(), a = Array.from(r).some(function(l) {
          var m = l.codePointAt(0) || 0;
          return m < 32 || m === 127;
        });
        if (!r || r === "." || r === ".." || i.startsWith(".airplan-") || i === ".airplan.json" || a || /[. ]$/.test(r) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(r))
          return !1;
        return !0;
      });
    }
    function J(e, t) {
      if (!U(t))
        return null;
      var r = String(t).split("/").map(function(a) {
        return encodeURIComponent(a);
      }).join("/"), i = new URL(r, e);
      if (i.origin !== e.origin || i.username || i.password || i.search || i.hash || !i.pathname.startsWith(e.pathname))
        return null;
      return i.href;
    }
    async function ee(e) {
      if (!e.ok)
        throw Error("marker request failed");
      var t = e.headers.get("content-length");
      if (t && /^\d+$/.test(t) && Number(t) > u) {
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
          if (a += l.value.byteLength, a > u)
            throw await r.cancel("marker is too large"), Error("marker is too large");
          i.push(l.value);
        }
      } finally {
        r.releaseLock();
      }
      var m = new Uint8Array(a), o = 0;
      i.forEach(function(Z) {
        m.set(Z, o), o += Z.byteLength;
      });
      var L = new TextDecoder("utf-8", { fatal: !0 }).decode(m);
      return te(L), JSON.parse(L);
    }
    function te(e) {
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
          var m = e[t++];
          if (m === '"')
            return JSON.parse(e.slice(l, t));
          if (m === "\\")
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
            var m = i();
            if (l.has(m))
              throw Error("JSON object has a duplicate field");
            if (l.add(m), r(), e[t++] !== ":")
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
        var L = e.slice(t).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!L)
          throw Error("JSON value is invalid");
        t += L[0].length;
      }
      if (a(), r(), t !== e.length)
        throw Error("JSON has trailing content");
    }
    function j(e, t, r, i) {
      if (!x(e))
        throw Error("marker is invalid");
      var a = e, l = t.pathname.split("/").filter(Boolean), m = l[l.length - 1] || "";
      if (a.schema !== "airplan-upload" || a.version !== 6 || a.kind !== "document" || a.directory !== m || !/^[a-z2-7]{26}$/.test(a.directory) || !We(a.created_at) || a.format !== "md" || !O(a.slug) || a.entrypoint !== a.slug + ".html" || !x(a.producer) || a.producer.name !== "airplan" || typeof a.producer.version !== "string" || a.producer.version.trim() !== a.producer.version || a.producer.version === "" || !s(a.render) || !x(a.revision) || a.revision.number !== r.number || a.revision.chain_id !== i || (a.revision.number === 1 ? a.revision.previous_url !== void 0 : typeof a.revision.previous_url !== "string" || !je(a.revision.previous_url)) || !Array.isArray(a.objects) || !Array.isArray(a.pages) || a.pages.length === 0)
        throw Error("marker identity is invalid");
      var o = J(t, a.entrypoint);
      if (o !== r.safeURL)
        throw Error("marker entrypoint is invalid");
      if (a.title !== void 0 && typeof a.title !== "string" || a.repo !== void 0 && !Fe(a.repo) || a.objects.length === 0 || a.pages.length > 100)
        throw Error("marker shape is invalid");
      var L = Ye(a), Z = new Set, z = new Set, P = new Set, K = new Map;
      if (a.pages.forEach(function(c, f) {
        if (!x(c) || !U(c.path) || Z.has(c.path) || z.has(c.path.toLowerCase()) || c.format !== "md" && c.format !== "txt" || typeof c.lang !== "string" || c.title !== void 0 && typeof c.title !== "string" || !U(c.page) || !U(c.source))
          throw Error("marker page descriptor is invalid");
        var N = se(c.path, c.format), D = c.path;
        if (f === 0) {
          if (N = a.entrypoint, D = a.slug + ".md", c.format !== a.format)
            throw Error("marker entry format is invalid");
        }
        if (c.page !== N || c.source !== D)
          throw Error("marker generated page mapping is invalid");
        var A = J(t, c.page);
        if (!A || P.has(A))
          throw Error("marker page object is invalid");
        if (!J(t, c.source))
          throw Error("marker source object is invalid");
        if (L.get(c.page) !== "page" || L.get(c.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var B = a.objects.find(function(W) {
          return W.name === c.source;
        }).content_type;
        if (c.format === "md" && B !== "text/markdown; charset=utf-8" || c.format === "txt" && B !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        Z.add(c.path), z.add(c.path.toLowerCase()), P.add(A), K.set(c.path, A);
      }), S(z))
        throw Error("marker page paths conflict");
      if (!Z.has(a.pages[0].path) || K.get(a.pages[0].path) !== o)
        throw Error("marker entry page is invalid");
      if (P.size !== a.pages.length || Array.from(L.values()).filter(function(c) {
        return c === "source";
      }).length !== a.pages.length)
        throw Error("marker page inventory is invalid");
      return K;
    }
    function se(e, t) {
      if (t !== "md")
        return e + ".html";
      var r = e.lastIndexOf("/"), i = e.lastIndexOf(".");
      return (i > r ? e.slice(0, i) : e) + ".html";
    }
    function s(e) {
      if (!x(e) || !x(e.template) || !x(e.themes) || !Number.isInteger(e.generation) || e.generation <= 0 || typeof e.indexable !== "boolean" || typeof e.no_external_assets !== "boolean" || !e.template || e.template.kind !== "builtin" && e.template.kind !== "custom" || e.mermaid_url !== void 0 && !Ge(e.mermaid_url) || !e.themes)
        return !1;
      if (e.template.kind === "builtin" && e.template.sha256 !== void 0 || e.template.kind === "custom" && !k(e.template.sha256))
        return !1;
      return h(e.themes.default_light) && h(e.themes.default_dark) && k(e.themes.catalog_sha256);
    }
    function h(e) {
      return typeof e === "string" && new TextEncoder().encode(e).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(e);
    }
    function S(e) {
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
    function k(e) {
      return typeof e === "string" && /^[0-9a-f]{64}$/.test(e);
    }
    function x(e) {
      return !!e && typeof e === "object" && !Array.isArray(e);
    }
    function O(e) {
      return typeof e === "string" && e.length <= 64 && /^[a-z0-9-]+$/.test(e);
    }
    function We(e) {
      if (typeof e !== "string")
        return !1;
      var t = e.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/);
      if (!t)
        return !1;
      var r = Number(t[1]), i = Number(t[2]), a = Number(t[3]), l = Number(t[4]), m = Number(t[5]), o = Number(t[6]), L = r % 4 === 0 && (r % 100 !== 0 || r % 400 === 0), Z = [31, L ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return i >= 1 && i <= 12 && a >= 1 && a <= Z[i - 1] && l <= 23 && m <= 59 && o <= 59;
    }
    function Fe(e) {
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
    function Je(e) {
      return typeof e === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(e);
    }
    function je(e) {
      try {
        var t = new URL(e);
        return (t.protocol === "https:" || t.protocol === "http:") && !t.username && !t.password && !t.search && !t.hash && t.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function Ge(e) {
      if (typeof e !== "string")
        return !1;
      try {
        var t = new URL(e);
        return t.protocol === "https:" && !!t.host && !t.username && !t.password && !t.hash;
      } catch {
        return !1;
      }
    }
    function Ye(e) {
      var t = new Map, r = new Set, i = 0, a = 0, l = 0, m = 0;
      if (e.objects.forEach(function(o) {
        if (!x(o) || !U(o.name) && o.name !== ".airplan-changes.diff" || t.has(o.name) || r.has(o.name.toLowerCase()) || !Number.isSafeInteger(o.bytes) || o.bytes < 0 || !k(o.sha256) || !Je(o.content_type))
          throw Error("marker object inventory is invalid");
        if (o.role === "page") {
          if (a += 1, o.bytes <= 0 || o.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (o.role === "source") {
          if (l += 1, o.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (o.role === "asset")
          m += 1;
        else if (o.role === "diff") {
          if (i += 1, o.name !== ".airplan-changes.diff" || o.bytes <= 0 || o.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        t.set(o.name, o.role), r.add(o.name.toLowerCase());
      }), S(r))
        throw Error("marker object paths conflict");
      if (a !== e.pages.length || l !== e.pages.length || a + m > 100 || (e.revision.number === 1 ? i !== 0 : i !== 1))
        throw Error("marker object counts are invalid");
      return t;
    }
    function Qe(e, t) {
      var r = window.location.hash;
      if (r === "#airplan-all-changes")
        return e + r;
      if (!t)
        return e;
      return t + (r && r !== "#airplan-all-changes" ? r : "");
    }
    function Xe(e) {
      var t = n.querySelector('meta[name="airplan-revision"]'), r = t ? Number(t.content) : Number(e.current_revision);
      if (!Number.isInteger(r) || r <= 0 || e.current_revision !== r || !Number.isInteger(e.latest_revision) || !Number.isInteger(e.last_assigned_revision) || !Array.isArray(e.revisions) || e.revisions.length === 0 || e.last_assigned_revision !== e.revisions.length || !/^[a-z2-7]{26}$/.test(e.chain_id) || E && E.content !== e.chain_id)
        throw Error("revision identity is invalid");
      var i = !1, a = 0, l = e.revisions.filter(function(f) {
        if (!f || !Number.isInteger(f.number) || f.number !== a + 1)
          return i = !0, !1;
        if (a = f.number, f.deleted)
          return !1;
        if (f.safeURL = X(f.url, !1), !f.safeURL)
          return i = !0, !1;
        if (f.number > 1) {
          var N = X(f.diff_url, !0);
          if (!N || new URL(N).pathname.replace(/[^/]+$/, "") !== new URL(f.safeURL).pathname.replace(/[^/]+$/, ""))
            return i = !0, !1;
        }
        return !0;
      });
      if (i || e.revisions[0].number !== 1 || !l.some(function(f) {
        return f.number === r;
      }))
        throw Error("revision entries are invalid");
      var m = l.find(function(f) {
        return f.number === r;
      }), o = new URL(window.location.href);
      if (o.search = "", o.hash = "", !m || !C || new URL(m.safeURL || "").pathname.replace(/[^/]+$/, "") !== C.pathname || !o.pathname.startsWith(C.pathname))
        throw Error("current revision URL is invalid");
      var L = Math.max.apply(null, l.map(function(f) {
        return f.number;
      }));
      if (L !== e.latest_revision)
        throw Error("latest is invalid");
      var Z = Array.from(n.querySelectorAll("[data-revision-controls]")), z = Array.from(n.querySelectorAll("[data-revision-heading]"));
      if (z.length === 0) {
        if (Z.length === 0)
          throw Error("revision controls are unavailable");
        var P = n.createElement("p");
        P.className = "revision-heading", P.setAttribute("data-revision-heading", ""), Z[0].appendChild(P), z.push(P);
      }
      Z.forEach(function(f) {
        f.hidden = !1;
      });
      var K = r < L, c = K ? "Revision " + r + " of " + L : "Revision " + r + " (Latest)";
      z.forEach(function(f) {
        var N = n.createElement("span");
        N.className = "revision-picker-label", N.textContent = c, N.setAttribute("aria-hidden", "true");
        var D = n.createElement("select");
        D.setAttribute("aria-label", "Document revision"), l.forEach(function(A) {
          var B = n.createElement("option");
          B.value = A.safeURL || "", B.textContent = A.number === L ? "Revision " + A.number + " (Latest)" : "Revision " + A.number + " of " + L, B.selected = A.number === r, D.appendChild(B);
        }), D.addEventListener("change", function() {
          var A = D.selectedIndex;
          if (A < 0 || A >= l.length)
            return;
          var B = l[A], W = B.safeURL || "";
          if (window.location.hash === "#airplan-all-changes") {
            window.location.assign(W + (B.number > 1 ? "#airplan-all-changes" : ""));
            return;
          }
          var nt = g ? new URL(g.content, window.location.href).href : "";
          if (!v || o.href === nt || !E) {
            window.location.assign(W);
            return;
          }
          f.setAttribute("aria-busy", "true"), D.disabled = !0;
          var Be = new URL("./", W), He = new URL(".airplan.json", Be);
          He.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(He, { cache: "no-store", credentials: "same-origin" }).then(ee).then(function(at) {
            var it = j(at, Be, B, E.content);
            window.location.assign(Qe(W, it.get(v.content) || null));
          }).catch(function() {
            console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(W);
          });
        }), f.replaceChildren(N, D), f.classList.add("is-picker"), f.classList.toggle("is-stale", K);
      }), n.body.classList.toggle("airplan-stale-revision", K);
    }
    if (R) {
      var xe = new URL(R.content, window.location.href);
      xe.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(xe, { cache: "no-store", credentials: "same-origin" }).then(function(e) {
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
        Xe(e), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: e
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var le = n.createElement("div");
    le.className = "sr-status", le.setAttribute("aria-live", "polite"), n.body.appendChild(le);
    var G = null;
    function et() {
      if (G !== null)
        return;
      G = Array.from(n.querySelectorAll("details:not([open])")), G.forEach(function(e) {
        e.open = !0;
      });
    }
    function tt() {
      if (G === null)
        return;
      G.forEach(function(e) {
        e.open = !1;
      }), G = null;
    }
    window.addEventListener("beforeprint", et), window.addEventListener("afterprint", tt);
    function pe(e, t, r) {
      le.textContent = t;
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
        pe(t, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(e).then(function() {
        pe(t, "Copied!", !0);
      }, function() {
        pe(t, "Copy failed", !1);
      });
    }
    var ke = n.getElementById("pages"), _ = n.querySelector(".pages-trigger"), b = null, ge = window.matchMedia("(max-width: 78rem)"), q = function() {};
    function we() {
      return b ? b.matches(":popover-open") : !1;
    }
    function re(e) {
      if (!b || !we())
        return;
      if (b.hidePopover(), e && _ && ge.matches)
        setTimeout(function() {
          _.focus();
        }, 0);
    }
    if (ke && _) {
      var Ae = ke.querySelector(".pages-list");
      if (Ae) {
        var ye = n.createElement("div");
        if ("popover" in ye && typeof ye.showPopover === "function") {
          let e = function() {
            if (!_ || !b)
              return;
            var t = _.getBoundingClientRect(), r = _.closest(".toolbar"), i = r ? r.getBoundingClientRect().bottom : t.bottom;
            b.style.setProperty("--pages-left", Math.max(16, t.left) + "px"), b.style.setProperty("--pages-top", i + "px"), b.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, t.left) - 16) + "px");
          };
          b = ye, b.className = "pages-popover", b.id = "pages-popover", b.setAttribute("popover", "auto");
          var ne = n.createElement("nav");
          ne.className = "pages-popover-nav", ne.setAttribute("aria-label", "Pages"), ne.appendChild(Ae.cloneNode(!0)), b.appendChild(ne), _.setAttribute("popovertarget", b.id), _.popoverTargetElement = b, b.addEventListener("beforetoggle", function(t) {
            if (t.newState !== "open")
              return;
            q(), e();
          }), b.addEventListener("toggle", function(t) {
            var r = t.newState === "open";
            if (_.setAttribute("aria-expanded", r ? "true" : "false"), n.body.classList.toggle("pages-popover-open", r), r) {
              var i = b.querySelector('[aria-current="page"]');
              if (i)
                i.scrollIntoView({ block: "nearest" });
            }
            V();
          }), ne.querySelectorAll("a").forEach(function(t) {
            t.addEventListener("click", function() {
              re(!1);
            });
          }), ge.addEventListener("change", function() {
            if (!ge.matches)
              re(!1);
          }), window.addEventListener("resize", function() {
            if (we())
              e();
          }), _.hidden = !1, _.setAttribute("aria-expanded", "false"), n.body.appendChild(b), n.body.classList.add("pages-popover-ready");
        }
      }
    }
    var Y = n.getElementById("source"), ce = n.getElementById("changes"), de = n.querySelector("[data-airplan-all-changes]"), I = n.getElementById("toc"), M = null, p = null, Se = window.matchMedia("(max-width: 78rem)");
    q = function() {
      if (p && p.open)
        p.close();
    };
    function V() {
      if (!I || !M || !p)
        return;
      var e = Se.matches && !y.hidden && !p.open && !we();
      if (M.classList.toggle("is-visible", e), M.tabIndex = e ? 0 : -1, M.setAttribute("aria-hidden", e ? "false" : "true"), p.open && (!Se.matches || y.hidden))
        q();
    }
    function Re(e) {
      if (re(!1), q(), y.hidden = e !== "rendered", Y)
        Y.hidden = e !== "source";
      if (ce)
        ce.hidden = e !== "changes";
      if (I)
        I.hidden = e !== "rendered";
      n.querySelectorAll(".viewtoggle button").forEach(function(t) {
        var r = t.dataset.view === e;
        t.classList.toggle("active", r), t.setAttribute("aria-pressed", r ? "true" : "false");
      }), V();
    }
    n.querySelectorAll(".viewtoggle button").forEach(function(e) {
      e.addEventListener("click", function() {
        Re(e.dataset.view || "rendered");
      });
    });
    var be = !1;
    n.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(e) {
      e.addEventListener("click", function() {
        be = new URL(e.href).pathname === window.location.pathname;
      });
    });
    function Ze() {
      var e = window.location.hash === "#airplan-all-changes" && !!de;
      if (re(!1), q(), n.body.classList.toggle("all-changes-active", e), de)
        de.hidden = !e;
      if (e) {
        if (y.hidden = !0, Y)
          Y.hidden = !0;
        if (ce)
          ce.hidden = !0;
        if (I)
          I.hidden = !0;
        if (be)
          de.querySelector("h1")?.focus();
      } else
        Re("rendered");
      be = !1, V();
    }
    if (window.addEventListener("hashchange", Ze), Ze(), I) {
      let e = function() {
        if (ae.length === 0) {
          V();
          return;
        }
        var r = 0;
        if (rt.forEach(function(a, l) {
          if (a && a.getBoundingClientRect().top <= 128)
            r = l;
        }), window.scrollY <= 128)
          r = 0;
        else if (window.innerHeight + window.scrollY >= n.documentElement.scrollHeight - 2)
          r = ae.length - 1;
        var i = ae[r].getAttribute("href");
        Ee.forEach(function(a) {
          var l = a.getAttribute("href") === i;
          if (a.classList.toggle("active", l), l)
            a.setAttribute("aria-current", "location");
          else
            a.removeAttribute("aria-current");
        }), V();
      }, t = function() {
        if (Me)
          return;
        Me = !0, window.requestAnimationFrame(function() {
          Me = !1, e();
        });
      };
      var ae = Array.from(I.querySelectorAll('a[href^="#"]')), _e = I.querySelector(".toc-list");
      if (_e)
        if (p = n.createElement("dialog"), typeof p.showModal === "function") {
          p.className = "toc-dialog", p.id = "toc-dialog", p.setAttribute("aria-labelledby", "toc-dialog-title");
          var ue = n.createElement("div");
          ue.className = "toc-dialog-panel";
          var he = n.createElement("div");
          he.className = "toc-dialog-header";
          var fe = n.createElement("h2");
          fe.className = "toc-dialog-title", fe.id = "toc-dialog-title", fe.textContent = "Contents";
          var Q = n.createElement("button");
          Q.className = "toc-dialog-close", Q.type = "button", Q.setAttribute("aria-label", "Close table of contents"), Q.innerHTML = Oe, he.appendChild(fe), he.appendChild(Q);
          var ie = n.createElement("nav");
          ie.className = "toc-dialog-nav", ie.setAttribute("aria-label", "Table of contents"), ie.appendChild(_e.cloneNode(!0)), ue.appendChild(he), ue.appendChild(ie), p.appendChild(ue), M = n.createElement("button"), M.className = "toc-trigger", M.type = "button", M.tabIndex = -1, M.setAttribute("aria-label", "Open table of contents"), M.setAttribute("aria-controls", "toc-dialog"), M.setAttribute("aria-haspopup", "dialog"), M.setAttribute("aria-hidden", "true"), M.innerHTML = Ke, n.body.appendChild(M), n.body.appendChild(p), n.body.classList.add("toc-dialog-ready"), M.addEventListener("click", function() {
            re(!1), p.showModal(), n.body.classList.add("toc-dialog-open"), V();
            var r = p.querySelector("a.active");
            if (r)
              r.scrollIntoView({ block: "nearest" });
          }), Q.addEventListener("click", q), p.addEventListener("click", function(r) {
            if (r.target === p)
              q();
          }), p.addEventListener("keydown", function(r) {
            if (r.key === "Escape")
              r.preventDefault(), q();
          }), p.addEventListener("close", function() {
            if (n.body.classList.remove("toc-dialog-open"), V(), M.classList.contains("is-visible"))
              setTimeout(function() {
                M.focus();
              }, 50);
          }), ie.querySelectorAll("a").forEach(function(r) {
            r.addEventListener("click", q);
          });
        } else
          p = null;
      var Ee = ae.slice();
      if (p)
        Ee = Ee.concat(Array.from(p.querySelectorAll('a[href^="#"]')));
      var rt = ae.map(function(r) {
        return n.getElementById((r.getAttribute("href") || "").slice(1));
      }), Me = !1;
      n.addEventListener("scroll", t, { passive: !0 }), window.addEventListener("resize", e), e();
    }
    var ve = n.querySelector(".top-controls");
    function me() {
      var e = n.documentElement.dataset.airplanFixedNavbar !== "false", t = e && ve ? ve.getBoundingClientRect().height : 0;
      n.documentElement.style.setProperty("--airplan-sticky-height", t + "px");
    }
    if (ve) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(me).observe(ve);
      window.addEventListener("resize", me), window.addEventListener("airplan:navbarchange", me), me();
    }
    let Le = n.querySelector(".copy-source");
    if (Le && Y)
      Le.addEventListener("click", function() {
        var e = Y.querySelector("pre");
        Te(e ? e.textContent : "", Le);
      });
    y.querySelectorAll("pre").forEach(function(e) {
      if (e.classList.contains("mermaid"))
        return;
      var t = n.createElement("div");
      t.className = "codewrap", e.parentNode?.insertBefore(t, e), t.appendChild(e);
      var r = n.createElement("button");
      r.className = "codecopy", r.type = "button", r.setAttribute("aria-label", "Copy code"), r.title = "Copy code", r.innerHTML = Ve + $e + ze, r.addEventListener("click", function() {
        var i = e.querySelector("code");
        Te((i || e).textContent, r);
      }), t.appendChild(r);
    });
  })();
})();
