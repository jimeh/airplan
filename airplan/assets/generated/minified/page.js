(() => {
  function A1(A) {
    return A === "system" || A === "light" || A === "dark";
  }
  function $0(A, H) {
    try {
      return A?.getItem(H) ?? null;
    } catch {
      return null;
    }
  }
  function t(A, H, Q) {
    try {
      if (Q === null)
        A?.removeItem(H);
      else
        A?.setItem(H, Q);
    } catch {}
  }
  function D0(A, H, Q) {
    let X = $0(Q, "airplan-color-mode");
    if (X === null) {
      let W = $0(Q, "airplan-theme");
      if (X = W === "light" || W === "dark" ? W : "system", X !== "system")
        t(Q, "airplan-color-mode", X);
    }
    let U = A1(X) ? X : "system", O = new Set(A.themes.map((W) => W.id)), $ = $0(Q, "airplan-light-theme"), K = $0(Q, "airplan-dark-theme"), w = $ !== null && O.has($) ? $ : A.defaultLight, F = K !== null && O.has(K) ? K : A.defaultDark;
    return I0(A, U, w, F, H);
  }
  function I0(A, H, Q, X, U) {
    let O = new Map(A.themes.map((n) => [n.id, n])), $ = O.has(Q) ? Q : A.defaultLight, K = O.has(X) ? X : A.defaultDark, w = H === "system" ? U ? "dark" : "light" : H, F = w === "light" ? $ : K, W = O.get(F)?.variant ?? w;
    return { mode: H, resolvedMode: w, lightTheme: $, darkTheme: K, theme: F, variant: W };
  }
  function j0(A, H) {
    if (H === "system")
      t(A, "airplan-color-mode", null), t(A, "airplan-theme", null);
    else
      t(A, "airplan-color-mode", H), t(A, "airplan-theme", H);
  }
  function y0(A, H, Q) {
    t(A, H === "light" ? "airplan-light-theme" : "airplan-dark-theme", Q);
  }
  function m0(A) {
    return {
      mode: A.mode,
      resolvedMode: A.resolvedMode,
      theme: A.theme,
      variant: A.variant
    };
  }

  (function() {
    let A = document, H = A.documentElement;
    A.querySelectorAll(".js-only").forEach((_) => {
      _.hidden = !1;
    });
    let Q = window.__AIRPLAN_THEME_CATALOG__;
    if (!Q)
      return;
    let X = Q, U = window.matchMedia("(prefers-color-scheme: dark)"), O;
    try {
      O = window.localStorage;
    } catch {}
    let $ = window.__airplanThemeState ?? D0(X, U.matches, O), K = A.querySelector("[data-airplan-appearance-trigger]"), w = A.querySelector("[data-airplan-appearance-panel]"), F = A.querySelector('select[data-airplan-theme-slot="light"]'), W = A.querySelector('select[data-airplan-theme-slot="dark"]'), n = Array.from(A.querySelectorAll("[data-airplan-color-mode]"));
    if (w)
      A.body.appendChild(w);
    function a(_) {
      if (!_ || _.options.length > 0)
        return;
      for (let [x, R] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let L = A.createElement("optgroup");
        L.label = R;
        for (let u of X.themes) {
          if (u.variant !== x)
            continue;
          let f = A.createElement("option");
          f.value = u.id, f.textContent = u.name, L.append(f);
        }
        if (L.children.length > 0)
          _.append(L);
      }
    }
    a(F), a(W);
    function v(_, x = !0) {
      if ($ = _, window.__airplanThemeState = $, H.dataset.airplanMode = $.mode, H.dataset.airplanResolvedMode = $.resolvedMode, H.dataset.airplanTheme = $.theme, H.dataset.airplanThemeVariant = $.variant, n.forEach((R) => {
        let L = R.dataset.airplanColorMode === $.mode;
        R.classList.toggle("active", L), R.setAttribute("aria-pressed", String(L));
      }), F)
        F.value = $.lightTheme;
      if (W)
        W.value = $.darkTheme;
      if (x)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: m0($) }));
    }
    function l(_ = {}) {
      v(I0(X, _.mode ?? $.mode, _.lightTheme ?? $.lightTheme, _.darkTheme ?? $.darkTheme, U.matches));
    }
    function r(_, x = !1) {
      if (!w || !K)
        return;
      if (_)
        e();
      if (w.hidden = !_, K.setAttribute("aria-expanded", String(_)), _)
        w.querySelector("button,select")?.focus();
      else if (x)
        K.focus();
    }
    function e() {
      if (!w || !K)
        return;
      let _ = K.getBoundingClientRect(), x = K.closest(".toolbar")?.getBoundingClientRect(), R = A.documentElement.clientWidth, L = Math.min(304, R - 32), u = Math.max(16, R - _.right);
      w.style.setProperty("--airplan-appearance-top", `${(x?.bottom ?? _.bottom) + 8}px`), w.style.setProperty("--airplan-appearance-right", `${Math.min(u, Math.max(16, R - L - 16))}px`);
    }
    K?.addEventListener("click", () => r(Boolean(w?.hidden ?? !0))), n.forEach((_) => _.addEventListener("click", () => {
      let x = _.dataset.airplanColorMode;
      if (!x)
        return;
      j0(O, x), l({ mode: x });
    }));
    function B0(_, x) {
      y0(O, _, x.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: x.value } })), l(_ === "light" ? { lightTheme: x.value } : { darkTheme: x.value });
    }
    F?.addEventListener("change", () => B0("light", F)), W?.addEventListener("change", () => B0("dark", W)), U.addEventListener("change", () => {
      if ($.mode === "system")
        l();
    }), A.addEventListener("keydown", (_) => {
      if (_.key === "Escape" && w && !w.hidden)
        _.preventDefault(), r(!1, !0);
    }), A.addEventListener("pointerdown", (_) => {
      if (!w || w.hidden || !K)
        return;
      let x = _.target;
      if (!(x instanceof Node) || w.contains(x) || K.contains(x))
        return;
      let L = (x instanceof Element ? x : x.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), u = w.contains(A.activeElement) && !L;
      if (r(!1), u)
        setTimeout(() => {
          if (A.activeElement === A.body || w.contains(A.activeElement))
            K.focus();
        });
    }), window.addEventListener("resize", () => {
      if (w && !w.hidden)
        e();
    }), window.addEventListener("scroll", () => {
      if (w && !w.hidden)
        e();
    }), v($, !1);
  })();

  var P0 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', n0 = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', v0 = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>';
  var b0 = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';
  var k0 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>';

  (function() {
    var A = document, H = 262144;
    let Q = A.getElementById("rendered");
    if (!Q)
      return;
    let X = Q;
    var U = A.querySelector('meta[name="airplan-versions"]'), O = A.querySelector('meta[name="airplan-revision-chain"]'), $ = A.querySelector('meta[name="airplan-page-path"]'), K = A.querySelector('meta[name="airplan-entrypoint"]'), w = U ? new URL(U.content, window.location.href) : null, F = w ? new URL("./", w) : null, W = F ? F.pathname.split("/").filter(Boolean) : [], n = W.slice(0, -1);
    function a(Z, E) {
      if (typeof Z !== "string")
        return null;
      try {
        var h = new URL(Z);
        if (h.origin !== window.location.origin || h.username || h.password || h.search || h.hash)
          return null;
        var M = h.pathname.split("/").filter(Boolean);
        if (M.length !== n.length + 2 || !n.every(function(C, V) {
          return M[V] === C;
        }) || !/^[a-z2-7]{26}$/.test(M[M.length - 2]))
          return null;
        var B = M[M.length - 1];
        if (E ? B !== ".airplan-changes.diff" : !B.endsWith(".html"))
          return null;
        return h.href;
      } catch {
        return null;
      }
    }
    function v(Z) {
      if (typeof Z !== "string" || Z === "" || Z.startsWith("/") || Z.includes("\\"))
        return !1;
      var E = Z.split("/");
      return E.every(function(h) {
        var M = h.toLowerCase(), B = Array.from(h).some(function(C) {
          var V = C.codePointAt(0) || 0;
          return V < 32 || V === 127;
        });
        if (!h || h === "." || h === ".." || M.startsWith(".airplan-") || M === ".airplan.json" || B || /[. ]$/.test(h) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(h))
          return !1;
        return !0;
      });
    }
    function l(Z, E) {
      if (!v(E))
        return null;
      var h = String(E).split("/").map(function(B) {
        return encodeURIComponent(B);
      }).join("/"), M = new URL(h, Z);
      if (M.origin !== Z.origin || M.username || M.password || M.search || M.hash || !M.pathname.startsWith(Z.pathname))
        return null;
      return M.href;
    }
    async function r(Z) {
      if (!Z.ok)
        throw Error("marker request failed");
      var E = Z.headers.get("content-length");
      if (E && /^\d+$/.test(E) && Number(E) > H) {
        if (Z.body)
          await Z.body.cancel("marker is too large");
        throw Error("marker is too large");
      }
      if (!Z.body || typeof Z.body.getReader !== "function")
        throw Error("bounded marker stream is unavailable");
      var h = Z.body.getReader(), M = [], B = 0;
      try {
        for (;; ) {
          var C = await h.read();
          if (C.done)
            break;
          if (B += C.value.byteLength, B > H)
            throw await h.cancel("marker is too large"), Error("marker is too large");
          M.push(C.value);
        }
      } finally {
        h.releaseLock();
      }
      var V = new Uint8Array(B), S = 0;
      M.forEach(function(T) {
        V.set(T, S), S += T.byteLength;
      });
      var I = new TextDecoder("utf-8", { fatal: !0 }).decode(V);
      return e(I), JSON.parse(I);
    }
    function e(Z) {
      var E = 0;
      function h() {
        while (/\s/.test(Z[E] || ""))
          E += 1;
      }
      function M() {
        if (Z[E] !== '"')
          throw Error("JSON string is invalid");
        var C = E++;
        while (E < Z.length) {
          var V = Z[E++];
          if (V === '"')
            return JSON.parse(Z.slice(C, E));
          if (V === "\\")
            E += 1;
        }
        throw Error("JSON string is incomplete");
      }
      function B() {
        if (h(), Z[E] === "{") {
          E += 1, h();
          var C = new Set;
          if (Z[E] === "}") {
            E += 1;
            return;
          }
          for (;; ) {
            h();
            var V = M();
            if (C.has(V))
              throw Error("JSON object has a duplicate field");
            if (C.add(V), h(), Z[E++] !== ":")
              throw Error("JSON object is invalid");
            B(), h();
            var S = Z[E++];
            if (S === "}")
              return;
            if (S !== ",")
              throw Error("JSON object is invalid");
          }
        }
        if (Z[E] === "[") {
          if (E += 1, h(), Z[E] === "]") {
            E += 1;
            return;
          }
          for (;; ) {
            B(), h();
            var S = Z[E++];
            if (S === "]")
              return;
            if (S !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (Z[E] === '"') {
          M();
          return;
        }
        var I = Z.slice(E).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!I)
          throw Error("JSON value is invalid");
        E += I[0].length;
      }
      if (B(), h(), E !== Z.length)
        throw Error("JSON has trailing content");
    }
    function B0(Z, E, h, M) {
      if (!f(Z))
        throw Error("marker is invalid");
      var B = Z, C = E.pathname.split("/").filter(Boolean), V = C[C.length - 1] || "";
      if (B.schema !== "airplan-upload" || B.version !== 6 || B.kind !== "document" || B.directory !== V || !/^[a-z2-7]{26}$/.test(B.directory) || !c0(B.created_at) || B.format !== "md" || !l0(B.slug) || B.entrypoint !== B.slug + ".html" || !f(B.producer) || B.producer.name !== "airplan" || typeof B.producer.version !== "string" || B.producer.version.trim() !== B.producer.version || B.producer.version === "" || !x(B.render) || !f(B.revision) || B.revision.number !== h.number || B.revision.chain_id !== M || (B.revision.number === 1 ? B.revision.previous_url !== void 0 : typeof B.revision.previous_url !== "string" || !p0(B.revision.previous_url)) || !Array.isArray(B.objects) || !Array.isArray(B.pages) || B.pages.length === 0)
        throw Error("marker identity is invalid");
      var S = l(E, B.entrypoint);
      if (S !== h.safeURL)
        throw Error("marker entrypoint is invalid");
      if (B.title !== void 0 && typeof B.title !== "string" || B.repo !== void 0 && !i0(B.repo) || B.objects.length === 0 || B.pages.length > 100)
        throw Error("marker shape is invalid");
      var I = o0(B), T = new Set, i = new Set, k = new Set, g = new Map;
      if (B.pages.forEach(function(q, G) {
        if (!f(q) || !v(q.path) || T.has(q.path) || i.has(q.path.toLowerCase()) || q.format !== "md" && q.format !== "txt" || typeof q.lang !== "string" || q.title !== void 0 && typeof q.title !== "string" || !v(q.page) || !v(q.source))
          throw Error("marker page descriptor is invalid");
        var y = _(q.path, q.format), P = q.path;
        if (G === 0) {
          if (y = B.entrypoint, P = B.slug + ".md", q.format !== B.format)
            throw Error("marker entry format is invalid");
        }
        if (q.page !== y || q.source !== P)
          throw Error("marker generated page mapping is invalid");
        var N = l(E, q.page);
        if (!N || k.has(N))
          throw Error("marker page object is invalid");
        if (!l(E, q.source))
          throw Error("marker source object is invalid");
        if (I.get(q.page) !== "page" || I.get(q.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var j = B.objects.find(function(p) {
          return p.name === q.source;
        }).content_type;
        if (q.format === "md" && j !== "text/markdown; charset=utf-8" || q.format === "txt" && j !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        T.add(q.path), i.add(q.path.toLowerCase()), k.add(N), g.set(q.path, N);
      }), L(i))
        throw Error("marker page paths conflict");
      if (!T.has(B.pages[0].path) || g.get(B.pages[0].path) !== S)
        throw Error("marker entry page is invalid");
      if (k.size !== B.pages.length || Array.from(I.values()).filter(function(q) {
        return q === "source";
      }).length !== B.pages.length)
        throw Error("marker page inventory is invalid");
      return g;
    }
    function _(Z, E) {
      if (E !== "md")
        return Z + ".html";
      var h = Z.lastIndexOf("/"), M = Z.lastIndexOf(".");
      return (M > h ? Z.slice(0, M) : Z) + ".html";
    }
    function x(Z) {
      if (!f(Z) || !f(Z.template) || !f(Z.themes) || !Number.isInteger(Z.generation) || Z.generation <= 0 || typeof Z.indexable !== "boolean" || typeof Z.no_external_assets !== "boolean" || !Z.template || Z.template.kind !== "builtin" && Z.template.kind !== "custom" || Z.mermaid_url !== void 0 && !d0(Z.mermaid_url) || !Z.themes)
        return !1;
      if (Z.template.kind === "builtin" && Z.template.sha256 !== void 0 || Z.template.kind === "custom" && !u(Z.template.sha256))
        return !1;
      return R(Z.themes.default_light) && R(Z.themes.default_dark) && u(Z.themes.catalog_sha256);
    }
    function R(Z) {
      return typeof Z === "string" && new TextEncoder().encode(Z).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(Z);
    }
    function L(Z) {
      for (var E of Z) {
        var h = E.indexOf("/");
        while (h >= 0) {
          if (Z.has(E.slice(0, h)))
            return !0;
          h = E.indexOf("/", h + 1);
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
    function l0(Z) {
      return typeof Z === "string" && Z.length <= 64 && /^[a-z0-9-]+$/.test(Z);
    }
    function c0(Z) {
      if (typeof Z !== "string")
        return !1;
      var E = Z.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/);
      if (!E)
        return !1;
      var h = Number(E[1]), M = Number(E[2]), B = Number(E[3]), C = Number(E[4]), V = Number(E[5]), S = Number(E[6]), I = h % 4 === 0 && (h % 100 !== 0 || h % 400 === 0), T = [31, I ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return M >= 1 && M <= 12 && B >= 1 && B <= T[M - 1] && C <= 23 && V <= 59 && S <= 59;
    }
    function i0(Z) {
      if (typeof Z !== "string" || Z === "" || Z.trim() !== Z)
        return !1;
      try {
        var E = new URL(Z);
        if (E.protocol !== "https:" || E.username || E.password || E.port || E.search || E.hash)
          return !1;
        var h = E.pathname.replace(/^\/+|\/+$/g, "").split("/");
        if (h.length !== 2)
          return !1;
        var M = h[0], B = h[1].replace(/\.git$/, "");
        if (!M || !B || M === "." || M === ".." || B === "." || B === ".." || /[?#@:\\]/.test(M + B))
          return !1;
        return Z === "https://" + E.hostname.toLowerCase() + "/" + M + "/" + B;
      } catch {
        return !1;
      }
    }
    function g0(Z) {
      return typeof Z === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(Z);
    }
    function p0(Z) {
      try {
        var E = new URL(Z);
        return (E.protocol === "https:" || E.protocol === "http:") && !E.username && !E.password && !E.search && !E.hash && E.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function d0(Z) {
      if (typeof Z !== "string")
        return !1;
      try {
        var E = new URL(Z);
        return E.protocol === "https:" && !!E.host && !E.username && !E.password && !E.hash;
      } catch {
        return !1;
      }
    }
    function o0(Z) {
      var E = new Map, h = new Set, M = 0, B = 0, C = 0, V = 0;
      if (Z.objects.forEach(function(S) {
        if (!f(S) || !v(S.name) && S.name !== ".airplan-changes.diff" || E.has(S.name) || h.has(S.name.toLowerCase()) || !Number.isSafeInteger(S.bytes) || S.bytes < 0 || !u(S.sha256) || !g0(S.content_type))
          throw Error("marker object inventory is invalid");
        if (S.role === "page") {
          if (B += 1, S.bytes <= 0 || S.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (S.role === "source") {
          if (C += 1, S.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (S.role === "asset")
          V += 1;
        else if (S.role === "diff") {
          if (M += 1, S.name !== ".airplan-changes.diff" || S.bytes <= 0 || S.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        E.set(S.name, S.role), h.add(S.name.toLowerCase());
      }), L(h))
        throw Error("marker object paths conflict");
      if (B !== Z.pages.length || C !== Z.pages.length || B + V > 100 || (Z.revision.number === 1 ? M !== 0 : M !== 1))
        throw Error("marker object counts are invalid");
      return E;
    }
    function s0(Z, E) {
      var h = window.location.hash;
      if (h === "#airplan-all-changes")
        return Z + h;
      if (!E)
        return Z;
      return E + (h && h !== "#airplan-all-changes" ? h : "");
    }
    function t0(Z) {
      var E = A.querySelector('meta[name="airplan-revision"]'), h = E ? Number(E.content) : Number(Z.current_revision);
      if (!Number.isInteger(h) || h <= 0 || Z.current_revision !== h || !Number.isInteger(Z.latest_revision) || !Number.isInteger(Z.last_assigned_revision) || !Array.isArray(Z.revisions) || Z.revisions.length === 0 || Z.last_assigned_revision !== Z.revisions.length || !/^[a-z2-7]{26}$/.test(Z.chain_id) || O && O.content !== Z.chain_id)
        throw Error("revision identity is invalid");
      var M = !1, B = 0, C = Z.revisions.filter(function(G) {
        if (!G || !Number.isInteger(G.number) || G.number !== B + 1)
          return M = !0, !1;
        if (B = G.number, G.deleted)
          return !1;
        if (G.safeURL = a(G.url, !1), !G.safeURL)
          return M = !0, !1;
        if (G.number > 1) {
          var y = a(G.diff_url, !0);
          if (!y || new URL(y).pathname.replace(/[^/]+$/, "") !== new URL(G.safeURL).pathname.replace(/[^/]+$/, ""))
            return M = !0, !1;
        }
        return !0;
      });
      if (M || Z.revisions[0].number !== 1 || !C.some(function(G) {
        return G.number === h;
      }))
        throw Error("revision entries are invalid");
      var V = C.find(function(G) {
        return G.number === h;
      }), S = new URL(window.location.href);
      if (S.search = "", S.hash = "", !V || !F || new URL(V.safeURL || "").pathname.replace(/[^/]+$/, "") !== F.pathname || !S.pathname.startsWith(F.pathname))
        throw Error("current revision URL is invalid");
      var I = Math.max.apply(null, C.map(function(G) {
        return G.number;
      }));
      if (I !== Z.latest_revision)
        throw Error("latest is invalid");
      var T = Array.from(A.querySelectorAll("[data-revision-controls]")), i = Array.from(A.querySelectorAll("[data-revision-heading]"));
      if (i.length === 0) {
        if (T.length === 0)
          throw Error("revision controls are unavailable");
        var k = A.createElement("p");
        k.className = "revision-heading", k.setAttribute("data-revision-heading", ""), T[0].appendChild(k), i.push(k);
      }
      T.forEach(function(G) {
        G.hidden = !1;
      });
      var g = h < I, q = g ? "Revision " + h + " of " + I : "Revision " + h + " (Latest)";
      i.forEach(function(G) {
        var y = A.createElement("span");
        y.className = "revision-picker-label", y.textContent = q, y.setAttribute("aria-hidden", "true");
        var P = A.createElement("select");
        P.setAttribute("aria-label", "Document revision"), C.forEach(function(N) {
          var j = A.createElement("option");
          j.value = N.safeURL || "", j.textContent = N.number === I ? "Revision " + N.number + " (Latest)" : "Revision " + N.number + " of " + I, j.selected = N.number === h, P.appendChild(j);
        }), P.addEventListener("change", function() {
          var N = P.selectedIndex;
          if (N < 0 || N >= C.length)
            return;
          var j = C[N], p = j.safeURL || "";
          if (window.location.hash === "#airplan-all-changes") {
            window.location.assign(p + (j.number > 1 ? "#airplan-all-changes" : ""));
            return;
          }
          var Z1 = K ? new URL(K.content, window.location.href).href : "";
          if (!$ || S.href === Z1 || !O) {
            window.location.assign(p);
            return;
          }
          G.setAttribute("aria-busy", "true"), P.disabled = !0;
          var T0 = new URL("./", p), u0 = new URL(".airplan.json", T0);
          u0.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(u0, { cache: "no-store", credentials: "same-origin" }).then(r).then(function(E1) {
            var h1 = B0(E1, T0, j, O.content);
            window.location.assign(s0(p, h1.get($.content) || null));
          }).catch(function() {
            console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(p);
          });
        }), G.replaceChildren(y, P), G.classList.add("is-picker"), G.classList.toggle("is-stale", g);
      }), A.body.classList.toggle("airplan-stale-revision", g);
    }
    if (U) {
      var O0 = new URL(U.content, window.location.href);
      O0.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(O0, { cache: "no-store", credentials: "same-origin" }).then(function(Z) {
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
        t0(Z), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: Z
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var M0 = A.createElement("div");
    M0.className = "sr-status", M0.setAttribute("aria-live", "polite"), A.body.appendChild(M0);
    var d = null;
    function a0() {
      if (d !== null)
        return;
      d = Array.from(A.querySelectorAll("details:not([open])")), d.forEach(function(Z) {
        Z.open = !0;
      });
    }
    function r0() {
      if (d === null)
        return;
      d.forEach(function(Z) {
        Z.open = !1;
      }), d = null;
    }
    window.addEventListener("beforeprint", a0), window.addEventListener("afterprint", r0);
    function x0(Z, E, h) {
      M0.textContent = E;
      var M = Z.querySelector(".action-label"), B = M ? M.textContent : "";
      if (M)
        M.textContent = h ? "Copied" : "Failed";
      Z.classList.add(h ? "is-copied" : "is-failed"), Z.disabled = !0, setTimeout(function() {
        if (Z.classList.remove("is-copied", "is-failed"), Z.disabled = !1, M)
          M.textContent = B;
      }, 1200);
    }
    function F0(Z, E) {
      if (!navigator.clipboard) {
        x0(E, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(Z).then(function() {
        x0(E, "Copied!", !0);
      }, function() {
        x0(E, "Copy failed", !1);
      });
    }
    var W0 = A.getElementById("pages"), D = A.querySelector(".pages-trigger"), Y = null, H0 = window.matchMedia("(max-width: 78rem)"), m = function() {};
    function V0() {
      return Y ? Y.matches(":popover-open") : !1;
    }
    function Z0(Z) {
      if (!Y || !V0())
        return;
      if (Y.hidePopover(), Z && D && H0.matches)
        setTimeout(function() {
          D.focus();
        }, 0);
    }
    if (W0 && D) {
      var L0 = W0.querySelector(".pages-list");
      if (L0) {
        var z0 = A.createElement("div");
        if ("popover" in z0 && typeof z0.showPopover === "function") {
          let Z = function() {
            if (!D || !Y)
              return;
            var E = D.getBoundingClientRect(), h = D.closest(".toolbar"), M = h ? h.getBoundingClientRect().bottom : E.bottom;
            Y.style.setProperty("--pages-left", Math.max(16, E.left) + "px"), Y.style.setProperty("--pages-top", M + "px"), Y.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, E.left) - 16) + "px");
          };
          Y = z0, Y.className = "pages-popover", Y.id = "pages-popover", Y.setAttribute("popover", "auto");
          var E0 = A.createElement("nav");
          E0.className = "pages-popover-nav", E0.setAttribute("aria-label", "Pages"), E0.appendChild(L0.cloneNode(!0)), Y.appendChild(E0), D.setAttribute("popovertarget", Y.id), D.popoverTargetElement = Y, Y.addEventListener("beforetoggle", function(E) {
            if (E.newState !== "open")
              return;
            m(), Z();
          }), Y.addEventListener("toggle", function(E) {
            var h = E.newState === "open";
            if (D.setAttribute("aria-expanded", h ? "true" : "false"), A.body.classList.toggle("pages-popover-open", h), h) {
              var M = Y.querySelector('[aria-current="page"]');
              if (M)
                M.scrollIntoView({ block: "nearest" });
            }
            c();
          }), E0.querySelectorAll("a").forEach(function(E) {
            E.addEventListener("click", function() {
              Z0(!1);
            });
          }), H0.addEventListener("change", function() {
            if (!H0.matches)
              Z0(!1);
          }), window.addEventListener("resize", function() {
            if (V0())
              Z();
          }), D.hidden = !1, D.setAttribute("aria-expanded", "false"), A.body.appendChild(Y), A.body.classList.add("pages-popover-ready");
        }
      }
    }
    var o = A.getElementById("source"), S0 = A.getElementById("changes"), _0 = A.querySelector("[data-airplan-all-changes]"), b = A.getElementById("toc"), J = null, z = null, N0 = window.matchMedia("(max-width: 78rem)");
    m = function() {
      if (z && z.open)
        z.close();
    };
    function c() {
      if (!b || !J || !z)
        return;
      var Z = N0.matches && !X.hidden && !z.open && !V0();
      if (J.classList.toggle("is-visible", Z), J.tabIndex = Z ? 0 : -1, J.setAttribute("aria-hidden", Z ? "false" : "true"), z.open && (!N0.matches || X.hidden))
        m();
    }
    function R0(Z) {
      if (Z0(!1), m(), X.hidden = Z !== "rendered", o)
        o.hidden = Z !== "source";
      if (S0)
        S0.hidden = Z !== "changes";
      if (b)
        b.hidden = Z !== "rendered";
      A.querySelectorAll(".viewtoggle button").forEach(function(E) {
        var h = E.dataset.view === Z;
        E.classList.toggle("active", h), E.setAttribute("aria-pressed", h ? "true" : "false");
      }), c();
    }
    A.querySelectorAll(".viewtoggle button").forEach(function(Z) {
      Z.addEventListener("click", function() {
        R0(Z.dataset.view || "rendered");
      });
    });
    var K0 = !1;
    A.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(Z) {
      Z.addEventListener("click", function() {
        K0 = new URL(Z.href).pathname === window.location.pathname;
      });
    });
    function U0() {
      var Z = window.location.hash === "#airplan-all-changes" && !!_0;
      if (Z0(!1), m(), A.body.classList.toggle("all-changes-active", Z), _0)
        _0.hidden = !Z;
      if (Z) {
        if (X.hidden = !0, o)
          o.hidden = !0;
        if (S0)
          S0.hidden = !0;
        if (b)
          b.hidden = !0;
        if (K0)
          _0.querySelector("h1")?.focus();
      } else
        R0("rendered");
      K0 = !1, c();
    }
    if (window.addEventListener("hashchange", U0), U0(), b) {
      let Z = function() {
        if (h0.length === 0) {
          c();
          return;
        }
        var h = 0;
        if (e0.forEach(function(B, C) {
          if (B && B.getBoundingClientRect().top <= 128)
            h = C;
        }), window.innerHeight + window.scrollY >= A.documentElement.scrollHeight - 2)
          h = h0.length - 1;
        var M = h0[h].getAttribute("href");
        Q0.forEach(function(B) {
          var C = B.getAttribute("href") === M;
          if (B.classList.toggle("active", C), C)
            B.setAttribute("aria-current", "location");
          else
            B.removeAttribute("aria-current");
        }), c();
      }, E = function() {
        if (X0)
          return;
        X0 = !0, window.requestAnimationFrame(function() {
          X0 = !1, Z();
        });
      };
      var h0 = Array.from(b.querySelectorAll('a[href^="#"]')), f0 = b.querySelector(".toc-list");
      if (f0)
        if (z = A.createElement("dialog"), typeof z.showModal === "function") {
          z.className = "toc-dialog", z.id = "toc-dialog", z.setAttribute("aria-labelledby", "toc-dialog-title");
          var C0 = A.createElement("div");
          C0.className = "toc-dialog-panel";
          var q0 = A.createElement("div");
          q0.className = "toc-dialog-header";
          var w0 = A.createElement("h2");
          w0.className = "toc-dialog-title", w0.id = "toc-dialog-title", w0.textContent = "Contents";
          var s = A.createElement("button");
          s.className = "toc-dialog-close", s.type = "button", s.setAttribute("aria-label", "Close table of contents"), s.innerHTML = P0, q0.appendChild(w0), q0.appendChild(s);
          var A0 = A.createElement("nav");
          A0.className = "toc-dialog-nav", A0.setAttribute("aria-label", "Table of contents"), A0.appendChild(f0.cloneNode(!0)), C0.appendChild(q0), C0.appendChild(A0), z.appendChild(C0), J = A.createElement("button"), J.className = "toc-trigger", J.type = "button", J.tabIndex = -1, J.setAttribute("aria-label", "Open table of contents"), J.setAttribute("aria-controls", "toc-dialog"), J.setAttribute("aria-haspopup", "dialog"), J.setAttribute("aria-hidden", "true"), J.innerHTML = k0, A.body.appendChild(J), A.body.appendChild(z), A.body.classList.add("toc-dialog-ready"), J.addEventListener("click", function() {
            Z0(!1), z.showModal(), A.body.classList.add("toc-dialog-open"), c();
            var h = z.querySelector("a.active");
            if (h)
              h.scrollIntoView({ block: "nearest" });
          }), s.addEventListener("click", m), z.addEventListener("click", function(h) {
            if (h.target === z)
              m();
          }), z.addEventListener("keydown", function(h) {
            if (h.key === "Escape")
              h.preventDefault(), m();
          }), z.addEventListener("close", function() {
            if (A.body.classList.remove("toc-dialog-open"), c(), J.classList.contains("is-visible"))
              setTimeout(function() {
                J.focus();
              }, 50);
          }), A0.querySelectorAll("a").forEach(function(h) {
            h.addEventListener("click", m);
          });
        } else
          z = null;
      var Q0 = h0.slice();
      if (z)
        Q0 = Q0.concat(Array.from(z.querySelectorAll('a[href^="#"]')));
      var e0 = h0.map(function(h) {
        return A.getElementById((h.getAttribute("href") || "").slice(1));
      }), X0 = !1;
      A.addEventListener("scroll", E, { passive: !0 }), window.addEventListener("resize", Z), Z();
    }
    var G0 = A.querySelector(".top-controls");
    function Y0() {
      var Z = G0 ? G0.getBoundingClientRect().height : 0;
      A.documentElement.style.setProperty("--airplan-sticky-height", Z + "px");
    }
    if (G0) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(Y0).observe(G0);
      window.addEventListener("resize", Y0), Y0();
    }
    let J0 = A.querySelector(".copy-source");
    if (J0 && o)
      J0.addEventListener("click", function() {
        var Z = o.querySelector("pre");
        F0(Z ? Z.textContent : "", J0);
      });
    X.querySelectorAll("pre").forEach(function(Z) {
      if (Z.classList.contains("mermaid"))
        return;
      var E = A.createElement("div");
      E.className = "codewrap", Z.parentNode?.insertBefore(E, Z), E.appendChild(Z);
      var h = A.createElement("button");
      h.className = "codecopy", h.type = "button", h.setAttribute("aria-label", "Copy code"), h.title = "Copy code", h.innerHTML = v0 + n0 + b0, h.addEventListener("click", function() {
        var M = Z.querySelector("code");
        F0((M || Z).textContent, h);
      }), E.appendChild(h);
    });
  })();
})();
