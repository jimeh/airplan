(() => {
  function E0(h) {
    return h === "system" || h === "light" || h === "dark";
  }
  function V1(h, V) {
    try {
      return h?.getItem(V) ?? null;
    } catch {
      return null;
    }
  }
  function s(h, V, $) {
    try {
      if ($ === null)
        h?.removeItem(V);
      else
        h?.setItem(V, $);
    } catch {}
  }
  function D1(h, V, $) {
    let z = V1($, "airplan-color-mode");
    if (z === null) {
      let W = V1($, "airplan-theme");
      if (z = W === "light" || W === "dark" ? W : "system", z !== "system")
        s($, "airplan-color-mode", z);
    }
    let f = E0(z) ? z : "system", x = new Set(h.themes.map((W) => W.id)), G = V1($, "airplan-light-theme"), w = V1($, "airplan-dark-theme"), Y = G !== null && x.has(G) ? G : h.defaultLight, F = w !== null && x.has(w) ? w : h.defaultDark;
    return O1(h, f, Y, F, V);
  }
  function O1(h, V, $, z, f) {
    let x = new Map(h.themes.map((u) => [u.id, u])), G = x.has($) ? $ : h.defaultLight, w = x.has(z) ? z : h.defaultDark, Y = V === "system" ? f ? "dark" : "light" : V, F = Y === "light" ? G : w, W = x.get(F)?.variant ?? Y;
    return { mode: V, resolvedMode: Y, lightTheme: G, darkTheme: w, theme: F, variant: W };
  }
  function j1(h, V) {
    if (V === "system")
      s(h, "airplan-color-mode", null), s(h, "airplan-theme", null);
    else
      s(h, "airplan-color-mode", V), s(h, "airplan-theme", V);
  }
  function u1(h, V, $) {
    s(h, V === "light" ? "airplan-light-theme" : "airplan-dark-theme", $);
  }
  function y1(h) {
    return {
      mode: h.mode,
      resolvedMode: h.resolvedMode,
      theme: h.theme,
      variant: h.variant
    };
  }

  (function() {
    let h = document, V = h.documentElement;
    h.querySelectorAll(".js-only").forEach((C) => {
      C.hidden = !1;
    });
    let $ = window.__AIRPLAN_THEME_CATALOG__;
    if (!$)
      return;
    let z = $, f = window.matchMedia("(prefers-color-scheme: dark)"), x;
    try {
      x = window.localStorage;
    } catch {}
    let G = window.__airplanThemeState ?? D1(z, f.matches, x), w = h.querySelector("[data-airplan-appearance-trigger]"), Y = h.querySelector("[data-airplan-appearance-panel]"), F = h.querySelector('select[data-airplan-theme-slot="light"]'), W = h.querySelector('select[data-airplan-theme-slot="dark"]'), u = Array.from(h.querySelectorAll("[data-airplan-color-mode]"));
    function o(C) {
      if (!C || C.options.length > 0)
        return;
      for (let [K, m] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let R = h.createElement("optgroup");
        R.label = m;
        for (let P of z.themes) {
          if (P.variant !== K)
            continue;
          let l = h.createElement("option");
          l.value = P.id, l.textContent = P.name, R.append(l);
        }
        if (R.children.length > 0)
          C.append(R);
      }
    }
    o(F), o(W);
    function y(C, K = !0) {
      if (G = C, window.__airplanThemeState = G, V.dataset.airplanMode = G.mode, V.dataset.airplanResolvedMode = G.resolvedMode, V.dataset.airplanTheme = G.theme, V.dataset.airplanThemeVariant = G.variant, u.forEach((m) => {
        let R = m.dataset.airplanColorMode === G.mode;
        m.classList.toggle("active", R), m.setAttribute("aria-pressed", String(R));
      }), F)
        F.value = G.lightTheme;
      if (W)
        W.value = G.darkTheme;
      if (K)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: y1(G) }));
    }
    function n(C = {}) {
      y(O1(z, C.mode ?? G.mode, C.lightTheme ?? G.lightTheme, C.darkTheme ?? G.darkTheme, f.matches));
    }
    function t(C, K = !1) {
      if (!Y || !w)
        return;
      if (Y.hidden = !C, w.setAttribute("aria-expanded", String(C)), C)
        Y.querySelector("button,select")?.focus();
      else if (K)
        w.focus();
    }
    w?.addEventListener("click", () => t(Boolean(Y?.hidden ?? !0))), u.forEach((C) => C.addEventListener("click", () => {
      let K = C.dataset.airplanColorMode;
      if (!K)
        return;
      j1(x, K), n({ mode: K });
    }));
    function A1(C, K) {
      u1(x, C, K.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: K.value } })), n(C === "light" ? { lightTheme: K.value } : { darkTheme: K.value });
    }
    F?.addEventListener("change", () => A1("light", F)), W?.addEventListener("change", () => A1("dark", W)), f.addEventListener("change", () => {
      if (G.mode === "system")
        n();
    }), h.addEventListener("keydown", (C) => {
      if (C.key === "Escape" && Y && !Y.hidden)
        C.preventDefault(), t(!1, !0);
    }), h.addEventListener("pointerdown", (C) => {
      if (!Y || Y.hidden || !w)
        return;
      let K = C.target;
      if (!(K instanceof Node) || Y.contains(K) || w.contains(K))
        return;
      let R = (K instanceof Element ? K : K.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), P = Y.contains(h.activeElement) && !R;
      if (t(!1), P)
        setTimeout(() => {
          if (h.activeElement === h.body || Y.contains(h.activeElement))
            w.focus();
        });
    }), y(G, !1);
  })();

  var m1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', P1 = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', v1 = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>';
  var b1 = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';
  var k1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>';

  (function() {
    var h = document, V = 262144;
    let $ = h.getElementById("rendered");
    if (!$)
      return;
    let z = $;
    var f = h.querySelector('meta[name="airplan-versions"]'), x = h.querySelector('meta[name="airplan-revision-chain"]'), G = h.querySelector('meta[name="airplan-page-path"]'), w = h.querySelector('meta[name="airplan-entrypoint"]'), Y = f ? new URL(f.content, window.location.href) : null, F = Y ? new URL("./", Y) : null, W = F ? F.pathname.split("/").filter(Boolean) : [], u = W.slice(0, -1);
    function o(Z, E) {
      if (typeof Z !== "string")
        return null;
      try {
        var A = new URL(Z);
        if (A.origin !== window.location.origin || A.username || A.password || A.search || A.hash)
          return null;
        var S = A.pathname.split("/").filter(Boolean);
        if (S.length !== u.length + 2 || !u.every(function(H, Q) {
          return S[Q] === H;
        }) || !/^[a-z2-7]{26}$/.test(S[S.length - 2]))
          return null;
        var B = S[S.length - 1];
        if (E ? B !== ".airplan-changes.diff" : !B.endsWith(".html"))
          return null;
        return A.href;
      } catch {
        return null;
      }
    }
    function y(Z) {
      if (typeof Z !== "string" || Z === "" || Z.startsWith("/") || Z.includes("\\"))
        return !1;
      var E = Z.split("/");
      return E.every(function(A) {
        var S = A.toLowerCase(), B = Array.from(A).some(function(H) {
          var Q = H.codePointAt(0) || 0;
          return Q < 32 || Q === 127;
        });
        if (!A || A === "." || A === ".." || S.startsWith(".airplan-") || S === ".airplan.json" || B || /[. ]$/.test(A) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(A))
          return !1;
        return !0;
      });
    }
    function n(Z, E) {
      if (!y(E))
        return null;
      var A = String(E).split("/").map(function(B) {
        return encodeURIComponent(B);
      }).join("/"), S = new URL(A, Z);
      if (S.origin !== Z.origin || S.username || S.password || S.search || S.hash || !S.pathname.startsWith(Z.pathname))
        return null;
      return S.href;
    }
    async function t(Z) {
      if (!Z.ok)
        throw Error("marker request failed");
      var E = Z.headers.get("content-length");
      if (E && /^\d+$/.test(E) && Number(E) > V) {
        if (Z.body)
          await Z.body.cancel("marker is too large");
        throw Error("marker is too large");
      }
      if (!Z.body || typeof Z.body.getReader !== "function")
        throw Error("bounded marker stream is unavailable");
      var A = Z.body.getReader(), S = [], B = 0;
      try {
        for (;; ) {
          var H = await A.read();
          if (H.done)
            break;
          if (B += H.value.byteLength, B > V)
            throw await A.cancel("marker is too large"), Error("marker is too large");
          S.push(H.value);
        }
      } finally {
        A.releaseLock();
      }
      var Q = new Uint8Array(B), _ = 0;
      S.forEach(function(N) {
        Q.set(N, _), _ += N.byteLength;
      });
      var O = new TextDecoder("utf-8", { fatal: !0 }).decode(Q);
      return A1(O), JSON.parse(O);
    }
    function A1(Z) {
      var E = 0;
      function A() {
        while (/\s/.test(Z[E] || ""))
          E += 1;
      }
      function S() {
        if (Z[E] !== '"')
          throw Error("JSON string is invalid");
        var H = E++;
        while (E < Z.length) {
          var Q = Z[E++];
          if (Q === '"')
            return JSON.parse(Z.slice(H, E));
          if (Q === "\\")
            E += 1;
        }
        throw Error("JSON string is incomplete");
      }
      function B() {
        if (A(), Z[E] === "{") {
          E += 1, A();
          var H = new Set;
          if (Z[E] === "}") {
            E += 1;
            return;
          }
          for (;; ) {
            A();
            var Q = S();
            if (H.has(Q))
              throw Error("JSON object has a duplicate field");
            if (H.add(Q), A(), Z[E++] !== ":")
              throw Error("JSON object is invalid");
            B(), A();
            var _ = Z[E++];
            if (_ === "}")
              return;
            if (_ !== ",")
              throw Error("JSON object is invalid");
          }
        }
        if (Z[E] === "[") {
          if (E += 1, A(), Z[E] === "]") {
            E += 1;
            return;
          }
          for (;; ) {
            B(), A();
            var _ = Z[E++];
            if (_ === "]")
              return;
            if (_ !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (Z[E] === '"') {
          S();
          return;
        }
        var O = Z.slice(E).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!O)
          throw Error("JSON value is invalid");
        E += O[0].length;
      }
      if (B(), A(), E !== Z.length)
        throw Error("JSON has trailing content");
    }
    function C(Z, E, A, S) {
      if (!v(Z))
        throw Error("marker is invalid");
      var B = Z, H = E.pathname.split("/").filter(Boolean), Q = H[H.length - 1] || "";
      if (B.schema !== "airplan-upload" || B.version !== 6 || B.kind !== "document" || B.directory !== Q || !/^[a-z2-7]{26}$/.test(B.directory) || !l1(B.created_at) || B.format !== "md" || !n1(B.slug) || B.entrypoint !== B.slug + ".html" || !v(B.producer) || B.producer.name !== "airplan" || typeof B.producer.version !== "string" || B.producer.version.trim() !== B.producer.version || B.producer.version === "" || !m(B.render) || !v(B.revision) || B.revision.number !== A.number || B.revision.chain_id !== S || (B.revision.number === 1 ? B.revision.previous_url !== void 0 : typeof B.revision.previous_url !== "string" || !g1(B.revision.previous_url)) || !Array.isArray(B.objects) || !Array.isArray(B.pages) || B.pages.length === 0)
        throw Error("marker identity is invalid");
      var _ = n(E, B.entrypoint);
      if (_ !== A.safeURL)
        throw Error("marker entrypoint is invalid");
      if (B.title !== void 0 && typeof B.title !== "string" || B.repo !== void 0 && !c1(B.repo) || B.objects.length === 0 || B.pages.length > 100)
        throw Error("marker shape is invalid");
      var O = d1(B), N = new Set, U = new Set, i = new Set, E1 = new Map;
      if (B.pages.forEach(function(M, k) {
        if (!v(M) || !y(M.path) || N.has(M.path) || U.has(M.path.toLowerCase()) || M.format !== "md" && M.format !== "txt" || typeof M.lang !== "string" || M.title !== void 0 && typeof M.title !== "string" || !y(M.page) || !y(M.source))
          throw Error("marker page descriptor is invalid");
        var q = K(M.path, M.format), L = M.path;
        if (k === 0) {
          if (q = B.entrypoint, L = B.slug + ".md", M.format !== B.format)
            throw Error("marker entry format is invalid");
        }
        if (M.page !== q || M.source !== L)
          throw Error("marker generated page mapping is invalid");
        var D = n(E, M.page);
        if (!D || i.has(D))
          throw Error("marker page object is invalid");
        if (!n(E, M.source))
          throw Error("marker source object is invalid");
        if (O.get(M.page) !== "page" || O.get(M.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var C1 = B.objects.find(function(G1) {
          return G1.name === M.source;
        }).content_type;
        if (M.format === "md" && C1 !== "text/markdown; charset=utf-8" || M.format === "txt" && C1 !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        N.add(M.path), U.add(M.path.toLowerCase()), i.add(D), E1.set(M.path, D);
      }), P(U))
        throw Error("marker page paths conflict");
      if (!N.has(B.pages[0].path) || E1.get(B.pages[0].path) !== _)
        throw Error("marker entry page is invalid");
      if (i.size !== B.pages.length || Array.from(O.values()).filter(function(M) {
        return M === "source";
      }).length !== B.pages.length)
        throw Error("marker page inventory is invalid");
      return E1;
    }
    function K(Z, E) {
      if (E !== "md")
        return Z + ".html";
      var A = Z.lastIndexOf("/"), S = Z.lastIndexOf(".");
      return (S > A ? Z.slice(0, S) : Z) + ".html";
    }
    function m(Z) {
      if (!v(Z) || !v(Z.template) || !v(Z.themes) || !Number.isInteger(Z.generation) || Z.generation <= 0 || typeof Z.indexable !== "boolean" || typeof Z.no_external_assets !== "boolean" || !Z.template || Z.template.kind !== "builtin" && Z.template.kind !== "custom" || Z.mermaid_url !== void 0 && !p1(Z.mermaid_url) || !Z.themes)
        return !1;
      if (Z.template.kind === "builtin" && Z.template.sha256 !== void 0 || Z.template.kind === "custom" && !l(Z.template.sha256))
        return !1;
      return R(Z.themes.default_light) && R(Z.themes.default_dark) && l(Z.themes.catalog_sha256);
    }
    function R(Z) {
      return typeof Z === "string" && new TextEncoder().encode(Z).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(Z);
    }
    function P(Z) {
      for (var E of Z) {
        var A = E.indexOf("/");
        while (A >= 0) {
          if (Z.has(E.slice(0, A)))
            return !0;
          A = E.indexOf("/", A + 1);
        }
      }
      return !1;
    }
    function l(Z) {
      return typeof Z === "string" && /^[0-9a-f]{64}$/.test(Z);
    }
    function v(Z) {
      return !!Z && typeof Z === "object" && !Array.isArray(Z);
    }
    function n1(Z) {
      return typeof Z === "string" && Z.length <= 64 && /^[a-z0-9-]+$/.test(Z);
    }
    function l1(Z) {
      if (typeof Z !== "string")
        return !1;
      var E = Z.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/);
      if (!E)
        return !1;
      var A = Number(E[1]), S = Number(E[2]), B = Number(E[3]), H = Number(E[4]), Q = Number(E[5]), _ = Number(E[6]), O = A % 4 === 0 && (A % 100 !== 0 || A % 400 === 0), N = [31, O ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return S >= 1 && S <= 12 && B >= 1 && B <= N[S - 1] && H <= 23 && Q <= 59 && _ <= 59;
    }
    function c1(Z) {
      if (typeof Z !== "string" || Z === "" || Z.trim() !== Z)
        return !1;
      try {
        var E = new URL(Z);
        if (E.protocol !== "https:" || E.username || E.password || E.port || E.search || E.hash)
          return !1;
        var A = E.pathname.replace(/^\/+|\/+$/g, "").split("/");
        if (A.length !== 2)
          return !1;
        var S = A[0], B = A[1].replace(/\.git$/, "");
        if (!S || !B || S === "." || S === ".." || B === "." || B === ".." || /[?#@:\\]/.test(S + B))
          return !1;
        return Z === "https://" + E.hostname.toLowerCase() + "/" + S + "/" + B;
      } catch {
        return !1;
      }
    }
    function i1(Z) {
      return typeof Z === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(Z);
    }
    function g1(Z) {
      try {
        var E = new URL(Z);
        return (E.protocol === "https:" || E.protocol === "http:") && !E.username && !E.password && !E.search && !E.hash && E.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function p1(Z) {
      if (typeof Z !== "string")
        return !1;
      try {
        var E = new URL(Z);
        return E.protocol === "https:" && !!E.host && !E.username && !E.password && !E.hash;
      } catch {
        return !1;
      }
    }
    function d1(Z) {
      var E = new Map, A = new Set, S = 0, B = 0, H = 0, Q = 0;
      if (Z.objects.forEach(function(_) {
        if (!v(_) || !y(_.name) && _.name !== ".airplan-changes.diff" || E.has(_.name) || A.has(_.name.toLowerCase()) || !Number.isSafeInteger(_.bytes) || _.bytes < 0 || !l(_.sha256) || !i1(_.content_type))
          throw Error("marker object inventory is invalid");
        if (_.role === "page") {
          if (B += 1, _.bytes <= 0 || _.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (_.role === "source") {
          if (H += 1, _.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (_.role === "asset")
          Q += 1;
        else if (_.role === "diff") {
          if (S += 1, _.name !== ".airplan-changes.diff" || _.bytes <= 0 || _.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        E.set(_.name, _.role), A.add(_.name.toLowerCase());
      }), P(A))
        throw Error("marker object paths conflict");
      if (B !== Z.pages.length || H !== Z.pages.length || B + Q > 100 || (Z.revision.number === 1 ? S !== 0 : S !== 1))
        throw Error("marker object counts are invalid");
      return E;
    }
    function s1(Z, E) {
      var A = window.location.hash;
      if (A === "#airplan-all-changes")
        return Z + A;
      if (!E)
        return Z;
      return E + (A && A !== "#airplan-all-changes" ? A : "");
    }
    function o1(Z) {
      var E = h.querySelector('meta[name="airplan-revision"]'), A = E ? Number(E.content) : Number(Z.current_revision);
      if (!Number.isInteger(A) || A <= 0 || Z.current_revision !== A || !Number.isInteger(Z.latest_revision) || !Number.isInteger(Z.last_assigned_revision) || !Array.isArray(Z.revisions) || Z.revisions.length === 0 || Z.last_assigned_revision !== Z.revisions.length || !/^[a-z2-7]{26}$/.test(Z.chain_id) || x && x.content !== Z.chain_id)
        throw Error("revision identity is invalid");
      var S = !1, B = 0, H = Z.revisions.filter(function(q) {
        if (!q || !Number.isInteger(q.number) || q.number !== B + 1)
          return S = !0, !1;
        if (B = q.number, q.deleted)
          return !1;
        if (q.safeURL = o(q.url, !1), !q.safeURL)
          return S = !0, !1;
        if (q.number > 1) {
          var L = o(q.diff_url, !0);
          if (!L || new URL(L).pathname.replace(/[^/]+$/, "") !== new URL(q.safeURL).pathname.replace(/[^/]+$/, ""))
            return S = !0, !1;
        }
        return !0;
      });
      if (S || Z.revisions[0].number !== 1 || !H.some(function(q) {
        return q.number === A;
      }))
        throw Error("revision entries are invalid");
      var Q = H.find(function(q) {
        return q.number === A;
      }), _ = new URL(window.location.href);
      if (_.search = "", _.hash = "", !Q || !F || new URL(Q.safeURL || "").pathname.replace(/[^/]+$/, "") !== F.pathname || !_.pathname.startsWith(F.pathname))
        throw Error("current revision URL is invalid");
      var O = Math.max.apply(null, H.map(function(q) {
        return q.number;
      }));
      if (O !== Z.latest_revision)
        throw Error("latest is invalid");
      var N = h.querySelector("[data-revision-controls]"), U = h.querySelector("[data-revision-heading]");
      if (!U) {
        if (!N)
          throw Error("revision controls are unavailable");
        U = h.createElement("p"), U.className = "revision-heading", U.setAttribute("data-revision-heading", ""), N.appendChild(U);
      }
      if (N)
        N.hidden = !1;
      var i = A < O, E1 = i ? "Revision " + A + " of " + O : "Revision " + A + " (Latest)", M = h.createElement("span");
      M.className = "revision-picker-label", M.textContent = E1, M.setAttribute("aria-hidden", "true");
      var k = h.createElement("select");
      k.setAttribute("aria-label", "Document revision"), H.forEach(function(q) {
        var L = h.createElement("option");
        L.value = q.safeURL || "", L.textContent = q.number === O ? "Revision " + q.number + " (Latest)" : "Revision " + q.number + " of " + O, L.selected = q.number === A, k.appendChild(L);
      }), k.addEventListener("change", function() {
        var q = k.selectedIndex;
        if (q < 0 || q >= H.length)
          return;
        var L = H[q], D = L.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(D + (L.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var C1 = w ? new URL(w.content, window.location.href).href : "";
        if (!G || _.href === C1 || !x) {
          window.location.assign(D);
          return;
        }
        U.setAttribute("aria-busy", "true"), k.disabled = !0;
        var G1 = new URL("./", D), T1 = new URL(".airplan.json", G1);
        T1.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(T1, { cache: "no-store", credentials: "same-origin" }).then(t).then(function(e1) {
          var Z0 = C(e1, G1, L, x.content);
          window.location.assign(s1(D, Z0.get(G.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(D);
        });
      }), U.replaceChildren(M, k), U.classList.add("is-picker"), U.classList.toggle("is-stale", i), h.body.classList.toggle("airplan-stale-revision", i);
    }
    if (f) {
      var x1 = new URL(f.content, window.location.href);
      x1.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(x1, { cache: "no-store", credentials: "same-origin" }).then(function(Z) {
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
        o1(Z), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: Z
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var h1 = h.createElement("div");
    h1.className = "sr-status", h1.setAttribute("aria-live", "polite"), h.body.appendChild(h1);
    var g = null;
    function t1() {
      if (g !== null)
        return;
      g = Array.from(h.querySelectorAll("details:not([open])")), g.forEach(function(Z) {
        Z.open = !0;
      });
    }
    function a1() {
      if (g === null)
        return;
      g.forEach(function(Z) {
        Z.open = !1;
      }), g = null;
    }
    window.addEventListener("beforeprint", t1), window.addEventListener("afterprint", a1);
    function K1(Z, E, A) {
      h1.textContent = E;
      var S = Z.querySelector(".action-label"), B = S ? S.textContent : "";
      if (S)
        S.textContent = A ? "Copied" : "Failed";
      Z.classList.add(A ? "is-copied" : "is-failed"), Z.disabled = !0, setTimeout(function() {
        if (Z.classList.remove("is-copied", "is-failed"), Z.disabled = !1, S)
          S.textContent = B;
      }, 1200);
    }
    function F1(Z, E) {
      if (!navigator.clipboard) {
        K1(E, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(Z).then(function() {
        K1(E, "Copied!", !0);
      }, function() {
        K1(E, "Copy failed", !1);
      });
    }
    var W1 = h.getElementById("pages"), T = h.querySelector(".pages-trigger"), J = null, Q1 = window.matchMedia("(max-width: 78rem)"), j = function() {};
    function X1() {
      return J ? J.matches(":popover-open") : !1;
    }
    function a(Z) {
      if (!J || !X1())
        return;
      if (J.hidePopover(), Z && T && Q1.matches)
        setTimeout(function() {
          T.focus();
        }, 0);
    }
    if (W1 && T) {
      var L1 = W1.querySelector(".pages-list");
      if (L1) {
        var Y1 = h.createElement("div");
        if ("popover" in Y1 && typeof Y1.showPopover === "function") {
          let Z = function() {
            if (!T || !J)
              return;
            var E = T.getBoundingClientRect(), A = T.closest(".toolbar"), S = A ? A.getBoundingClientRect().bottom : E.bottom;
            J.style.setProperty("--pages-left", Math.max(16, E.left) + "px"), J.style.setProperty("--pages-top", S + "px"), J.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, E.left) - 16) + "px");
          };
          J = Y1, J.className = "pages-popover", J.id = "pages-popover", J.setAttribute("popover", "auto");
          var r = h.createElement("nav");
          r.className = "pages-popover-nav", r.setAttribute("aria-label", "Pages"), r.appendChild(L1.cloneNode(!0)), J.appendChild(r), T.setAttribute("popovertarget", J.id), T.popoverTargetElement = J, J.addEventListener("beforetoggle", function(E) {
            if (E.newState !== "open")
              return;
            j(), Z();
          }), J.addEventListener("toggle", function(E) {
            var A = E.newState === "open";
            if (T.setAttribute("aria-expanded", A ? "true" : "false"), h.body.classList.toggle("pages-popover-open", A), A) {
              var S = J.querySelector('[aria-current="page"]');
              if (S)
                S.scrollIntoView({ block: "nearest" });
            }
            c();
          }), r.querySelectorAll("a").forEach(function(E) {
            E.addEventListener("click", function() {
              a(!1);
            });
          }), Q1.addEventListener("change", function() {
            if (!Q1.matches)
              a(!1);
          }), window.addEventListener("resize", function() {
            if (X1())
              Z();
          }), T.hidden = !1, T.setAttribute("aria-expanded", "false"), h.body.appendChild(J), h.body.classList.add("pages-popover-ready");
        }
      }
    }
    var p = h.getElementById("source"), B1 = h.getElementById("changes"), S1 = h.querySelector("[data-airplan-all-changes]"), b = h.getElementById("toc"), I = null, X = null, N1 = window.matchMedia("(max-width: 78rem)");
    j = function() {
      if (X && X.open)
        X.close();
    };
    function c() {
      if (!b || !I || !X)
        return;
      var Z = N1.matches && !z.hidden && !X.open && !X1();
      if (I.classList.toggle("is-visible", Z), I.tabIndex = Z ? 0 : -1, I.setAttribute("aria-hidden", Z ? "false" : "true"), X.open && (!N1.matches || z.hidden))
        j();
    }
    function U1(Z) {
      if (a(!1), j(), z.hidden = Z !== "rendered", p)
        p.hidden = Z !== "source";
      if (B1)
        B1.hidden = Z !== "changes";
      if (b)
        b.hidden = Z !== "rendered";
      h.querySelectorAll(".viewtoggle button").forEach(function(E) {
        var A = E.dataset.view === Z;
        E.classList.toggle("active", A), E.setAttribute("aria-pressed", A ? "true" : "false");
      }), c();
    }
    h.querySelectorAll(".viewtoggle button").forEach(function(Z) {
      Z.addEventListener("click", function() {
        U1(Z.dataset.view || "rendered");
      });
    });
    var $1 = !1;
    h.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(Z) {
      Z.addEventListener("click", function() {
        $1 = new URL(Z.href).pathname === window.location.pathname;
      });
    });
    function f1() {
      var Z = window.location.hash === "#airplan-all-changes" && !!S1;
      if (a(!1), j(), h.body.classList.toggle("all-changes-active", Z), S1)
        S1.hidden = !Z;
      if (Z) {
        if (z.hidden = !0, p)
          p.hidden = !0;
        if (B1)
          B1.hidden = !0;
        if (b)
          b.hidden = !0;
        if ($1)
          S1.querySelector("h1")?.focus();
      } else
        U1("rendered");
      $1 = !1, c();
    }
    if (window.addEventListener("hashchange", f1), f1(), b) {
      let Z = function() {
        if (e.length === 0) {
          c();
          return;
        }
        var A = 0;
        if (r1.forEach(function(B, H) {
          if (B && B.getBoundingClientRect().top <= 128)
            A = H;
        }), window.innerHeight + window.scrollY >= h.documentElement.scrollHeight - 2)
          A = e.length - 1;
        var S = e[A].getAttribute("href");
        z1.forEach(function(B) {
          var H = B.getAttribute("href") === S;
          if (B.classList.toggle("active", H), H)
            B.setAttribute("aria-current", "location");
          else
            B.removeAttribute("aria-current");
        }), c();
      }, E = function() {
        if (J1)
          return;
        J1 = !0, window.requestAnimationFrame(function() {
          J1 = !1, Z();
        });
      };
      var e = Array.from(b.querySelectorAll('a[href^="#"]')), R1 = b.querySelector(".toc-list");
      if (R1)
        if (X = h.createElement("dialog"), typeof X.showModal === "function") {
          X.className = "toc-dialog", X.id = "toc-dialog", X.setAttribute("aria-labelledby", "toc-dialog-title");
          var _1 = h.createElement("div");
          _1.className = "toc-dialog-panel";
          var M1 = h.createElement("div");
          M1.className = "toc-dialog-header";
          var H1 = h.createElement("h2");
          H1.className = "toc-dialog-title", H1.id = "toc-dialog-title", H1.textContent = "Contents";
          var d = h.createElement("button");
          d.className = "toc-dialog-close", d.type = "button", d.setAttribute("aria-label", "Close table of contents"), d.innerHTML = m1, M1.appendChild(H1), M1.appendChild(d);
          var Z1 = h.createElement("nav");
          Z1.className = "toc-dialog-nav", Z1.setAttribute("aria-label", "Table of contents"), Z1.appendChild(R1.cloneNode(!0)), _1.appendChild(M1), _1.appendChild(Z1), X.appendChild(_1), I = h.createElement("button"), I.className = "toc-trigger", I.type = "button", I.tabIndex = -1, I.setAttribute("aria-label", "Open table of contents"), I.setAttribute("aria-controls", "toc-dialog"), I.setAttribute("aria-haspopup", "dialog"), I.setAttribute("aria-hidden", "true"), I.innerHTML = k1, h.body.appendChild(I), h.body.appendChild(X), h.body.classList.add("toc-dialog-ready"), I.addEventListener("click", function() {
            a(!1), X.showModal(), h.body.classList.add("toc-dialog-open"), c();
            var A = X.querySelector("a.active");
            if (A)
              A.scrollIntoView({ block: "nearest" });
          }), d.addEventListener("click", j), X.addEventListener("click", function(A) {
            if (A.target === X)
              j();
          }), X.addEventListener("keydown", function(A) {
            if (A.key === "Escape")
              A.preventDefault(), j();
          }), X.addEventListener("close", function() {
            if (h.body.classList.remove("toc-dialog-open"), c(), I.classList.contains("is-visible"))
              setTimeout(function() {
                I.focus();
              }, 50);
          }), Z1.querySelectorAll("a").forEach(function(A) {
            A.addEventListener("click", j);
          });
        } else
          X = null;
      var z1 = e.slice();
      if (X)
        z1 = z1.concat(Array.from(X.querySelectorAll('a[href^="#"]')));
      var r1 = e.map(function(A) {
        return h.getElementById((A.getAttribute("href") || "").slice(1));
      }), J1 = !1;
      h.addEventListener("scroll", E, { passive: !0 }), window.addEventListener("resize", Z), Z();
    }
    var q1 = h.querySelector(".toolbar");
    function w1() {
      var Z = q1 && window.matchMedia("(max-width: 78rem)").matches ? q1.getBoundingClientRect().height : 0;
      h.documentElement.style.setProperty("--airplan-sticky-height", Z + "px");
    }
    if (q1) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(w1).observe(q1);
      window.addEventListener("resize", w1), w1();
    }
    let I1 = h.querySelector(".copy-source");
    if (I1 && p)
      I1.addEventListener("click", function() {
        var Z = p.querySelector("pre");
        F1(Z ? Z.textContent : "", I1);
      });
    z.querySelectorAll("pre").forEach(function(Z) {
      if (Z.classList.contains("mermaid"))
        return;
      var E = h.createElement("div");
      E.className = "codewrap", Z.parentNode?.insertBefore(E, Z), E.appendChild(Z);
      var A = h.createElement("button");
      A.className = "codecopy", A.type = "button", A.setAttribute("aria-label", "Copy code"), A.title = "Copy code", A.innerHTML = v1 + P1 + b1, A.addEventListener("click", function() {
        var S = Z.querySelector("code");
        F1((S || Z).textContent, A);
      }), E.appendChild(A);
    });
  })();
})();
