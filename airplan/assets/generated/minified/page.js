(() => {
  function E0(S) {
    return S === "system" || S === "light" || S === "dark";
  }
  function K1(S, K) {
    try {
      return S?.getItem(K) ?? null;
    } catch {
      return null;
    }
  }
  function s(S, K, z) {
    try {
      if (z === null)
        S?.removeItem(K);
      else
        S?.setItem(K, z);
    } catch {}
  }
  function D1(S, K, z) {
    let J = K1(z, "airplan-color-mode");
    if (J === null) {
      let L = K1(z, "airplan-theme");
      if (J = L === "light" || L === "dark" ? L : "system", J !== "system")
        s(z, "airplan-color-mode", J);
    }
    let U = E0(J) ? J : "system", x = new Set(S.themes.map((L) => L.id)), G = K1(z, "airplan-light-theme"), V = K1(z, "airplan-dark-theme"), $ = G !== null && x.has(G) ? G : S.defaultLight, W = V !== null && x.has(V) ? V : S.defaultDark;
    return F1(S, U, $, W, K);
  }
  function F1(S, K, z, J, U) {
    let x = new Map(S.themes.map((j) => [j.id, j])), G = x.has(z) ? z : S.defaultLight, V = x.has(J) ? J : S.defaultDark, $ = K === "system" ? U ? "dark" : "light" : K, W = $ === "light" ? G : V, L = x.get(W)?.variant ?? $;
    return { mode: K, resolvedMode: $, lightTheme: G, darkTheme: V, theme: W, variant: L };
  }
  function j1(S, K) {
    if (K === "system")
      s(S, "airplan-color-mode", null), s(S, "airplan-theme", null);
    else
      s(S, "airplan-color-mode", K), s(S, "airplan-theme", K);
  }
  function u1(S, K, z) {
    s(S, K === "light" ? "airplan-light-theme" : "airplan-dark-theme", z);
  }
  function y1(S) {
    return {
      mode: S.mode,
      resolvedMode: S.resolvedMode,
      theme: S.theme,
      variant: S.variant
    };
  }

  (function() {
    let S = document, K = S.documentElement;
    S.querySelectorAll(".js-only").forEach((H) => {
      H.hidden = !1;
    });
    let z = window.__AIRPLAN_THEME_CATALOG__;
    if (!z)
      return;
    let J = z, U = window.matchMedia("(prefers-color-scheme: dark)"), x;
    try {
      x = window.localStorage;
    } catch {}
    let G = window.__airplanThemeState ?? D1(J, U.matches, x), V = S.querySelector("[data-airplan-appearance-trigger]"), $ = S.querySelector("[data-airplan-appearance-panel]"), W = S.querySelector('select[data-airplan-theme-slot="light"]'), L = S.querySelector('select[data-airplan-theme-slot="dark"]'), j = Array.from(S.querySelectorAll("[data-airplan-color-mode]"));
    function o(H) {
      if (!H || H.options.length > 0)
        return;
      for (let [Q, y] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let R = S.createElement("optgroup");
        R.label = y;
        for (let m of J.themes) {
          if (m.variant !== Q)
            continue;
          let n = S.createElement("option");
          n.value = m.id, n.textContent = m.name, R.append(n);
        }
        if (R.children.length > 0)
          H.append(R);
      }
    }
    o(W), o(L);
    function u(H, Q = !0) {
      if (G = H, window.__airplanThemeState = G, K.dataset.airplanMode = G.mode, K.dataset.airplanResolvedMode = G.resolvedMode, K.dataset.airplanTheme = G.theme, K.dataset.airplanThemeVariant = G.variant, j.forEach((y) => {
        let R = y.dataset.airplanColorMode === G.mode;
        y.classList.toggle("active", R), y.setAttribute("aria-pressed", String(R));
      }), W)
        W.value = G.lightTheme;
      if (L)
        L.value = G.darkTheme;
      if (Q)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: y1(G) }));
    }
    function k(H = {}) {
      u(F1(J, H.mode ?? G.mode, H.lightTheme ?? G.lightTheme, H.darkTheme ?? G.darkTheme, U.matches));
    }
    function t(H, Q = !1) {
      if (!$ || !V)
        return;
      if ($.hidden = !H, V.setAttribute("aria-expanded", String(H)), H)
        $.querySelector("button,select")?.focus();
      else if (Q)
        V.focus();
    }
    V?.addEventListener("click", () => t(Boolean($?.hidden ?? !0))), j.forEach((H) => H.addEventListener("click", () => {
      let Q = H.dataset.airplanColorMode;
      if (!Q)
        return;
      j1(x, Q), k({ mode: Q });
    }));
    function A1(H, Q) {
      u1(x, H, Q.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: Q.value } })), k(H === "light" ? { lightTheme: Q.value } : { darkTheme: Q.value });
    }
    W?.addEventListener("change", () => A1("light", W)), L?.addEventListener("change", () => A1("dark", L)), U.addEventListener("change", () => {
      if (G.mode === "system")
        k();
    }), S.addEventListener("keydown", (H) => {
      if (H.key === "Escape" && $ && !$.hidden)
        H.preventDefault(), t(!1, !0);
    }), S.addEventListener("pointerdown", (H) => {
      if (!$ || $.hidden || !V)
        return;
      let Q = H.target;
      if (!(Q instanceof Node) || $.contains(Q) || V.contains(Q))
        return;
      let R = (Q instanceof Element ? Q : Q.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), m = $.contains(S.activeElement) && !R;
      if (t(!1), m)
        setTimeout(() => {
          if (S.activeElement === S.body || $.contains(S.activeElement))
            V.focus();
        });
    }), u(G, !1);
  })();

  var m1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', P1 = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', b1 = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>';
  var v1 = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';
  var k1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>';

  (function() {
    var S = document, K = 262144;
    let z = S.getElementById("rendered");
    if (!z)
      return;
    let J = z;
    var U = S.querySelector('meta[name="airplan-versions"]'), x = S.querySelector('meta[name="airplan-revision-chain"]'), G = S.querySelector('meta[name="airplan-page-path"]'), V = S.querySelector('meta[name="airplan-entrypoint"]'), $ = U ? new URL(U.content, window.location.href) : null, W = $ ? new URL("./", $) : null, L = W ? W.pathname.split("/").filter(Boolean) : [], j = L.slice(0, -1);
    function o(Z, E) {
      if (typeof Z !== "string")
        return null;
      try {
        var A = new URL(Z);
        if (A.origin !== window.location.origin || A.username || A.password || A.search || A.hash)
          return null;
        var _ = A.pathname.split("/").filter(Boolean);
        if (_.length !== j.length + 2 || !j.every(function(C, X) {
          return _[X] === C;
        }) || !/^[a-z2-7]{26}$/.test(_[_.length - 2]))
          return null;
        var B = _[_.length - 1];
        if (E ? B !== ".airplan-changes.diff" : !B.endsWith(".html"))
          return null;
        return A.href;
      } catch {
        return null;
      }
    }
    function u(Z) {
      if (typeof Z !== "string" || Z === "" || Z.startsWith("/") || Z.includes("\\"))
        return !1;
      var E = Z.split("/");
      return E.every(function(A) {
        var _ = A.toLowerCase(), B = Array.from(A).some(function(C) {
          var X = C.codePointAt(0) || 0;
          return X < 32 || X === 127;
        });
        if (!A || A === "." || A === ".." || _.startsWith(".airplan-") || _ === ".airplan.json" || B || /[. ]$/.test(A) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(A))
          return !1;
        return !0;
      });
    }
    function k(Z, E) {
      if (!u(E))
        return null;
      var A = String(E).split("/").map(function(B) {
        return encodeURIComponent(B);
      }).join("/"), _ = new URL(A, Z);
      if (_.origin !== Z.origin || _.username || _.password || _.search || _.hash || !_.pathname.startsWith(Z.pathname))
        return null;
      return _.href;
    }
    async function t(Z) {
      if (!Z.ok)
        throw Error("marker request failed");
      var E = Z.headers.get("content-length");
      if (E && /^\d+$/.test(E) && Number(E) > K) {
        if (Z.body)
          await Z.body.cancel("marker is too large");
        throw Error("marker is too large");
      }
      if (!Z.body || typeof Z.body.getReader !== "function")
        throw Error("bounded marker stream is unavailable");
      var A = Z.body.getReader(), _ = [], B = 0;
      try {
        for (;; ) {
          var C = await A.read();
          if (C.done)
            break;
          if (B += C.value.byteLength, B > K)
            throw await A.cancel("marker is too large"), Error("marker is too large");
          _.push(C.value);
        }
      } finally {
        A.releaseLock();
      }
      var X = new Uint8Array(B), h = 0;
      _.forEach(function(O) {
        X.set(O, h), h += O.byteLength;
      });
      var w = new TextDecoder("utf-8", { fatal: !0 }).decode(X);
      return A1(w), JSON.parse(w);
    }
    function A1(Z) {
      var E = 0;
      function A() {
        while (/\s/.test(Z[E] || ""))
          E += 1;
      }
      function _() {
        if (Z[E] !== '"')
          throw Error("JSON string is invalid");
        var C = E++;
        while (E < Z.length) {
          var X = Z[E++];
          if (X === '"')
            return JSON.parse(Z.slice(C, E));
          if (X === "\\")
            E += 1;
        }
        throw Error("JSON string is incomplete");
      }
      function B() {
        if (A(), Z[E] === "{") {
          E += 1, A();
          var C = new Set;
          if (Z[E] === "}") {
            E += 1;
            return;
          }
          for (;; ) {
            A();
            var X = _();
            if (C.has(X))
              throw Error("JSON object has a duplicate field");
            if (C.add(X), A(), Z[E++] !== ":")
              throw Error("JSON object is invalid");
            B(), A();
            var h = Z[E++];
            if (h === "}")
              return;
            if (h !== ",")
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
            var h = Z[E++];
            if (h === "]")
              return;
            if (h !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (Z[E] === '"') {
          _();
          return;
        }
        var w = Z.slice(E).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!w)
          throw Error("JSON value is invalid");
        E += w[0].length;
      }
      if (B(), A(), E !== Z.length)
        throw Error("JSON has trailing content");
    }
    function H(Z, E, A, _) {
      if (!P(Z))
        throw Error("marker is invalid");
      var B = Z, C = E.pathname.split("/").filter(Boolean), X = C[C.length - 1] || "";
      if (B.schema !== "airplan-upload" || B.version !== 6 || B.kind !== "document" || B.directory !== X || !/^[a-z2-7]{26}$/.test(B.directory) || !l1(B.created_at) || B.format !== "md" || !n1(B.slug) || B.entrypoint !== B.slug + ".html" || !P(B.producer) || B.producer.name !== "airplan" || typeof B.producer.version !== "string" || B.producer.version.trim() !== B.producer.version || B.producer.version === "" || !y(B.render) || !P(B.revision) || B.revision.number !== A.number || B.revision.chain_id !== _ || (B.revision.number === 1 ? B.revision.previous_url !== void 0 : typeof B.revision.previous_url !== "string" || !g1(B.revision.previous_url)) || !Array.isArray(B.objects) || !Array.isArray(B.pages) || B.pages.length === 0)
        throw Error("marker identity is invalid");
      var h = k(E, B.entrypoint);
      if (h !== A.safeURL)
        throw Error("marker entrypoint is invalid");
      if (B.title !== void 0 && typeof B.title !== "string" || B.repo !== void 0 && !c1(B.repo) || B.objects.length === 0 || B.pages.length > 100)
        throw Error("marker shape is invalid");
      var w = d1(B), O = new Set, d = new Set, c = new Set, E1 = new Map;
      if (B.pages.forEach(function(M, v) {
        if (!P(M) || !u(M.path) || O.has(M.path) || d.has(M.path.toLowerCase()) || M.format !== "md" && M.format !== "txt" || typeof M.lang !== "string" || M.title !== void 0 && typeof M.title !== "string" || !u(M.page) || !u(M.source))
          throw Error("marker page descriptor is invalid");
        var q = Q(M.path, M.format), N = M.path;
        if (v === 0) {
          if (q = B.entrypoint, N = B.slug + ".md", M.format !== B.format)
            throw Error("marker entry format is invalid");
        }
        if (M.page !== q || M.source !== N)
          throw Error("marker generated page mapping is invalid");
        var T = k(E, M.page);
        if (!T || c.has(T))
          throw Error("marker page object is invalid");
        if (!k(E, M.source))
          throw Error("marker source object is invalid");
        if (w.get(M.page) !== "page" || w.get(M.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var H1 = B.objects.find(function(G1) {
          return G1.name === M.source;
        }).content_type;
        if (M.format === "md" && H1 !== "text/markdown; charset=utf-8" || M.format === "txt" && H1 !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        O.add(M.path), d.add(M.path.toLowerCase()), c.add(T), E1.set(M.path, T);
      }), m(d))
        throw Error("marker page paths conflict");
      if (!O.has(B.pages[0].path) || E1.get(B.pages[0].path) !== h)
        throw Error("marker entry page is invalid");
      if (c.size !== B.pages.length || Array.from(w.values()).filter(function(M) {
        return M === "source";
      }).length !== B.pages.length)
        throw Error("marker page inventory is invalid");
      return E1;
    }
    function Q(Z, E) {
      if (E !== "md")
        return Z + ".html";
      var A = Z.lastIndexOf("/"), _ = Z.lastIndexOf(".");
      return (_ > A ? Z.slice(0, _) : Z) + ".html";
    }
    function y(Z) {
      if (!P(Z) || !P(Z.template) || !P(Z.themes) || !Number.isInteger(Z.generation) || Z.generation <= 0 || typeof Z.indexable !== "boolean" || typeof Z.no_external_assets !== "boolean" || !Z.template || Z.template.kind !== "builtin" && Z.template.kind !== "custom" || Z.mermaid_url !== void 0 && !p1(Z.mermaid_url) || !Z.themes)
        return !1;
      if (Z.template.kind === "builtin" && Z.template.sha256 !== void 0 || Z.template.kind === "custom" && !n(Z.template.sha256))
        return !1;
      return R(Z.themes.default_light) && R(Z.themes.default_dark) && n(Z.themes.catalog_sha256);
    }
    function R(Z) {
      return typeof Z === "string" && new TextEncoder().encode(Z).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(Z);
    }
    function m(Z) {
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
    function n(Z) {
      return typeof Z === "string" && /^[0-9a-f]{64}$/.test(Z);
    }
    function P(Z) {
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
      var A = Number(E[1]), _ = Number(E[2]), B = Number(E[3]), C = Number(E[4]), X = Number(E[5]), h = Number(E[6]), w = A % 4 === 0 && (A % 100 !== 0 || A % 400 === 0), O = [31, w ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return _ >= 1 && _ <= 12 && B >= 1 && B <= O[_ - 1] && C <= 23 && X <= 59 && h <= 59;
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
        var _ = A[0], B = A[1].replace(/\.git$/, "");
        if (!_ || !B || _ === "." || _ === ".." || B === "." || B === ".." || /[?#@:\\]/.test(_ + B))
          return !1;
        return Z === "https://" + E.hostname.toLowerCase() + "/" + _ + "/" + B;
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
      var E = new Map, A = new Set, _ = 0, B = 0, C = 0, X = 0;
      if (Z.objects.forEach(function(h) {
        if (!P(h) || !u(h.name) && h.name !== ".airplan-changes.diff" || E.has(h.name) || A.has(h.name.toLowerCase()) || !Number.isSafeInteger(h.bytes) || h.bytes < 0 || !n(h.sha256) || !i1(h.content_type))
          throw Error("marker object inventory is invalid");
        if (h.role === "page") {
          if (B += 1, h.bytes <= 0 || h.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (h.role === "source") {
          if (C += 1, h.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (h.role === "asset")
          X += 1;
        else if (h.role === "diff") {
          if (_ += 1, h.name !== ".airplan-changes.diff" || h.bytes <= 0 || h.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        E.set(h.name, h.role), A.add(h.name.toLowerCase());
      }), m(A))
        throw Error("marker object paths conflict");
      if (B !== Z.pages.length || C !== Z.pages.length || B + X > 100 || (Z.revision.number === 1 ? _ !== 0 : _ !== 1))
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
      var E = S.querySelector('meta[name="airplan-revision"]'), A = E ? Number(E.content) : Number(Z.current_revision);
      if (!Number.isInteger(A) || A <= 0 || Z.current_revision !== A || !Number.isInteger(Z.latest_revision) || !Number.isInteger(Z.last_assigned_revision) || !Array.isArray(Z.revisions) || Z.revisions.length === 0 || Z.last_assigned_revision !== Z.revisions.length || !/^[a-z2-7]{26}$/.test(Z.chain_id) || x && x.content !== Z.chain_id)
        throw Error("revision identity is invalid");
      var _ = !1, B = 0, C = Z.revisions.filter(function(q) {
        if (!q || !Number.isInteger(q.number) || q.number !== B + 1)
          return _ = !0, !1;
        if (B = q.number, q.deleted)
          return !1;
        if (q.safeURL = o(q.url, !1), !q.safeURL)
          return _ = !0, !1;
        if (q.number > 1) {
          var N = o(q.diff_url, !0);
          if (!N || new URL(N).pathname.replace(/[^/]+$/, "") !== new URL(q.safeURL).pathname.replace(/[^/]+$/, ""))
            return _ = !0, !1;
        }
        return !0;
      });
      if (_ || Z.revisions[0].number !== 1 || !C.some(function(q) {
        return q.number === A;
      }))
        throw Error("revision entries are invalid");
      var X = C.find(function(q) {
        return q.number === A;
      }), h = new URL(window.location.href);
      if (h.search = "", h.hash = "", !X || !W || new URL(X.safeURL || "").pathname.replace(/[^/]+$/, "") !== W.pathname || !h.pathname.startsWith(W.pathname))
        throw Error("current revision URL is invalid");
      var w = Math.max.apply(null, C.map(function(q) {
        return q.number;
      }));
      if (w !== Z.latest_revision)
        throw Error("latest is invalid");
      var O = S.querySelector("[data-revision-heading]");
      if (!O) {
        O = S.createElement("p"), O.className = "revision-heading", O.setAttribute("data-revision-heading", "");
        var d = S.getElementById("rendered");
        if (!d)
          throw Error("rendered view is unavailable");
        d.prepend(O);
      }
      var c = A < w, E1 = c ? "Revision " + A + " of " + w : "Revision " + A + " (Latest)", M = S.createElement("span");
      M.className = "revision-picker-label", M.textContent = E1, M.setAttribute("aria-hidden", "true");
      var v = S.createElement("select");
      v.setAttribute("aria-label", "Document revision"), C.forEach(function(q) {
        var N = S.createElement("option");
        N.value = q.safeURL || "", N.textContent = q.number === w ? "Revision " + q.number + " (Latest)" : "Revision " + q.number + " of " + w, N.selected = q.number === A, v.appendChild(N);
      }), v.addEventListener("change", function() {
        var q = v.selectedIndex;
        if (q < 0 || q >= C.length)
          return;
        var N = C[q], T = N.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(T + (N.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var H1 = V ? new URL(V.content, window.location.href).href : "";
        if (!G || h.href === H1 || !x) {
          window.location.assign(T);
          return;
        }
        O.setAttribute("aria-busy", "true"), v.disabled = !0;
        var G1 = new URL("./", T), T1 = new URL(".airplan.json", G1);
        T1.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(T1, { cache: "no-store", credentials: "same-origin" }).then(t).then(function(e1) {
          var Z0 = H(e1, G1, N, x.content);
          window.location.assign(s1(T, Z0.get(G.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(T);
        });
      }), O.replaceChildren(M, v), O.classList.add("is-picker"), O.classList.toggle("is-stale", c), S.body.classList.toggle("airplan-stale-revision", c);
    }
    if (U) {
      var w1 = new URL(U.content, window.location.href);
      w1.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(w1, { cache: "no-store", credentials: "same-origin" }).then(function(Z) {
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
    var S1 = S.createElement("div");
    S1.className = "sr-status", S1.setAttribute("aria-live", "polite"), S.body.appendChild(S1);
    var i = null;
    function t1() {
      if (i !== null)
        return;
      i = Array.from(S.querySelectorAll("details:not([open])")), i.forEach(function(Z) {
        Z.open = !0;
      });
    }
    function a1() {
      if (i === null)
        return;
      i.forEach(function(Z) {
        Z.open = !1;
      }), i = null;
    }
    window.addEventListener("beforeprint", t1), window.addEventListener("afterprint", a1);
    function Q1(Z, E, A) {
      S1.textContent = E;
      var _ = Z.querySelector(".action-label"), B = _ ? _.textContent : "";
      if (_)
        _.textContent = A ? "Copied" : "Failed";
      Z.classList.add(A ? "is-copied" : "is-failed"), Z.disabled = !0, setTimeout(function() {
        if (Z.classList.remove("is-copied", "is-failed"), Z.disabled = !1, _)
          _.textContent = B;
      }, 1200);
    }
    function x1(Z, E) {
      if (!navigator.clipboard) {
        Q1(E, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(Z).then(function() {
        Q1(E, "Copied!", !0);
      }, function() {
        Q1(E, "Copy failed", !1);
      });
    }
    var W1 = S.getElementById("pages"), f = S.querySelector(".pages-trigger"), I = null, X1 = window.matchMedia("(max-width: 78rem)"), D = function() {};
    function Y1() {
      return I ? I.matches(":popover-open") : !1;
    }
    function a(Z) {
      if (!I || !Y1())
        return;
      if (I.hidePopover(), Z && f && X1.matches)
        setTimeout(function() {
          f.focus();
        }, 0);
    }
    if (W1 && f) {
      var L1 = W1.querySelector(".pages-list");
      if (L1) {
        var $1 = S.createElement("div");
        if ("popover" in $1 && typeof $1.showPopover === "function") {
          let Z = function() {
            if (!f || !I)
              return;
            var E = f.getBoundingClientRect();
            I.style.setProperty("--pages-left", Math.max(16, E.left) + "px"), I.style.setProperty("--pages-top", E.bottom + "px"), I.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, E.left) - 16) + "px");
          };
          I = $1, I.className = "pages-popover", I.id = "pages-popover", I.setAttribute("popover", "auto");
          var r = S.createElement("nav");
          r.className = "pages-popover-nav", r.setAttribute("aria-label", "Pages"), r.appendChild(L1.cloneNode(!0)), I.appendChild(r), f.setAttribute("popovertarget", I.id), f.popoverTargetElement = I, I.addEventListener("beforetoggle", function(E) {
            if (E.newState !== "open")
              return;
            D(), Z();
          }), I.addEventListener("toggle", function(E) {
            var A = E.newState === "open";
            if (f.setAttribute("aria-expanded", A ? "true" : "false"), S.body.classList.toggle("pages-popover-open", A), A) {
              var _ = I.querySelector('[aria-current="page"]');
              if (_)
                _.scrollIntoView({ block: "nearest" });
            }
            l();
          }), r.querySelectorAll("a").forEach(function(E) {
            E.addEventListener("click", function() {
              a(!1);
            });
          }), X1.addEventListener("change", function() {
            if (!X1.matches)
              a(!1);
          }), window.addEventListener("resize", function() {
            if (Y1())
              Z();
          }), f.hidden = !1, f.setAttribute("aria-expanded", "false"), S.body.appendChild(I), S.body.classList.add("pages-popover-ready");
        }
      }
    }
    var g = S.getElementById("source"), B1 = S.getElementById("changes"), _1 = S.querySelector("[data-airplan-all-changes]"), b = S.getElementById("toc"), F = null, Y = null, N1 = window.matchMedia("(max-width: 78rem)");
    D = function() {
      if (Y && Y.open)
        Y.close();
    };
    function l() {
      if (!b || !F || !Y)
        return;
      var Z = N1.matches && !J.hidden && !Y.open && !Y1();
      if (F.classList.toggle("is-visible", Z), F.tabIndex = Z ? 0 : -1, F.setAttribute("aria-hidden", Z ? "false" : "true"), Y.open && (!N1.matches || J.hidden))
        D();
    }
    function U1(Z) {
      if (a(!1), D(), J.hidden = Z !== "rendered", g)
        g.hidden = Z !== "source";
      if (B1)
        B1.hidden = Z !== "changes";
      if (b)
        b.hidden = Z !== "rendered";
      S.querySelectorAll(".viewtoggle button").forEach(function(E) {
        var A = E.dataset.view === Z;
        E.classList.toggle("active", A), E.setAttribute("aria-pressed", A ? "true" : "false");
      }), l();
    }
    S.querySelectorAll(".viewtoggle button").forEach(function(Z) {
      Z.addEventListener("click", function() {
        U1(Z.dataset.view || "rendered");
      });
    });
    var z1 = !1;
    S.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(Z) {
      Z.addEventListener("click", function() {
        z1 = new URL(Z.href).pathname === window.location.pathname;
      });
    });
    function R1() {
      var Z = window.location.hash === "#airplan-all-changes" && !!_1;
      if (a(!1), D(), S.body.classList.toggle("all-changes-active", Z), _1)
        _1.hidden = !Z;
      if (Z) {
        if (J.hidden = !0, g)
          g.hidden = !0;
        if (B1)
          B1.hidden = !0;
        if (b)
          b.hidden = !0;
        if (z1)
          _1.querySelector("h1")?.focus();
      } else
        U1("rendered");
      z1 = !1, l();
    }
    if (window.addEventListener("hashchange", R1), R1(), b) {
      let Z = function() {
        if (e.length === 0) {
          l();
          return;
        }
        var A = 0;
        if (r1.forEach(function(B, C) {
          if (B && B.getBoundingClientRect().top <= 128)
            A = C;
        }), window.innerHeight + window.scrollY >= S.documentElement.scrollHeight - 2)
          A = e.length - 1;
        var _ = e[A].getAttribute("href");
        J1.forEach(function(B) {
          var C = B.getAttribute("href") === _;
          if (B.classList.toggle("active", C), C)
            B.setAttribute("aria-current", "location");
          else
            B.removeAttribute("aria-current");
        }), l();
      }, E = function() {
        if (I1)
          return;
        I1 = !0, window.requestAnimationFrame(function() {
          I1 = !1, Z();
        });
      };
      var e = Array.from(b.querySelectorAll('a[href^="#"]')), f1 = b.querySelector(".toc-list");
      if (f1)
        if (Y = S.createElement("dialog"), typeof Y.showModal === "function") {
          Y.className = "toc-dialog", Y.id = "toc-dialog", Y.setAttribute("aria-labelledby", "toc-dialog-title");
          var h1 = S.createElement("div");
          h1.className = "toc-dialog-panel";
          var M1 = S.createElement("div");
          M1.className = "toc-dialog-header";
          var C1 = S.createElement("h2");
          C1.className = "toc-dialog-title", C1.id = "toc-dialog-title", C1.textContent = "Contents";
          var p = S.createElement("button");
          p.className = "toc-dialog-close", p.type = "button", p.setAttribute("aria-label", "Close table of contents"), p.innerHTML = m1, M1.appendChild(C1), M1.appendChild(p);
          var Z1 = S.createElement("nav");
          Z1.className = "toc-dialog-nav", Z1.setAttribute("aria-label", "Table of contents"), Z1.appendChild(f1.cloneNode(!0)), h1.appendChild(M1), h1.appendChild(Z1), Y.appendChild(h1), F = S.createElement("button"), F.className = "toc-trigger", F.type = "button", F.tabIndex = -1, F.setAttribute("aria-label", "Open table of contents"), F.setAttribute("aria-controls", "toc-dialog"), F.setAttribute("aria-haspopup", "dialog"), F.setAttribute("aria-hidden", "true"), F.innerHTML = k1, S.body.appendChild(F), S.body.appendChild(Y), S.body.classList.add("toc-dialog-ready"), F.addEventListener("click", function() {
            a(!1), Y.showModal(), S.body.classList.add("toc-dialog-open"), l();
            var A = Y.querySelector("a.active");
            if (A)
              A.scrollIntoView({ block: "nearest" });
          }), p.addEventListener("click", D), Y.addEventListener("click", function(A) {
            if (A.target === Y)
              D();
          }), Y.addEventListener("keydown", function(A) {
            if (A.key === "Escape")
              A.preventDefault(), D();
          }), Y.addEventListener("close", function() {
            if (S.body.classList.remove("toc-dialog-open"), l(), F.classList.contains("is-visible"))
              setTimeout(function() {
                F.focus();
              }, 50);
          }), Z1.querySelectorAll("a").forEach(function(A) {
            A.addEventListener("click", D);
          });
        } else
          Y = null;
      var J1 = e.slice();
      if (Y)
        J1 = J1.concat(Array.from(Y.querySelectorAll('a[href^="#"]')));
      var r1 = e.map(function(A) {
        return S.getElementById((A.getAttribute("href") || "").slice(1));
      }), I1 = !1;
      S.addEventListener("scroll", E, { passive: !0 }), window.addEventListener("resize", Z), Z();
    }
    var q1 = S.querySelector(".toolbar");
    function O1() {
      var Z = q1 && window.matchMedia("(max-width: 78rem)").matches ? q1.getBoundingClientRect().height : 0;
      S.documentElement.style.setProperty("--airplan-sticky-height", Z + "px");
    }
    if (q1) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(O1).observe(q1);
      window.addEventListener("resize", O1), O1();
    }
    let V1 = S.querySelector(".copy-source");
    if (V1 && g)
      V1.addEventListener("click", function() {
        var Z = g.querySelector("pre");
        x1(Z ? Z.textContent : "", V1);
      });
    J.querySelectorAll("pre").forEach(function(Z) {
      if (Z.classList.contains("mermaid"))
        return;
      var E = S.createElement("div");
      E.className = "codewrap", Z.parentNode?.insertBefore(E, Z), E.appendChild(Z);
      var A = S.createElement("button");
      A.className = "codecopy", A.type = "button", A.setAttribute("aria-label", "Copy code"), A.title = "Copy code", A.innerHTML = b1 + P1 + v1, A.addEventListener("click", function() {
        var _ = Z.querySelector("code");
        x1((_ || Z).textContent, A);
      }), E.appendChild(A);
    });
  })();
})();
