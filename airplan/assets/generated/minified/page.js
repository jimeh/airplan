(() => {
  function E0(h) {
    return h === "system" || h === "light" || h === "dark";
  }
  function K1(h, K) {
    try {
      return h?.getItem(K) ?? null;
    } catch {
      return null;
    }
  }
  function s(h, K, z) {
    try {
      if (z === null)
        h?.removeItem(K);
      else
        h?.setItem(K, z);
    } catch {}
  }
  function D1(h, K, z) {
    let J = K1(z, "airplan-color-mode");
    if (J === null) {
      let L = K1(z, "airplan-theme");
      if (J = L === "light" || L === "dark" ? L : "system", J !== "system")
        s(z, "airplan-color-mode", J);
    }
    let U = E0(J) ? J : "system", F = new Set(h.themes.map((L) => L.id)), G = K1(z, "airplan-light-theme"), V = K1(z, "airplan-dark-theme"), $ = G !== null && F.has(G) ? G : h.defaultLight, W = V !== null && F.has(V) ? V : h.defaultDark;
    return w1(h, U, $, W, K);
  }
  function w1(h, K, z, J, U) {
    let F = new Map(h.themes.map((j) => [j.id, j])), G = F.has(z) ? z : h.defaultLight, V = F.has(J) ? J : h.defaultDark, $ = K === "system" ? U ? "dark" : "light" : K, W = $ === "light" ? G : V, L = F.get(W)?.variant ?? $;
    return { mode: K, resolvedMode: $, lightTheme: G, darkTheme: V, theme: W, variant: L };
  }
  function j1(h, K) {
    if (K === "system")
      s(h, "airplan-color-mode", null), s(h, "airplan-theme", null);
    else
      s(h, "airplan-color-mode", K), s(h, "airplan-theme", K);
  }
  function u1(h, K, z) {
    s(h, K === "light" ? "airplan-light-theme" : "airplan-dark-theme", z);
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
    let h = document, K = h.documentElement;
    h.querySelectorAll(".js-only").forEach((q) => {
      q.hidden = !1;
    });
    let z = window.__AIRPLAN_THEME_CATALOG__;
    if (!z)
      return;
    let J = z, U = window.matchMedia("(prefers-color-scheme: dark)"), F;
    try {
      F = window.localStorage;
    } catch {}
    let G = window.__airplanThemeState ?? D1(J, U.matches, F), V = h.querySelector("[data-airplan-appearance-trigger]"), $ = h.querySelector("[data-airplan-appearance-panel]"), W = h.querySelector('select[data-airplan-theme-slot="light"]'), L = h.querySelector('select[data-airplan-theme-slot="dark"]'), j = Array.from(h.querySelectorAll("[data-airplan-color-mode]"));
    function o(q) {
      if (!q || q.options.length > 0)
        return;
      for (let [Q, y] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let R = h.createElement("optgroup");
        R.label = y;
        for (let m of J.themes) {
          if (m.variant !== Q)
            continue;
          let n = h.createElement("option");
          n.value = m.id, n.textContent = m.name, R.append(n);
        }
        if (R.children.length > 0)
          q.append(R);
      }
    }
    o(W), o(L);
    function u(q, Q = !0) {
      if (G = q, window.__airplanThemeState = G, K.dataset.airplanMode = G.mode, K.dataset.airplanResolvedMode = G.resolvedMode, K.dataset.airplanTheme = G.theme, K.dataset.airplanThemeVariant = G.variant, j.forEach((y) => {
        let R = y.dataset.airplanColorMode === G.mode;
        y.classList.toggle("active", R), y.setAttribute("aria-pressed", String(R));
      }), W)
        W.value = G.lightTheme;
      if (L)
        L.value = G.darkTheme;
      if (Q)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: y1(G) }));
    }
    function k(q = {}) {
      u(w1(J, q.mode ?? G.mode, q.lightTheme ?? G.lightTheme, q.darkTheme ?? G.darkTheme, U.matches));
    }
    function t(q, Q = !1) {
      if (!$ || !V)
        return;
      if ($.hidden = !q, V.setAttribute("aria-expanded", String(q)), q)
        $.querySelector("button,select")?.focus();
      else if (Q)
        V.focus();
    }
    V?.addEventListener("click", () => t(Boolean($?.hidden ?? !0))), j.forEach((q) => q.addEventListener("click", () => {
      let Q = q.dataset.airplanColorMode;
      if (!Q)
        return;
      j1(F, Q), k({ mode: Q });
    }));
    function A1(q, Q) {
      u1(F, q, Q.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: Q.value } })), k(q === "light" ? { lightTheme: Q.value } : { darkTheme: Q.value });
    }
    W?.addEventListener("change", () => A1("light", W)), L?.addEventListener("change", () => A1("dark", L)), U.addEventListener("change", () => {
      if (G.mode === "system")
        k();
    }), h.addEventListener("keydown", (q) => {
      if (q.key === "Escape" && $ && !$.hidden)
        q.preventDefault(), t(!1, !0);
    }), h.addEventListener("pointerdown", (q) => {
      if (!$ || $.hidden || !V)
        return;
      let Q = q.target;
      if (!(Q instanceof Node) || $.contains(Q) || V.contains(Q))
        return;
      let R = (Q instanceof Element ? Q : Q.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), m = $.contains(h.activeElement) && !R;
      if (t(!1), m)
        setTimeout(() => {
          if (h.activeElement === h.body || $.contains(h.activeElement))
            V.focus();
        });
    }), u(G, !1);
  })();

  var m1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', P1 = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', v1 = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>';
  var b1 = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';
  var k1 = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>';

  (function() {
    var h = document, K = 262144;
    let z = h.getElementById("rendered");
    if (!z)
      return;
    let J = z;
    var U = h.querySelector('meta[name="airplan-versions"]'), F = h.querySelector('meta[name="airplan-revision-chain"]'), G = h.querySelector('meta[name="airplan-page-path"]'), V = h.querySelector('meta[name="airplan-entrypoint"]'), $ = U ? new URL(U.content, window.location.href) : null, W = $ ? new URL("./", $) : null, L = W ? W.pathname.split("/").filter(Boolean) : [], j = L.slice(0, -1);
    function o(Z, E) {
      if (typeof Z !== "string")
        return null;
      try {
        var A = new URL(Z);
        if (A.origin !== window.location.origin || A.username || A.password || A.search || A.hash)
          return null;
        var B = A.pathname.split("/").filter(Boolean);
        if (B.length !== j.length + 2 || !j.every(function(C, X) {
          return B[X] === C;
        }) || !/^[a-z2-7]{26}$/.test(B[B.length - 2]))
          return null;
        var S = B[B.length - 1];
        if (E ? S !== ".airplan-changes.diff" : !S.endsWith(".html"))
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
        var B = A.toLowerCase(), S = Array.from(A).some(function(C) {
          var X = C.codePointAt(0) || 0;
          return X < 32 || X === 127;
        });
        if (!A || A === "." || A === ".." || B.startsWith(".airplan-") || B === ".airplan.json" || S || /[. ]$/.test(A) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(A))
          return !1;
        return !0;
      });
    }
    function k(Z, E) {
      if (!u(E))
        return null;
      var A = String(E).split("/").map(function(S) {
        return encodeURIComponent(S);
      }).join("/"), B = new URL(A, Z);
      if (B.origin !== Z.origin || B.username || B.password || B.search || B.hash || !B.pathname.startsWith(Z.pathname))
        return null;
      return B.href;
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
      var A = Z.body.getReader(), B = [], S = 0;
      try {
        for (;; ) {
          var C = await A.read();
          if (C.done)
            break;
          if (S += C.value.byteLength, S > K)
            throw await A.cancel("marker is too large"), Error("marker is too large");
          B.push(C.value);
        }
      } finally {
        A.releaseLock();
      }
      var X = new Uint8Array(S), _ = 0;
      B.forEach(function(O) {
        X.set(O, _), _ += O.byteLength;
      });
      var x = new TextDecoder("utf-8", { fatal: !0 }).decode(X);
      return A1(x), JSON.parse(x);
    }
    function A1(Z) {
      var E = 0;
      function A() {
        while (/\s/.test(Z[E] || ""))
          E += 1;
      }
      function B() {
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
      function S() {
        if (A(), Z[E] === "{") {
          E += 1, A();
          var C = new Set;
          if (Z[E] === "}") {
            E += 1;
            return;
          }
          for (;; ) {
            A();
            var X = B();
            if (C.has(X))
              throw Error("JSON object has a duplicate field");
            if (C.add(X), A(), Z[E++] !== ":")
              throw Error("JSON object is invalid");
            S(), A();
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
            S(), A();
            var _ = Z[E++];
            if (_ === "]")
              return;
            if (_ !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (Z[E] === '"') {
          B();
          return;
        }
        var x = Z.slice(E).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!x)
          throw Error("JSON value is invalid");
        E += x[0].length;
      }
      if (S(), A(), E !== Z.length)
        throw Error("JSON has trailing content");
    }
    function q(Z, E, A, B) {
      if (!P(Z))
        throw Error("marker is invalid");
      var S = Z, C = E.pathname.split("/").filter(Boolean), X = C[C.length - 1] || "";
      if (S.schema !== "airplan-upload" || S.version !== 6 || S.kind !== "document" || S.directory !== X || !/^[a-z2-7]{26}$/.test(S.directory) || !l1(S.created_at) || S.format !== "md" || !n1(S.slug) || S.entrypoint !== S.slug + ".html" || !P(S.producer) || S.producer.name !== "airplan" || typeof S.producer.version !== "string" || S.producer.version.trim() !== S.producer.version || S.producer.version === "" || !y(S.render) || !P(S.revision) || S.revision.number !== A.number || S.revision.chain_id !== B || (S.revision.number === 1 ? S.revision.previous_url !== void 0 : typeof S.revision.previous_url !== "string" || !g1(S.revision.previous_url)) || !Array.isArray(S.objects) || !Array.isArray(S.pages) || S.pages.length === 0)
        throw Error("marker identity is invalid");
      var _ = k(E, S.entrypoint);
      if (_ !== A.safeURL)
        throw Error("marker entrypoint is invalid");
      if (S.title !== void 0 && typeof S.title !== "string" || S.repo !== void 0 && !c1(S.repo) || S.objects.length === 0 || S.pages.length > 100)
        throw Error("marker shape is invalid");
      var x = d1(S), O = new Set, d = new Set, c = new Set, E1 = new Map;
      if (S.pages.forEach(function(M, b) {
        if (!P(M) || !u(M.path) || O.has(M.path) || d.has(M.path.toLowerCase()) || M.format !== "md" && M.format !== "txt" || typeof M.lang !== "string" || M.title !== void 0 && typeof M.title !== "string" || !u(M.page) || !u(M.source))
          throw Error("marker page descriptor is invalid");
        var H = Q(M.path, M.format), N = M.path;
        if (b === 0) {
          if (H = S.entrypoint, N = S.slug + ".md", M.format !== S.format)
            throw Error("marker entry format is invalid");
        }
        if (M.page !== H || M.source !== N)
          throw Error("marker generated page mapping is invalid");
        var T = k(E, M.page);
        if (!T || c.has(T))
          throw Error("marker page object is invalid");
        if (!k(E, M.source))
          throw Error("marker source object is invalid");
        if (x.get(M.page) !== "page" || x.get(M.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var q1 = S.objects.find(function(G1) {
          return G1.name === M.source;
        }).content_type;
        if (M.format === "md" && q1 !== "text/markdown; charset=utf-8" || M.format === "txt" && q1 !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        O.add(M.path), d.add(M.path.toLowerCase()), c.add(T), E1.set(M.path, T);
      }), m(d))
        throw Error("marker page paths conflict");
      if (!O.has(S.pages[0].path) || E1.get(S.pages[0].path) !== _)
        throw Error("marker entry page is invalid");
      if (c.size !== S.pages.length || Array.from(x.values()).filter(function(M) {
        return M === "source";
      }).length !== S.pages.length)
        throw Error("marker page inventory is invalid");
      return E1;
    }
    function Q(Z, E) {
      if (E !== "md")
        return Z + ".html";
      var A = Z.lastIndexOf("/"), B = Z.lastIndexOf(".");
      return (B > A ? Z.slice(0, B) : Z) + ".html";
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
      var A = Number(E[1]), B = Number(E[2]), S = Number(E[3]), C = Number(E[4]), X = Number(E[5]), _ = Number(E[6]), x = A % 4 === 0 && (A % 100 !== 0 || A % 400 === 0), O = [31, x ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return B >= 1 && B <= 12 && S >= 1 && S <= O[B - 1] && C <= 23 && X <= 59 && _ <= 59;
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
        var B = A[0], S = A[1].replace(/\.git$/, "");
        if (!B || !S || B === "." || B === ".." || S === "." || S === ".." || /[?#@:\\]/.test(B + S))
          return !1;
        return Z === "https://" + E.hostname.toLowerCase() + "/" + B + "/" + S;
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
      var E = new Map, A = new Set, B = 0, S = 0, C = 0, X = 0;
      if (Z.objects.forEach(function(_) {
        if (!P(_) || !u(_.name) && _.name !== ".airplan-changes.diff" || E.has(_.name) || A.has(_.name.toLowerCase()) || !Number.isSafeInteger(_.bytes) || _.bytes < 0 || !n(_.sha256) || !i1(_.content_type))
          throw Error("marker object inventory is invalid");
        if (_.role === "page") {
          if (S += 1, _.bytes <= 0 || _.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (_.role === "source") {
          if (C += 1, _.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (_.role === "asset")
          X += 1;
        else if (_.role === "diff") {
          if (B += 1, _.name !== ".airplan-changes.diff" || _.bytes <= 0 || _.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        E.set(_.name, _.role), A.add(_.name.toLowerCase());
      }), m(A))
        throw Error("marker object paths conflict");
      if (S !== Z.pages.length || C !== Z.pages.length || S + X > 100 || (Z.revision.number === 1 ? B !== 0 : B !== 1))
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
      if (!Number.isInteger(A) || A <= 0 || Z.current_revision !== A || !Number.isInteger(Z.latest_revision) || !Number.isInteger(Z.last_assigned_revision) || !Array.isArray(Z.revisions) || Z.revisions.length === 0 || Z.last_assigned_revision !== Z.revisions.length || !/^[a-z2-7]{26}$/.test(Z.chain_id) || F && F.content !== Z.chain_id)
        throw Error("revision identity is invalid");
      var B = !1, S = 0, C = Z.revisions.filter(function(H) {
        if (!H || !Number.isInteger(H.number) || H.number !== S + 1)
          return B = !0, !1;
        if (S = H.number, H.deleted)
          return !1;
        if (H.safeURL = o(H.url, !1), !H.safeURL)
          return B = !0, !1;
        if (H.number > 1) {
          var N = o(H.diff_url, !0);
          if (!N || new URL(N).pathname.replace(/[^/]+$/, "") !== new URL(H.safeURL).pathname.replace(/[^/]+$/, ""))
            return B = !0, !1;
        }
        return !0;
      });
      if (B || Z.revisions[0].number !== 1 || !C.some(function(H) {
        return H.number === A;
      }))
        throw Error("revision entries are invalid");
      var X = C.find(function(H) {
        return H.number === A;
      }), _ = new URL(window.location.href);
      if (_.search = "", _.hash = "", !X || !W || new URL(X.safeURL || "").pathname.replace(/[^/]+$/, "") !== W.pathname || !_.pathname.startsWith(W.pathname))
        throw Error("current revision URL is invalid");
      var x = Math.max.apply(null, C.map(function(H) {
        return H.number;
      }));
      if (x !== Z.latest_revision)
        throw Error("latest is invalid");
      var O = h.querySelector("[data-revision-heading]");
      if (!O) {
        O = h.createElement("p"), O.className = "revision-heading", O.setAttribute("data-revision-heading", "");
        var d = h.getElementById("rendered");
        if (!d)
          throw Error("rendered view is unavailable");
        d.prepend(O);
      }
      var c = A < x, E1 = c ? "Revision " + A + " of " + x : "Revision " + A + " (Latest)", M = h.createElement("span");
      M.className = "revision-picker-label", M.textContent = E1, M.setAttribute("aria-hidden", "true");
      var b = h.createElement("select");
      b.setAttribute("aria-label", "Document revision"), C.forEach(function(H) {
        var N = h.createElement("option");
        N.value = H.safeURL || "", N.textContent = H.number === x ? "Revision " + H.number + " (Latest)" : "Revision " + H.number + " of " + x, N.selected = H.number === A, b.appendChild(N);
      }), b.addEventListener("change", function() {
        var H = b.selectedIndex;
        if (H < 0 || H >= C.length)
          return;
        var N = C[H], T = N.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(T + (N.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var q1 = V ? new URL(V.content, window.location.href).href : "";
        if (!G || _.href === q1 || !F) {
          window.location.assign(T);
          return;
        }
        O.setAttribute("aria-busy", "true"), b.disabled = !0;
        var G1 = new URL("./", T), T1 = new URL(".airplan.json", G1);
        T1.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(T1, { cache: "no-store", credentials: "same-origin" }).then(t).then(function(e1) {
          var Z0 = q(e1, G1, N, F.content);
          window.location.assign(s1(T, Z0.get(G.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(T);
        });
      }), O.replaceChildren(M, b), O.classList.add("is-picker"), O.classList.toggle("is-stale", c), h.body.classList.toggle("airplan-stale-revision", c);
    }
    if (U) {
      var x1 = new URL(U.content, window.location.href);
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
    var i = null;
    function t1() {
      if (i !== null)
        return;
      i = Array.from(h.querySelectorAll("details:not([open])")), i.forEach(function(Z) {
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
      h1.textContent = E;
      var B = Z.querySelector(".action-label"), S = B ? B.textContent : "";
      if (B)
        B.textContent = A ? "Copied" : "Failed";
      Z.classList.add(A ? "is-copied" : "is-failed"), Z.disabled = !0, setTimeout(function() {
        if (Z.classList.remove("is-copied", "is-failed"), Z.disabled = !1, B)
          B.textContent = S;
      }, 1200);
    }
    function F1(Z, E) {
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
    var W1 = h.getElementById("pages"), f = h.querySelector(".pages-trigger"), I = null, X1 = window.matchMedia("(max-width: 78rem)"), D = function() {};
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
        var $1 = h.createElement("div");
        if ("popover" in $1 && typeof $1.showPopover === "function") {
          let Z = function() {
            if (!f || !I)
              return;
            var E = f.getBoundingClientRect(), A = f.closest(".toolbar"), B = A ? A.getBoundingClientRect().bottom : E.bottom;
            I.style.setProperty("--pages-left", Math.max(16, E.left) + "px"), I.style.setProperty("--pages-top", B + "px"), I.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, E.left) - 16) + "px");
          };
          I = $1, I.className = "pages-popover", I.id = "pages-popover", I.setAttribute("popover", "auto");
          var r = h.createElement("nav");
          r.className = "pages-popover-nav", r.setAttribute("aria-label", "Pages"), r.appendChild(L1.cloneNode(!0)), I.appendChild(r), f.setAttribute("popovertarget", I.id), f.popoverTargetElement = I, I.addEventListener("beforetoggle", function(E) {
            if (E.newState !== "open")
              return;
            D(), Z();
          }), I.addEventListener("toggle", function(E) {
            var A = E.newState === "open";
            if (f.setAttribute("aria-expanded", A ? "true" : "false"), h.body.classList.toggle("pages-popover-open", A), A) {
              var B = I.querySelector('[aria-current="page"]');
              if (B)
                B.scrollIntoView({ block: "nearest" });
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
          }), f.hidden = !1, f.setAttribute("aria-expanded", "false"), h.body.appendChild(I), h.body.classList.add("pages-popover-ready");
        }
      }
    }
    var g = h.getElementById("source"), S1 = h.getElementById("changes"), B1 = h.querySelector("[data-airplan-all-changes]"), v = h.getElementById("toc"), w = null, Y = null, N1 = window.matchMedia("(max-width: 78rem)");
    D = function() {
      if (Y && Y.open)
        Y.close();
    };
    function l() {
      if (!v || !w || !Y)
        return;
      var Z = N1.matches && !J.hidden && !Y.open && !Y1();
      if (w.classList.toggle("is-visible", Z), w.tabIndex = Z ? 0 : -1, w.setAttribute("aria-hidden", Z ? "false" : "true"), Y.open && (!N1.matches || J.hidden))
        D();
    }
    function U1(Z) {
      if (a(!1), D(), J.hidden = Z !== "rendered", g)
        g.hidden = Z !== "source";
      if (S1)
        S1.hidden = Z !== "changes";
      if (v)
        v.hidden = Z !== "rendered";
      h.querySelectorAll(".viewtoggle button").forEach(function(E) {
        var A = E.dataset.view === Z;
        E.classList.toggle("active", A), E.setAttribute("aria-pressed", A ? "true" : "false");
      }), l();
    }
    h.querySelectorAll(".viewtoggle button").forEach(function(Z) {
      Z.addEventListener("click", function() {
        U1(Z.dataset.view || "rendered");
      });
    });
    var z1 = !1;
    h.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(Z) {
      Z.addEventListener("click", function() {
        z1 = new URL(Z.href).pathname === window.location.pathname;
      });
    });
    function R1() {
      var Z = window.location.hash === "#airplan-all-changes" && !!B1;
      if (a(!1), D(), h.body.classList.toggle("all-changes-active", Z), B1)
        B1.hidden = !Z;
      if (Z) {
        if (J.hidden = !0, g)
          g.hidden = !0;
        if (S1)
          S1.hidden = !0;
        if (v)
          v.hidden = !0;
        if (z1)
          B1.querySelector("h1")?.focus();
      } else
        U1("rendered");
      z1 = !1, l();
    }
    if (window.addEventListener("hashchange", R1), R1(), v) {
      let Z = function() {
        if (e.length === 0) {
          l();
          return;
        }
        var A = 0;
        if (r1.forEach(function(S, C) {
          if (S && S.getBoundingClientRect().top <= 128)
            A = C;
        }), window.innerHeight + window.scrollY >= h.documentElement.scrollHeight - 2)
          A = e.length - 1;
        var B = e[A].getAttribute("href");
        J1.forEach(function(S) {
          var C = S.getAttribute("href") === B;
          if (S.classList.toggle("active", C), C)
            S.setAttribute("aria-current", "location");
          else
            S.removeAttribute("aria-current");
        }), l();
      }, E = function() {
        if (I1)
          return;
        I1 = !0, window.requestAnimationFrame(function() {
          I1 = !1, Z();
        });
      };
      var e = Array.from(v.querySelectorAll('a[href^="#"]')), f1 = v.querySelector(".toc-list");
      if (f1)
        if (Y = h.createElement("dialog"), typeof Y.showModal === "function") {
          Y.className = "toc-dialog", Y.id = "toc-dialog", Y.setAttribute("aria-labelledby", "toc-dialog-title");
          var _1 = h.createElement("div");
          _1.className = "toc-dialog-panel";
          var M1 = h.createElement("div");
          M1.className = "toc-dialog-header";
          var C1 = h.createElement("h2");
          C1.className = "toc-dialog-title", C1.id = "toc-dialog-title", C1.textContent = "Contents";
          var p = h.createElement("button");
          p.className = "toc-dialog-close", p.type = "button", p.setAttribute("aria-label", "Close table of contents"), p.innerHTML = m1, M1.appendChild(C1), M1.appendChild(p);
          var Z1 = h.createElement("nav");
          Z1.className = "toc-dialog-nav", Z1.setAttribute("aria-label", "Table of contents"), Z1.appendChild(f1.cloneNode(!0)), _1.appendChild(M1), _1.appendChild(Z1), Y.appendChild(_1), w = h.createElement("button"), w.className = "toc-trigger", w.type = "button", w.tabIndex = -1, w.setAttribute("aria-label", "Open table of contents"), w.setAttribute("aria-controls", "toc-dialog"), w.setAttribute("aria-haspopup", "dialog"), w.setAttribute("aria-hidden", "true"), w.innerHTML = k1, h.body.appendChild(w), h.body.appendChild(Y), h.body.classList.add("toc-dialog-ready"), w.addEventListener("click", function() {
            a(!1), Y.showModal(), h.body.classList.add("toc-dialog-open"), l();
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
            if (h.body.classList.remove("toc-dialog-open"), l(), w.classList.contains("is-visible"))
              setTimeout(function() {
                w.focus();
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
        return h.getElementById((A.getAttribute("href") || "").slice(1));
      }), I1 = !1;
      h.addEventListener("scroll", E, { passive: !0 }), window.addEventListener("resize", Z), Z();
    }
    var H1 = h.querySelector(".toolbar");
    function O1() {
      var Z = H1 && window.matchMedia("(max-width: 78rem)").matches ? H1.getBoundingClientRect().height : 0;
      h.documentElement.style.setProperty("--airplan-sticky-height", Z + "px");
    }
    if (H1) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(O1).observe(H1);
      window.addEventListener("resize", O1), O1();
    }
    let V1 = h.querySelector(".copy-source");
    if (V1 && g)
      V1.addEventListener("click", function() {
        var Z = g.querySelector("pre");
        F1(Z ? Z.textContent : "", V1);
      });
    J.querySelectorAll("pre").forEach(function(Z) {
      if (Z.classList.contains("mermaid"))
        return;
      var E = h.createElement("div");
      E.className = "codewrap", Z.parentNode?.insertBefore(E, Z), E.appendChild(Z);
      var A = h.createElement("button");
      A.className = "codecopy", A.type = "button", A.setAttribute("aria-label", "Copy code"), A.title = "Copy code", A.innerHTML = v1 + P1 + b1, A.addEventListener("click", function() {
        var B = Z.querySelector("code");
        F1((B || Z).textContent, A);
      }), E.appendChild(A);
    });
  })();
})();
