(() => {
  function A0(A) {
    return A === "system" || A === "light" || A === "dark";
  }
  function V1(A, $) {
    try {
      return A?.getItem($) ?? null;
    } catch {
      return null;
    }
  }
  function t(A, $, Q) {
    try {
      if (Q === null)
        A?.removeItem($);
      else
        A?.setItem($, Q);
    } catch {}
  }
  function D1(A, $, Q) {
    let X = V1(Q, "airplan-color-mode");
    if (X === null) {
      let W = V1(Q, "airplan-theme");
      if (X = W === "light" || W === "dark" ? W : "system", X !== "system")
        t(Q, "airplan-color-mode", X);
    }
    let U = A0(X) ? X : "system", I = new Set(A.themes.map((W) => W.id)), V = V1(Q, "airplan-light-theme"), K = V1(Q, "airplan-dark-theme"), x = V !== null && I.has(V) ? V : A.defaultLight, F = K !== null && I.has(K) ? K : A.defaultDark;
    return O1(A, U, x, F, $);
  }
  function O1(A, $, Q, X, U) {
    let I = new Map(A.themes.map((v) => [v.id, v])), V = I.has(Q) ? Q : A.defaultLight, K = I.has(X) ? X : A.defaultDark, x = $ === "system" ? U ? "dark" : "light" : $, F = x === "light" ? V : K, W = I.get(F)?.variant ?? x;
    return { mode: $, resolvedMode: x, lightTheme: V, darkTheme: K, theme: F, variant: W };
  }
  function j1(A, $) {
    if ($ === "system")
      t(A, "airplan-color-mode", null), t(A, "airplan-theme", null);
    else
      t(A, "airplan-color-mode", $), t(A, "airplan-theme", $);
  }
  function y1(A, $, Q) {
    t(A, $ === "light" ? "airplan-light-theme" : "airplan-dark-theme", Q);
  }
  function m1(A) {
    return {
      mode: A.mode,
      resolvedMode: A.resolvedMode,
      theme: A.theme,
      variant: A.variant
    };
  }

  (function() {
    let A = document, $ = A.documentElement;
    A.querySelectorAll(".js-only").forEach((_) => {
      _.hidden = !1;
    });
    let Q = window.__AIRPLAN_THEME_CATALOG__;
    if (!Q)
      return;
    let X = Q, U = window.matchMedia("(prefers-color-scheme: dark)"), I;
    try {
      I = window.localStorage;
    } catch {}
    let V = window.__airplanThemeState ?? D1(X, U.matches, I), K = A.querySelector("[data-airplan-appearance-trigger]"), x = A.querySelector("[data-airplan-appearance-panel]"), F = A.querySelector('select[data-airplan-theme-slot="light"]'), W = A.querySelector('select[data-airplan-theme-slot="dark"]'), v = Array.from(A.querySelectorAll("[data-airplan-color-mode]"));
    if (x)
      A.body.appendChild(x);
    function a(_) {
      if (!_ || _.options.length > 0)
        return;
      for (let [G, R] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let L = A.createElement("optgroup");
        L.label = R;
        for (let u of X.themes) {
          if (u.variant !== G)
            continue;
          let f = A.createElement("option");
          f.value = u.id, f.textContent = u.name, L.append(f);
        }
        if (L.children.length > 0)
          _.append(L);
      }
    }
    a(F), a(W);
    function n(_, G = !0) {
      if (V = _, window.__airplanThemeState = V, $.dataset.airplanMode = V.mode, $.dataset.airplanResolvedMode = V.resolvedMode, $.dataset.airplanTheme = V.theme, $.dataset.airplanThemeVariant = V.variant, v.forEach((R) => {
        let L = R.dataset.airplanColorMode === V.mode;
        R.classList.toggle("active", L), R.setAttribute("aria-pressed", String(L));
      }), F)
        F.value = V.lightTheme;
      if (W)
        W.value = V.darkTheme;
      if (G)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: m1(V) }));
    }
    function k(_ = {}) {
      n(O1(X, _.mode ?? V.mode, _.lightTheme ?? V.lightTheme, _.darkTheme ?? V.darkTheme, U.matches));
    }
    function r(_, G = !1) {
      if (!x || !K)
        return;
      if (_)
        e();
      if (x.hidden = !_, K.setAttribute("aria-expanded", String(_)), _)
        x.querySelector("button,select")?.focus();
      else if (G)
        K.focus();
    }
    function e() {
      if (!x || !K)
        return;
      let _ = K.getBoundingClientRect(), G = K.closest(".toolbar")?.getBoundingClientRect(), R = A.documentElement.clientWidth, L = Math.min(304, R - 32), u = Math.max(16, R - _.right);
      x.style.setProperty("--airplan-appearance-top", `${(G?.bottom ?? _.bottom) + 8}px`), x.style.setProperty("--airplan-appearance-right", `${Math.min(u, Math.max(16, R - L - 16))}px`);
    }
    K?.addEventListener("click", () => r(Boolean(x?.hidden ?? !0))), v.forEach((_) => _.addEventListener("click", () => {
      let G = _.dataset.airplanColorMode;
      if (!G)
        return;
      j1(I, G), k({ mode: G });
    }));
    function B1(_, G) {
      y1(I, _, G.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: G.value } })), k(_ === "light" ? { lightTheme: G.value } : { darkTheme: G.value });
    }
    F?.addEventListener("change", () => B1("light", F)), W?.addEventListener("change", () => B1("dark", W)), U.addEventListener("change", () => {
      if (V.mode === "system")
        k();
    }), A.addEventListener("keydown", (_) => {
      if (_.key === "Escape" && x && !x.hidden)
        _.preventDefault(), r(!1, !0);
    }), A.addEventListener("pointerdown", (_) => {
      if (!x || x.hidden || !K)
        return;
      let G = _.target;
      if (!(G instanceof Node) || x.contains(G) || K.contains(G))
        return;
      let L = (G instanceof Element ? G : G.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), u = x.contains(A.activeElement) && !L;
      if (r(!1), u)
        setTimeout(() => {
          if (A.activeElement === A.body || x.contains(A.activeElement))
            K.focus();
        });
    }), window.addEventListener("resize", () => {
      if (x && !x.hidden)
        e();
    }), window.addEventListener("scroll", () => {
      if (x && !x.hidden)
        e();
    }), n(V, !1);
  })();

  var P1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', v1 = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', n1 = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>';
  var b1 = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';
  var l1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>';

  (function() {
    var A = document, $ = 262144;
    let Q = A.getElementById("rendered");
    if (!Q)
      return;
    let X = Q;
    var U = A.querySelector('meta[name="airplan-versions"]'), I = A.querySelector('meta[name="airplan-revision-chain"]'), V = A.querySelector('meta[name="airplan-page-path"]'), K = A.querySelector('meta[name="airplan-entrypoint"]'), x = U ? new URL(U.content, window.location.href) : null, F = x ? new URL("./", x) : null, W = F ? F.pathname.split("/").filter(Boolean) : [], v = W.slice(0, -1);
    function a(Z, h) {
      if (typeof Z !== "string")
        return null;
      try {
        var E = new URL(Z);
        if (E.origin !== window.location.origin || E.username || E.password || E.search || E.hash)
          return null;
        var M = E.pathname.split("/").filter(Boolean);
        if (M.length !== v.length + 2 || !v.every(function(C, H) {
          return M[H] === C;
        }) || !/^[a-z2-7]{26}$/.test(M[M.length - 2]))
          return null;
        var B = M[M.length - 1];
        if (h ? B !== ".airplan-changes.diff" : !B.endsWith(".html"))
          return null;
        return E.href;
      } catch {
        return null;
      }
    }
    function n(Z) {
      if (typeof Z !== "string" || Z === "" || Z.startsWith("/") || Z.includes("\\"))
        return !1;
      var h = Z.split("/");
      return h.every(function(E) {
        var M = E.toLowerCase(), B = Array.from(E).some(function(C) {
          var H = C.codePointAt(0) || 0;
          return H < 32 || H === 127;
        });
        if (!E || E === "." || E === ".." || M.startsWith(".airplan-") || M === ".airplan.json" || B || /[. ]$/.test(E) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(E))
          return !1;
        return !0;
      });
    }
    function k(Z, h) {
      if (!n(h))
        return null;
      var E = String(h).split("/").map(function(B) {
        return encodeURIComponent(B);
      }).join("/"), M = new URL(E, Z);
      if (M.origin !== Z.origin || M.username || M.password || M.search || M.hash || !M.pathname.startsWith(Z.pathname))
        return null;
      return M.href;
    }
    async function r(Z) {
      if (!Z.ok)
        throw Error("marker request failed");
      var h = Z.headers.get("content-length");
      if (h && /^\d+$/.test(h) && Number(h) > $) {
        if (Z.body)
          await Z.body.cancel("marker is too large");
        throw Error("marker is too large");
      }
      if (!Z.body || typeof Z.body.getReader !== "function")
        throw Error("bounded marker stream is unavailable");
      var E = Z.body.getReader(), M = [], B = 0;
      try {
        for (;; ) {
          var C = await E.read();
          if (C.done)
            break;
          if (B += C.value.byteLength, B > $)
            throw await E.cancel("marker is too large"), Error("marker is too large");
          M.push(C.value);
        }
      } finally {
        E.releaseLock();
      }
      var H = new Uint8Array(B), S = 0;
      M.forEach(function(T) {
        H.set(T, S), S += T.byteLength;
      });
      var O = new TextDecoder("utf-8", { fatal: !0 }).decode(H);
      return e(O), JSON.parse(O);
    }
    function e(Z) {
      var h = 0;
      function E() {
        while (/\s/.test(Z[h] || ""))
          h += 1;
      }
      function M() {
        if (Z[h] !== '"')
          throw Error("JSON string is invalid");
        var C = h++;
        while (h < Z.length) {
          var H = Z[h++];
          if (H === '"')
            return JSON.parse(Z.slice(C, h));
          if (H === "\\")
            h += 1;
        }
        throw Error("JSON string is incomplete");
      }
      function B() {
        if (E(), Z[h] === "{") {
          h += 1, E();
          var C = new Set;
          if (Z[h] === "}") {
            h += 1;
            return;
          }
          for (;; ) {
            E();
            var H = M();
            if (C.has(H))
              throw Error("JSON object has a duplicate field");
            if (C.add(H), E(), Z[h++] !== ":")
              throw Error("JSON object is invalid");
            B(), E();
            var S = Z[h++];
            if (S === "}")
              return;
            if (S !== ",")
              throw Error("JSON object is invalid");
          }
        }
        if (Z[h] === "[") {
          if (h += 1, E(), Z[h] === "]") {
            h += 1;
            return;
          }
          for (;; ) {
            B(), E();
            var S = Z[h++];
            if (S === "]")
              return;
            if (S !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (Z[h] === '"') {
          M();
          return;
        }
        var O = Z.slice(h).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!O)
          throw Error("JSON value is invalid");
        h += O[0].length;
      }
      if (B(), E(), h !== Z.length)
        throw Error("JSON has trailing content");
    }
    function B1(Z, h, E, M) {
      if (!f(Z))
        throw Error("marker is invalid");
      var B = Z, C = h.pathname.split("/").filter(Boolean), H = C[C.length - 1] || "";
      if (B.schema !== "airplan-upload" || B.version !== 6 || B.kind !== "document" || B.directory !== H || !/^[a-z2-7]{26}$/.test(B.directory) || !c1(B.created_at) || B.format !== "md" || !k1(B.slug) || B.entrypoint !== B.slug + ".html" || !f(B.producer) || B.producer.name !== "airplan" || typeof B.producer.version !== "string" || B.producer.version.trim() !== B.producer.version || B.producer.version === "" || !G(B.render) || !f(B.revision) || B.revision.number !== E.number || B.revision.chain_id !== M || (B.revision.number === 1 ? B.revision.previous_url !== void 0 : typeof B.revision.previous_url !== "string" || !g1(B.revision.previous_url)) || !Array.isArray(B.objects) || !Array.isArray(B.pages) || B.pages.length === 0)
        throw Error("marker identity is invalid");
      var S = k(h, B.entrypoint);
      if (S !== E.safeURL)
        throw Error("marker entrypoint is invalid");
      if (B.title !== void 0 && typeof B.title !== "string" || B.repo !== void 0 && !i1(B.repo) || B.objects.length === 0 || B.pages.length > 100)
        throw Error("marker shape is invalid");
      var O = o1(B), T = new Set, i = new Set, l = new Set, p = new Map;
      if (B.pages.forEach(function(w, q) {
        if (!f(w) || !n(w.path) || T.has(w.path) || i.has(w.path.toLowerCase()) || w.format !== "md" && w.format !== "txt" || typeof w.lang !== "string" || w.title !== void 0 && typeof w.title !== "string" || !n(w.page) || !n(w.source))
          throw Error("marker page descriptor is invalid");
        var y = _(w.path, w.format), P = w.path;
        if (q === 0) {
          if (y = B.entrypoint, P = B.slug + ".md", w.format !== B.format)
            throw Error("marker entry format is invalid");
        }
        if (w.page !== y || w.source !== P)
          throw Error("marker generated page mapping is invalid");
        var N = k(h, w.page);
        if (!N || l.has(N))
          throw Error("marker page object is invalid");
        if (!k(h, w.source))
          throw Error("marker source object is invalid");
        if (O.get(w.page) !== "page" || O.get(w.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var j = B.objects.find(function(g) {
          return g.name === w.source;
        }).content_type;
        if (w.format === "md" && j !== "text/markdown; charset=utf-8" || w.format === "txt" && j !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        T.add(w.path), i.add(w.path.toLowerCase()), l.add(N), p.set(w.path, N);
      }), L(i))
        throw Error("marker page paths conflict");
      if (!T.has(B.pages[0].path) || p.get(B.pages[0].path) !== S)
        throw Error("marker entry page is invalid");
      if (l.size !== B.pages.length || Array.from(O.values()).filter(function(w) {
        return w === "source";
      }).length !== B.pages.length)
        throw Error("marker page inventory is invalid");
      return p;
    }
    function _(Z, h) {
      if (h !== "md")
        return Z + ".html";
      var E = Z.lastIndexOf("/"), M = Z.lastIndexOf(".");
      return (M > E ? Z.slice(0, M) : Z) + ".html";
    }
    function G(Z) {
      if (!f(Z) || !f(Z.template) || !f(Z.themes) || !Number.isInteger(Z.generation) || Z.generation <= 0 || typeof Z.indexable !== "boolean" || typeof Z.no_external_assets !== "boolean" || !Z.template || Z.template.kind !== "builtin" && Z.template.kind !== "custom" || Z.mermaid_url !== void 0 && !d1(Z.mermaid_url) || !Z.themes)
        return !1;
      if (Z.template.kind === "builtin" && Z.template.sha256 !== void 0 || Z.template.kind === "custom" && !u(Z.template.sha256))
        return !1;
      return R(Z.themes.default_light) && R(Z.themes.default_dark) && u(Z.themes.catalog_sha256);
    }
    function R(Z) {
      return typeof Z === "string" && new TextEncoder().encode(Z).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(Z);
    }
    function L(Z) {
      for (var h of Z) {
        var E = h.indexOf("/");
        while (E >= 0) {
          if (Z.has(h.slice(0, E)))
            return !0;
          E = h.indexOf("/", E + 1);
        }
      }
      return !1;
    }
    function u(Z) {
      return typeof Z === "string" && /^[0-9a-f]{64}$/.test(Z);
    }
    function f(Z) {
      return !!Z && typeof Z === "object" && !Array.isArray(Z);
    }
    function k1(Z) {
      return typeof Z === "string" && Z.length <= 64 && /^[a-z0-9-]+$/.test(Z);
    }
    function c1(Z) {
      if (typeof Z !== "string")
        return !1;
      var h = Z.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/);
      if (!h)
        return !1;
      var E = Number(h[1]), M = Number(h[2]), B = Number(h[3]), C = Number(h[4]), H = Number(h[5]), S = Number(h[6]), O = E % 4 === 0 && (E % 100 !== 0 || E % 400 === 0), T = [31, O ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return M >= 1 && M <= 12 && B >= 1 && B <= T[M - 1] && C <= 23 && H <= 59 && S <= 59;
    }
    function i1(Z) {
      if (typeof Z !== "string" || Z === "" || Z.trim() !== Z)
        return !1;
      try {
        var h = new URL(Z);
        if (h.protocol !== "https:" || h.username || h.password || h.port || h.search || h.hash)
          return !1;
        var E = h.pathname.replace(/^\/+|\/+$/g, "").split("/");
        if (E.length !== 2)
          return !1;
        var M = E[0], B = E[1].replace(/\.git$/, "");
        if (!M || !B || M === "." || M === ".." || B === "." || B === ".." || /[?#@:\\]/.test(M + B))
          return !1;
        return Z === "https://" + h.hostname.toLowerCase() + "/" + M + "/" + B;
      } catch {
        return !1;
      }
    }
    function p1(Z) {
      return typeof Z === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(Z);
    }
    function g1(Z) {
      try {
        var h = new URL(Z);
        return (h.protocol === "https:" || h.protocol === "http:") && !h.username && !h.password && !h.search && !h.hash && h.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function d1(Z) {
      if (typeof Z !== "string")
        return !1;
      try {
        var h = new URL(Z);
        return h.protocol === "https:" && !!h.host && !h.username && !h.password && !h.hash;
      } catch {
        return !1;
      }
    }
    function o1(Z) {
      var h = new Map, E = new Set, M = 0, B = 0, C = 0, H = 0;
      if (Z.objects.forEach(function(S) {
        if (!f(S) || !n(S.name) && S.name !== ".airplan-changes.diff" || h.has(S.name) || E.has(S.name.toLowerCase()) || !Number.isSafeInteger(S.bytes) || S.bytes < 0 || !u(S.sha256) || !p1(S.content_type))
          throw Error("marker object inventory is invalid");
        if (S.role === "page") {
          if (B += 1, S.bytes <= 0 || S.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (S.role === "source") {
          if (C += 1, S.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (S.role === "asset")
          H += 1;
        else if (S.role === "diff") {
          if (M += 1, S.name !== ".airplan-changes.diff" || S.bytes <= 0 || S.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        h.set(S.name, S.role), E.add(S.name.toLowerCase());
      }), L(E))
        throw Error("marker object paths conflict");
      if (B !== Z.pages.length || C !== Z.pages.length || B + H > 100 || (Z.revision.number === 1 ? M !== 0 : M !== 1))
        throw Error("marker object counts are invalid");
      return h;
    }
    function s1(Z, h) {
      var E = window.location.hash;
      if (E === "#airplan-all-changes")
        return Z + E;
      if (!h)
        return Z;
      return h + (E && E !== "#airplan-all-changes" ? E : "");
    }
    function t1(Z) {
      var h = A.querySelector('meta[name="airplan-revision"]'), E = h ? Number(h.content) : Number(Z.current_revision);
      if (!Number.isInteger(E) || E <= 0 || Z.current_revision !== E || !Number.isInteger(Z.latest_revision) || !Number.isInteger(Z.last_assigned_revision) || !Array.isArray(Z.revisions) || Z.revisions.length === 0 || Z.last_assigned_revision !== Z.revisions.length || !/^[a-z2-7]{26}$/.test(Z.chain_id) || I && I.content !== Z.chain_id)
        throw Error("revision identity is invalid");
      var M = !1, B = 0, C = Z.revisions.filter(function(q) {
        if (!q || !Number.isInteger(q.number) || q.number !== B + 1)
          return M = !0, !1;
        if (B = q.number, q.deleted)
          return !1;
        if (q.safeURL = a(q.url, !1), !q.safeURL)
          return M = !0, !1;
        if (q.number > 1) {
          var y = a(q.diff_url, !0);
          if (!y || new URL(y).pathname.replace(/[^/]+$/, "") !== new URL(q.safeURL).pathname.replace(/[^/]+$/, ""))
            return M = !0, !1;
        }
        return !0;
      });
      if (M || Z.revisions[0].number !== 1 || !C.some(function(q) {
        return q.number === E;
      }))
        throw Error("revision entries are invalid");
      var H = C.find(function(q) {
        return q.number === E;
      }), S = new URL(window.location.href);
      if (S.search = "", S.hash = "", !H || !F || new URL(H.safeURL || "").pathname.replace(/[^/]+$/, "") !== F.pathname || !S.pathname.startsWith(F.pathname))
        throw Error("current revision URL is invalid");
      var O = Math.max.apply(null, C.map(function(q) {
        return q.number;
      }));
      if (O !== Z.latest_revision)
        throw Error("latest is invalid");
      var T = Array.from(A.querySelectorAll("[data-revision-controls]")), i = Array.from(A.querySelectorAll("[data-revision-heading]"));
      if (i.length === 0) {
        if (T.length === 0)
          throw Error("revision controls are unavailable");
        var l = A.createElement("p");
        l.className = "revision-heading", l.setAttribute("data-revision-heading", ""), T[0].appendChild(l), i.push(l);
      }
      T.forEach(function(q) {
        q.hidden = !1;
      });
      var p = E < O, w = p ? "Revision " + E + " of " + O : "Revision " + E + " (Latest)";
      i.forEach(function(q) {
        var y = A.createElement("span");
        y.className = "revision-picker-label", y.textContent = w, y.setAttribute("aria-hidden", "true");
        var P = A.createElement("select");
        P.setAttribute("aria-label", "Document revision"), C.forEach(function(N) {
          var j = A.createElement("option");
          j.value = N.safeURL || "", j.textContent = N.number === O ? "Revision " + N.number + " (Latest)" : "Revision " + N.number + " of " + O, j.selected = N.number === E, P.appendChild(j);
        }), P.addEventListener("change", function() {
          var N = P.selectedIndex;
          if (N < 0 || N >= C.length)
            return;
          var j = C[N], g = j.safeURL || "";
          if (window.location.hash === "#airplan-all-changes") {
            window.location.assign(g + (j.number > 1 ? "#airplan-all-changes" : ""));
            return;
          }
          var Z0 = K ? new URL(K.content, window.location.href).href : "";
          if (!V || S.href === Z0 || !I) {
            window.location.assign(g);
            return;
          }
          q.setAttribute("aria-busy", "true"), P.disabled = !0;
          var T1 = new URL("./", g), u1 = new URL(".airplan.json", T1);
          u1.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(u1, { cache: "no-store", credentials: "same-origin" }).then(r).then(function(h0) {
            var E0 = B1(h0, T1, j, I.content);
            window.location.assign(s1(g, E0.get(V.content) || null));
          }).catch(function() {
            console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(g);
          });
        }), q.replaceChildren(y, P), q.classList.add("is-picker"), q.classList.toggle("is-stale", p);
      }), A.body.classList.toggle("airplan-stale-revision", p);
    }
    if (U) {
      var I1 = new URL(U.content, window.location.href);
      I1.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(I1, { cache: "no-store", credentials: "same-origin" }).then(function(Z) {
        if (Z.status === 404)
          return null;
        if (!Z.ok)
          throw Error("metadata request failed");
        return Z.json();
      }).then(function(Z) {
        if (Z === null)
          return;
        if (!Z || Z.schema !== "airplan-versions" || Z.version !== 1 || !Array.isArray(Z.revisions) || Z.revisions.length < 2)
          throw Error("metadata is invalid");
        t1(Z), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: Z
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var M1 = A.createElement("div");
    M1.className = "sr-status", M1.setAttribute("aria-live", "polite"), A.body.appendChild(M1);
    var d = null;
    function a1() {
      if (d !== null)
        return;
      d = Array.from(A.querySelectorAll("details:not([open])")), d.forEach(function(Z) {
        Z.open = !0;
      });
    }
    function r1() {
      if (d === null)
        return;
      d.forEach(function(Z) {
        Z.open = !1;
      }), d = null;
    }
    window.addEventListener("beforeprint", a1), window.addEventListener("afterprint", r1);
    function G1(Z, h, E) {
      M1.textContent = h;
      var M = Z.querySelector(".action-label"), B = M ? M.textContent : "";
      if (M)
        M.textContent = E ? "Copied" : "Failed";
      Z.classList.add(E ? "is-copied" : "is-failed"), Z.disabled = !0, setTimeout(function() {
        if (Z.classList.remove("is-copied", "is-failed"), Z.disabled = !1, M)
          M.textContent = B;
      }, 1200);
    }
    function F1(Z, h) {
      if (!navigator.clipboard) {
        G1(h, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(Z).then(function() {
        G1(h, "Copied!", !0);
      }, function() {
        G1(h, "Copy failed", !1);
      });
    }
    var W1 = A.getElementById("pages"), D = A.querySelector(".pages-trigger"), Y = null, $1 = window.matchMedia("(max-width: 78rem)"), m = function() {};
    function H1() {
      return Y ? Y.matches(":popover-open") : !1;
    }
    function Z1(Z) {
      if (!Y || !H1())
        return;
      if (Y.hidePopover(), Z && D && $1.matches)
        setTimeout(function() {
          D.focus();
        }, 0);
    }
    if (W1 && D) {
      var L1 = W1.querySelector(".pages-list");
      if (L1) {
        var z1 = A.createElement("div");
        if ("popover" in z1 && typeof z1.showPopover === "function") {
          let Z = function() {
            if (!D || !Y)
              return;
            var h = D.getBoundingClientRect(), E = D.closest(".toolbar"), M = E ? E.getBoundingClientRect().bottom : h.bottom;
            Y.style.setProperty("--pages-left", Math.max(16, h.left) + "px"), Y.style.setProperty("--pages-top", M + "px"), Y.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, h.left) - 16) + "px");
          };
          Y = z1, Y.className = "pages-popover", Y.id = "pages-popover", Y.setAttribute("popover", "auto");
          var h1 = A.createElement("nav");
          h1.className = "pages-popover-nav", h1.setAttribute("aria-label", "Pages"), h1.appendChild(L1.cloneNode(!0)), Y.appendChild(h1), D.setAttribute("popovertarget", Y.id), D.popoverTargetElement = Y, Y.addEventListener("beforetoggle", function(h) {
            if (h.newState !== "open")
              return;
            m(), Z();
          }), Y.addEventListener("toggle", function(h) {
            var E = h.newState === "open";
            if (D.setAttribute("aria-expanded", E ? "true" : "false"), A.body.classList.toggle("pages-popover-open", E), E) {
              var M = Y.querySelector('[aria-current="page"]');
              if (M)
                M.scrollIntoView({ block: "nearest" });
            }
            c();
          }), h1.querySelectorAll("a").forEach(function(h) {
            h.addEventListener("click", function() {
              Z1(!1);
            });
          }), $1.addEventListener("change", function() {
            if (!$1.matches)
              Z1(!1);
          }), window.addEventListener("resize", function() {
            if (H1())
              Z();
          }), D.hidden = !1, D.setAttribute("aria-expanded", "false"), A.body.appendChild(Y), A.body.classList.add("pages-popover-ready");
        }
      }
    }
    var o = A.getElementById("source"), S1 = A.getElementById("changes"), _1 = A.querySelector("[data-airplan-all-changes]"), b = A.getElementById("toc"), J = null, z = null, N1 = window.matchMedia("(max-width: 78rem)");
    m = function() {
      if (z && z.open)
        z.close();
    };
    function c() {
      if (!b || !J || !z)
        return;
      var Z = N1.matches && !X.hidden && !z.open && !H1();
      if (J.classList.toggle("is-visible", Z), J.tabIndex = Z ? 0 : -1, J.setAttribute("aria-hidden", Z ? "false" : "true"), z.open && (!N1.matches || X.hidden))
        m();
    }
    function R1(Z) {
      if (Z1(!1), m(), X.hidden = Z !== "rendered", o)
        o.hidden = Z !== "source";
      if (S1)
        S1.hidden = Z !== "changes";
      if (b)
        b.hidden = Z !== "rendered";
      A.querySelectorAll(".viewtoggle button").forEach(function(h) {
        var E = h.dataset.view === Z;
        h.classList.toggle("active", E), h.setAttribute("aria-pressed", E ? "true" : "false");
      }), c();
    }
    A.querySelectorAll(".viewtoggle button").forEach(function(Z) {
      Z.addEventListener("click", function() {
        R1(Z.dataset.view || "rendered");
      });
    });
    var K1 = !1;
    A.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(Z) {
      Z.addEventListener("click", function() {
        K1 = new URL(Z.href).pathname === window.location.pathname;
      });
    });
    function U1() {
      var Z = window.location.hash === "#airplan-all-changes" && !!_1;
      if (Z1(!1), m(), A.body.classList.toggle("all-changes-active", Z), _1)
        _1.hidden = !Z;
      if (Z) {
        if (X.hidden = !0, o)
          o.hidden = !0;
        if (S1)
          S1.hidden = !0;
        if (b)
          b.hidden = !0;
        if (K1)
          _1.querySelector("h1")?.focus();
      } else
        R1("rendered");
      K1 = !1, c();
    }
    if (window.addEventListener("hashchange", U1), U1(), b) {
      let Z = function() {
        if (E1.length === 0) {
          c();
          return;
        }
        var E = 0;
        if (e1.forEach(function(B, C) {
          if (B && B.getBoundingClientRect().top <= 128)
            E = C;
        }), window.scrollY <= 128)
          E = 0;
        else if (window.innerHeight + window.scrollY >= A.documentElement.scrollHeight - 2)
          E = E1.length - 1;
        var M = E1[E].getAttribute("href");
        Q1.forEach(function(B) {
          var C = B.getAttribute("href") === M;
          if (B.classList.toggle("active", C), C)
            B.setAttribute("aria-current", "location");
          else
            B.removeAttribute("aria-current");
        }), c();
      }, h = function() {
        if (X1)
          return;
        X1 = !0, window.requestAnimationFrame(function() {
          X1 = !1, Z();
        });
      };
      var E1 = Array.from(b.querySelectorAll('a[href^="#"]')), f1 = b.querySelector(".toc-list");
      if (f1)
        if (z = A.createElement("dialog"), typeof z.showModal === "function") {
          z.className = "toc-dialog", z.id = "toc-dialog", z.setAttribute("aria-labelledby", "toc-dialog-title");
          var C1 = A.createElement("div");
          C1.className = "toc-dialog-panel";
          var w1 = A.createElement("div");
          w1.className = "toc-dialog-header";
          var x1 = A.createElement("h2");
          x1.className = "toc-dialog-title", x1.id = "toc-dialog-title", x1.textContent = "Contents";
          var s = A.createElement("button");
          s.className = "toc-dialog-close", s.type = "button", s.setAttribute("aria-label", "Close table of contents"), s.innerHTML = P1, w1.appendChild(x1), w1.appendChild(s);
          var A1 = A.createElement("nav");
          A1.className = "toc-dialog-nav", A1.setAttribute("aria-label", "Table of contents"), A1.appendChild(f1.cloneNode(!0)), C1.appendChild(w1), C1.appendChild(A1), z.appendChild(C1), J = A.createElement("button"), J.className = "toc-trigger", J.type = "button", J.tabIndex = -1, J.setAttribute("aria-label", "Open table of contents"), J.setAttribute("aria-controls", "toc-dialog"), J.setAttribute("aria-haspopup", "dialog"), J.setAttribute("aria-hidden", "true"), J.innerHTML = l1, A.body.appendChild(J), A.body.appendChild(z), A.body.classList.add("toc-dialog-ready"), J.addEventListener("click", function() {
            Z1(!1), z.showModal(), A.body.classList.add("toc-dialog-open"), c();
            var E = z.querySelector("a.active");
            if (E)
              E.scrollIntoView({ block: "nearest" });
          }), s.addEventListener("click", m), z.addEventListener("click", function(E) {
            if (E.target === z)
              m();
          }), z.addEventListener("keydown", function(E) {
            if (E.key === "Escape")
              E.preventDefault(), m();
          }), z.addEventListener("close", function() {
            if (A.body.classList.remove("toc-dialog-open"), c(), J.classList.contains("is-visible"))
              setTimeout(function() {
                J.focus();
              }, 50);
          }), A1.querySelectorAll("a").forEach(function(E) {
            E.addEventListener("click", m);
          });
        } else
          z = null;
      var Q1 = E1.slice();
      if (z)
        Q1 = Q1.concat(Array.from(z.querySelectorAll('a[href^="#"]')));
      var e1 = E1.map(function(E) {
        return A.getElementById((E.getAttribute("href") || "").slice(1));
      }), X1 = !1;
      A.addEventListener("scroll", h, { passive: !0 }), window.addEventListener("resize", Z), Z();
    }
    var q1 = A.querySelector(".top-controls");
    function Y1() {
      var Z = q1 ? q1.getBoundingClientRect().height : 0;
      A.documentElement.style.setProperty("--airplan-sticky-height", Z + "px");
    }
    if (q1) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(Y1).observe(q1);
      window.addEventListener("resize", Y1), Y1();
    }
    let J1 = A.querySelector(".copy-source");
    if (J1 && o)
      J1.addEventListener("click", function() {
        var Z = o.querySelector("pre");
        F1(Z ? Z.textContent : "", J1);
      });
    X.querySelectorAll("pre").forEach(function(Z) {
      if (Z.classList.contains("mermaid"))
        return;
      var h = A.createElement("div");
      h.className = "codewrap", Z.parentNode?.insertBefore(h, Z), h.appendChild(Z);
      var E = A.createElement("button");
      E.className = "codecopy", E.type = "button", E.setAttribute("aria-label", "Copy code"), E.title = "Copy code", E.innerHTML = n1 + v1 + b1, E.addEventListener("click", function() {
        var M = Z.querySelector("code");
        F1((M || Z).textContent, E);
      }), h.appendChild(E);
    });
  })();
})();
