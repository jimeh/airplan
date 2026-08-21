(() => {
  function __(q) {
    return q === "system" || q === "light" || q === "dark";
  }
  function zE(q, z) {
    try {
      return q?.getItem(z) ?? null;
    } catch {
      return null;
    }
  }
  function s(q, z, O) {
    try {
      if (O === null)
        q?.removeItem(z);
      else
        q?.setItem(z, O);
    } catch {}
  }
  function fE(q, z, O) {
    let F = zE(O, "airplan-color-mode");
    if (F === null) {
      let L = zE(O, "airplan-theme");
      if (F = L === "light" || L === "dark" ? L : "system", F !== "system")
        s(O, "airplan-color-mode", F);
    }
    let x = __(F) ? F : "system", w = new Set(q.themes.map((L) => L.id)), $ = zE(O, "airplan-light-theme"), N = zE(O, "airplan-dark-theme"), I = $ !== null && w.has($) ? $ : q.defaultLight, U = N !== null && w.has(N) ? N : q.defaultDark;
    return ME(q, x, I, U, z);
  }
  function ME(q, z, O, F, x) {
    let w = new Map(q.themes.map((h) => [h.id, h])), $ = w.has(O) ? O : q.defaultLight, N = w.has(F) ? F : q.defaultDark, I = z === "system" ? x ? "dark" : "light" : z, U = I === "light" ? $ : N, L = w.get(U)?.variant ?? I;
    return { mode: z, resolvedMode: I, lightTheme: $, darkTheme: N, theme: U, variant: L };
  }
  function hE(q, z) {
    if (z === "system")
      s(q, "airplan-color-mode", null), s(q, "airplan-theme", null);
    else
      s(q, "airplan-color-mode", z), s(q, "airplan-theme", z);
  }
  function yE(q, z, O) {
    s(q, z === "light" ? "airplan-light-theme" : "airplan-dark-theme", O);
  }
  function PE(q) {
    return {
      mode: q.mode,
      resolvedMode: q.resolvedMode,
      theme: q.theme,
      variant: q.variant
    };
  }

  (function() {
    let q = document, z = q.documentElement;
    q.querySelectorAll(".js-only").forEach((Z) => {
      Z.hidden = !1;
    });
    let O = window.__AIRPLAN_THEME_CATALOG__;
    if (!O)
      return;
    let F = O, x = window.matchMedia("(prefers-color-scheme: dark)"), w;
    try {
      w = window.localStorage;
    } catch {}
    let $ = window.__airplanThemeState ?? fE(F, x.matches, w), N = q.querySelector("[data-airplan-appearance-trigger]"), I = q.querySelector("[data-airplan-appearance-panel]"), U = q.querySelector('select[data-airplan-theme-slot="light"]'), L = q.querySelector('select[data-airplan-theme-slot="dark"]'), h = Array.from(q.querySelectorAll("[data-airplan-color-mode]"));
    function o(Z) {
      if (!Z || Z.options.length > 0)
        return;
      for (let [J, P] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let T = q.createElement("optgroup");
        T.label = P;
        for (let u of F.themes) {
          if (u.variant !== J)
            continue;
          let n = q.createElement("option");
          n.value = u.id, n.textContent = u.name, T.append(n);
        }
        if (T.children.length > 0)
          Z.append(T);
      }
    }
    o(U), o(L);
    function y(Z, J = !0) {
      if ($ = Z, window.__airplanThemeState = $, z.dataset.airplanMode = $.mode, z.dataset.airplanResolvedMode = $.resolvedMode, z.dataset.airplanTheme = $.theme, z.dataset.airplanThemeVariant = $.variant, h.forEach((P) => {
        let T = P.dataset.airplanColorMode === $.mode;
        P.classList.toggle("active", T), P.setAttribute("aria-pressed", String(T));
      }), U)
        U.value = $.lightTheme;
      if (L)
        L.value = $.darkTheme;
      if (J)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: PE($) }));
    }
    function v(Z = {}) {
      y(ME(F, Z.mode ?? $.mode, Z.lightTheme ?? $.lightTheme, Z.darkTheme ?? $.darkTheme, x.matches));
    }
    function t(Z, J = !1) {
      if (!I || !N)
        return;
      if (I.hidden = !Z, N.setAttribute("aria-expanded", String(Z)), Z)
        I.querySelector("button,select")?.focus();
      else if (J)
        N.focus();
    }
    N?.addEventListener("click", () => t(Boolean(I?.hidden ?? !0))), h.forEach((Z) => Z.addEventListener("click", () => {
      let J = Z.dataset.airplanColorMode;
      if (!J)
        return;
      hE(w, J), v({ mode: J });
    }));
    function SE(Z, J) {
      yE(w, Z, J.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: J.value } })), v(Z === "light" ? { lightTheme: J.value } : { darkTheme: J.value });
    }
    U?.addEventListener("change", () => SE("light", U)), L?.addEventListener("change", () => SE("dark", L)), x.addEventListener("change", () => {
      if ($.mode === "system")
        v();
    }), q.addEventListener("keydown", (Z) => {
      if (Z.key === "Escape" && I && !I.hidden)
        Z.preventDefault(), t(!1, !0);
    }), q.addEventListener("pointerdown", (Z) => {
      if (!I || I.hidden || !N)
        return;
      let J = Z.target;
      if (!(J instanceof Node) || I.contains(J) || N.contains(J))
        return;
      let T = (J instanceof Element ? J : J.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), u = I.contains(q.activeElement) && !T;
      if (t(!1), u)
        setTimeout(() => {
          if (q.activeElement === q.body || I.contains(q.activeElement))
            N.focus();
        });
    }), y($, !1);
  })();

  (function() {
    var q = document, z = 262144;
    let O = q.getElementById("rendered");
    if (!O)
      return;
    let F = O;
    var x = q.querySelector('meta[name="airplan-versions"]'), w = q.querySelector('meta[name="airplan-revision-chain"]'), $ = q.querySelector('meta[name="airplan-page-path"]'), N = q.querySelector('meta[name="airplan-entrypoint"]'), I = x ? new URL(x.content, window.location.href) : null, U = I ? new URL("./", I) : null, L = U ? U.pathname.split("/").filter(Boolean) : [], h = L.slice(0, -1);
    function o(E, _) {
      if (typeof E !== "string")
        return null;
      try {
        var S = new URL(E);
        if (S.origin !== window.location.origin || S.username || S.password || S.search || S.hash)
          return null;
        var G = S.pathname.split("/").filter(Boolean);
        if (G.length !== h.length + 2 || !h.every(function(Q, X) {
          return G[X] === Q;
        }) || !/^[a-z2-7]{26}$/.test(G[G.length - 2]))
          return null;
        var A = G[G.length - 1];
        if (_ ? A !== ".airplan-changes.diff" : !A.endsWith(".html"))
          return null;
        return S.href;
      } catch {
        return null;
      }
    }
    function y(E) {
      if (typeof E !== "string" || E === "" || E.startsWith("/") || E.includes("\\"))
        return !1;
      var _ = E.split("/");
      return _.every(function(S) {
        var G = S.toLowerCase(), A = Array.from(S).some(function(Q) {
          var X = Q.codePointAt(0) || 0;
          return X < 32 || X === 127;
        });
        if (!S || S === "." || S === ".." || G.startsWith(".airplan-") || G === ".airplan.json" || A || /[. ]$/.test(S) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(S))
          return !1;
        return !0;
      });
    }
    function v(E, _) {
      if (!y(_))
        return null;
      var S = String(_).split("/").map(function(A) {
        return encodeURIComponent(A);
      }).join("/"), G = new URL(S, E);
      if (G.origin !== E.origin || G.username || G.password || G.search || G.hash || !G.pathname.startsWith(E.pathname))
        return null;
      return G.href;
    }
    async function t(E) {
      if (!E.ok)
        throw Error("marker request failed");
      var _ = E.headers.get("content-length");
      if (_ && /^\d+$/.test(_) && Number(_) > z) {
        if (E.body)
          await E.body.cancel("marker is too large");
        throw Error("marker is too large");
      }
      if (!E.body || typeof E.body.getReader !== "function")
        throw Error("bounded marker stream is unavailable");
      var S = E.body.getReader(), G = [], A = 0;
      try {
        for (;; ) {
          var Q = await S.read();
          if (Q.done)
            break;
          if (A += Q.value.byteLength, A > z)
            throw await S.cancel("marker is too large"), Error("marker is too large");
          G.push(Q.value);
        }
      } finally {
        S.releaseLock();
      }
      var X = new Uint8Array(A), B = 0;
      G.forEach(function(W) {
        X.set(W, B), B += W.byteLength;
      });
      var C = new TextDecoder("utf-8", { fatal: !0 }).decode(X);
      return SE(C), JSON.parse(C);
    }
    function SE(E) {
      var _ = 0;
      function S() {
        while (/\s/.test(E[_] || ""))
          _ += 1;
      }
      function G() {
        if (E[_] !== '"')
          throw Error("JSON string is invalid");
        var Q = _++;
        while (_ < E.length) {
          var X = E[_++];
          if (X === '"')
            return JSON.parse(E.slice(Q, _));
          if (X === "\\")
            _ += 1;
        }
        throw Error("JSON string is incomplete");
      }
      function A() {
        if (S(), E[_] === "{") {
          _ += 1, S();
          var Q = new Set;
          if (E[_] === "}") {
            _ += 1;
            return;
          }
          for (;; ) {
            S();
            var X = G();
            if (Q.has(X))
              throw Error("JSON object has a duplicate field");
            if (Q.add(X), S(), E[_++] !== ":")
              throw Error("JSON object is invalid");
            A(), S();
            var B = E[_++];
            if (B === "}")
              return;
            if (B !== ",")
              throw Error("JSON object is invalid");
          }
        }
        if (E[_] === "[") {
          if (_ += 1, S(), E[_] === "]") {
            _ += 1;
            return;
          }
          for (;; ) {
            A(), S();
            var B = E[_++];
            if (B === "]")
              return;
            if (B !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (E[_] === '"') {
          G();
          return;
        }
        var C = E.slice(_).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!C)
          throw Error("JSON value is invalid");
        _ += C[0].length;
      }
      if (A(), S(), _ !== E.length)
        throw Error("JSON has trailing content");
    }
    function Z(E, _, S, G) {
      if (!m(E))
        throw Error("marker is invalid");
      var A = E, Q = _.pathname.split("/").filter(Boolean), X = Q[Q.length - 1] || "";
      if (A.schema !== "airplan-upload" || A.version !== 6 || A.kind !== "document" || A.directory !== X || !/^[a-z2-7]{26}$/.test(A.directory) || !mE(A.created_at) || A.format !== "md" || !uE(A.slug) || A.entrypoint !== A.slug + ".html" || !m(A.producer) || A.producer.name !== "airplan" || typeof A.producer.version !== "string" || A.producer.version.trim() !== A.producer.version || A.producer.version === "" || !P(A.render) || !m(A.revision) || A.revision.number !== S.number || A.revision.chain_id !== G || (A.revision.number === 1 ? A.revision.previous_url !== void 0 : typeof A.revision.previous_url !== "string" || !vE(A.revision.previous_url)) || !Array.isArray(A.objects) || !Array.isArray(A.pages) || A.pages.length === 0)
        throw Error("marker identity is invalid");
      var B = v(_, A.entrypoint);
      if (B !== S.safeURL)
        throw Error("marker entrypoint is invalid");
      if (A.title !== void 0 && typeof A.title !== "string" || A.repo !== void 0 && !bE(A.repo) || A.objects.length === 0 || A.pages.length > 100)
        throw Error("marker shape is invalid");
      var C = lE(A), W = new Set, d = new Set, g = new Set, _E = new Map;
      if (A.pages.forEach(function(K, k) {
        if (!m(K) || !y(K.path) || W.has(K.path) || d.has(K.path.toLowerCase()) || K.format !== "md" && K.format !== "txt" || typeof K.lang !== "string" || K.title !== void 0 && typeof K.title !== "string" || !y(K.page) || !y(K.source))
          throw Error("marker page descriptor is invalid");
        var Y = J(K.path, K.format), R = K.path;
        if (k === 0) {
          if (Y = A.entrypoint, R = A.slug + ".md", K.format !== A.format)
            throw Error("marker entry format is invalid");
        }
        if (K.page !== Y || K.source !== R)
          throw Error("marker generated page mapping is invalid");
        var D = v(_, K.page);
        if (!D || g.has(D))
          throw Error("marker page object is invalid");
        if (!v(_, K.source))
          throw Error("marker source object is invalid");
        if (C.get(K.page) !== "page" || C.get(K.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var ZE = A.objects.find(function($E) {
          return $E.name === K.source;
        }).content_type;
        if (K.format === "md" && ZE !== "text/markdown; charset=utf-8" || K.format === "txt" && ZE !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        W.add(K.path), d.add(K.path.toLowerCase()), g.add(D), _E.set(K.path, D);
      }), u(d))
        throw Error("marker page paths conflict");
      if (!W.has(A.pages[0].path) || _E.get(A.pages[0].path) !== B)
        throw Error("marker entry page is invalid");
      if (g.size !== A.pages.length || Array.from(C.values()).filter(function(K) {
        return K === "source";
      }).length !== A.pages.length)
        throw Error("marker page inventory is invalid");
      return _E;
    }
    function J(E, _) {
      if (_ !== "md")
        return E + ".html";
      var S = E.lastIndexOf("/"), G = E.lastIndexOf(".");
      return (G > S ? E.slice(0, G) : E) + ".html";
    }
    function P(E) {
      if (!m(E) || !m(E.template) || !m(E.themes) || !Number.isInteger(E.generation) || E.generation <= 0 || typeof E.indexable !== "boolean" || typeof E.no_external_assets !== "boolean" || !E.template || E.template.kind !== "builtin" && E.template.kind !== "custom" || E.mermaid_url !== void 0 && !nE(E.mermaid_url) || !E.themes)
        return !1;
      if (E.template.kind === "builtin" && E.template.sha256 !== void 0 || E.template.kind === "custom" && !n(E.template.sha256))
        return !1;
      return T(E.themes.default_light) && T(E.themes.default_dark) && n(E.themes.catalog_sha256);
    }
    function T(E) {
      return typeof E === "string" && new TextEncoder().encode(E).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(E);
    }
    function u(E) {
      for (var _ of E) {
        var S = _.indexOf("/");
        while (S >= 0) {
          if (E.has(_.slice(0, S)))
            return !0;
          S = _.indexOf("/", S + 1);
        }
      }
      return !1;
    }
    function n(E) {
      return typeof E === "string" && /^[0-9a-f]{64}$/.test(E);
    }
    function m(E) {
      return !!E && typeof E === "object" && !Array.isArray(E);
    }
    function uE(E) {
      return typeof E === "string" && E.length <= 64 && /^[a-z0-9-]+$/.test(E);
    }
    function mE(E) {
      if (typeof E !== "string")
        return !1;
      var _ = E.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/);
      if (!_)
        return !1;
      var S = Number(_[1]), G = Number(_[2]), A = Number(_[3]), Q = Number(_[4]), X = Number(_[5]), B = Number(_[6]), C = S % 4 === 0 && (S % 100 !== 0 || S % 400 === 0), W = [31, C ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return G >= 1 && G <= 12 && A >= 1 && A <= W[G - 1] && Q <= 23 && X <= 59 && B <= 59;
    }
    function bE(E) {
      if (typeof E !== "string" || E === "" || E.trim() !== E)
        return !1;
      try {
        var _ = new URL(E);
        if (_.protocol !== "https:" || _.username || _.password || _.port || _.search || _.hash)
          return !1;
        var S = _.pathname.replace(/^\/+|\/+$/g, "").split("/");
        if (S.length !== 2)
          return !1;
        var G = S[0], A = S[1].replace(/\.git$/, "");
        if (!G || !A || G === "." || G === ".." || A === "." || A === ".." || /[?#@:\\]/.test(G + A))
          return !1;
        return E === "https://" + _.hostname.toLowerCase() + "/" + G + "/" + A;
      } catch {
        return !1;
      }
    }
    function kE(E) {
      return typeof E === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(E);
    }
    function vE(E) {
      try {
        var _ = new URL(E);
        return (_.protocol === "https:" || _.protocol === "http:") && !_.username && !_.password && !_.search && !_.hash && _.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function nE(E) {
      if (typeof E !== "string")
        return !1;
      try {
        var _ = new URL(E);
        return _.protocol === "https:" && !!_.host && !_.username && !_.password && !_.hash;
      } catch {
        return !1;
      }
    }
    function lE(E) {
      var _ = new Map, S = new Set, G = 0, A = 0, Q = 0, X = 0;
      if (E.objects.forEach(function(B) {
        if (!m(B) || !y(B.name) && B.name !== ".airplan-changes.diff" || _.has(B.name) || S.has(B.name.toLowerCase()) || !Number.isSafeInteger(B.bytes) || B.bytes < 0 || !n(B.sha256) || !kE(B.content_type))
          throw Error("marker object inventory is invalid");
        if (B.role === "page") {
          if (A += 1, B.bytes <= 0 || B.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (B.role === "source") {
          if (Q += 1, B.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (B.role === "asset")
          X += 1;
        else if (B.role === "diff") {
          if (G += 1, B.name !== ".airplan-changes.diff" || B.bytes <= 0 || B.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        _.set(B.name, B.role), S.add(B.name.toLowerCase());
      }), u(S))
        throw Error("marker object paths conflict");
      if (A !== E.pages.length || Q !== E.pages.length || A + X > 100 || (E.revision.number === 1 ? G !== 0 : G !== 1))
        throw Error("marker object counts are invalid");
      return _;
    }
    function gE(E, _) {
      var S = window.location.hash;
      if (S === "#airplan-all-changes")
        return E + S;
      if (!_)
        return E;
      return _ + (S && S !== "#airplan-all-changes" ? S : "");
    }
    function pE(E) {
      var _ = q.querySelector('meta[name="airplan-revision"]'), S = _ ? Number(_.content) : Number(E.current_revision);
      if (!Number.isInteger(S) || S <= 0 || E.current_revision !== S || !Number.isInteger(E.latest_revision) || !Number.isInteger(E.last_assigned_revision) || !Array.isArray(E.revisions) || E.revisions.length === 0 || E.last_assigned_revision !== E.revisions.length || !/^[a-z2-7]{26}$/.test(E.chain_id) || w && w.content !== E.chain_id)
        throw Error("revision identity is invalid");
      var G = !1, A = 0, Q = E.revisions.filter(function(Y) {
        if (!Y || !Number.isInteger(Y.number) || Y.number !== A + 1)
          return G = !0, !1;
        if (A = Y.number, Y.deleted)
          return !1;
        if (Y.safeURL = o(Y.url, !1), !Y.safeURL)
          return G = !0, !1;
        if (Y.number > 1) {
          var R = o(Y.diff_url, !0);
          if (!R || new URL(R).pathname.replace(/[^/]+$/, "") !== new URL(Y.safeURL).pathname.replace(/[^/]+$/, ""))
            return G = !0, !1;
        }
        return !0;
      });
      if (G || E.revisions[0].number !== 1 || !Q.some(function(Y) {
        return Y.number === S;
      }))
        throw Error("revision entries are invalid");
      var X = Q.find(function(Y) {
        return Y.number === S;
      }), B = new URL(window.location.href);
      if (B.search = "", B.hash = "", !X || !U || new URL(X.safeURL || "").pathname.replace(/[^/]+$/, "") !== U.pathname || !B.pathname.startsWith(U.pathname))
        throw Error("current revision URL is invalid");
      var C = Math.max.apply(null, Q.map(function(Y) {
        return Y.number;
      }));
      if (C !== E.latest_revision)
        throw Error("latest is invalid");
      var W = q.querySelector("[data-revision-heading]");
      if (!W) {
        W = q.createElement("p"), W.className = "revision-heading", W.setAttribute("data-revision-heading", "");
        var d = q.getElementById("rendered");
        if (!d)
          throw Error("rendered view is unavailable");
        d.prepend(W);
      }
      var g = S < C, _E = g ? "Revision " + S + " of " + C : "Revision " + S + " (Latest)", K = q.createElement("span");
      K.className = "revision-picker-label", K.textContent = _E, K.setAttribute("aria-hidden", "true");
      var k = q.createElement("select");
      k.setAttribute("aria-label", "Document revision"), Q.forEach(function(Y) {
        var R = q.createElement("option");
        R.value = Y.safeURL || "", R.textContent = Y.number === C ? "Revision " + Y.number + " (Latest)" : "Revision " + Y.number + " of " + C, R.selected = Y.number === S, k.appendChild(R);
      }), k.addEventListener("change", function() {
        var Y = k.selectedIndex;
        if (Y < 0 || Y >= Q.length)
          return;
        var R = Q[Y], D = R.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(D + (R.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var ZE = N ? new URL(N.content, window.location.href).href : "";
        if (!$ || B.href === ZE || !w) {
          window.location.assign(D);
          return;
        }
        W.setAttribute("aria-busy", "true"), k.disabled = !0;
        var $E = new URL("./", D), DE = new URL(".airplan.json", $E);
        DE.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(DE, { cache: "no-store", credentials: "same-origin" }).then(t).then(function(eE) {
          var E_ = Z(eE, $E, R, w.content);
          window.location.assign(gE(D, E_.get($.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(D);
        });
      }), W.replaceChildren(K, k), W.classList.add("is-picker"), W.classList.toggle("is-stale", g), q.body.classList.toggle("airplan-stale-revision", g);
    }
    if (x) {
      var CE = new URL(x.content, window.location.href);
      CE.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(CE, { cache: "no-store", credentials: "same-origin" }).then(function(E) {
        if (E.status === 404)
          return null;
        if (!E.ok)
          throw Error("metadata request failed");
        return E.json();
      }).then(function(E) {
        if (E === null)
          return;
        if (!E || E.schema !== "airplan-versions" || E.version !== 1 || !Array.isArray(E.revisions) || E.revisions.length < 2)
          throw Error("metadata is invalid");
        pE(E), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: E
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var qE = q.createElement("div");
    qE.className = "sr-status", qE.setAttribute("aria-live", "polite"), q.body.appendChild(qE);
    var p = null;
    function cE() {
      if (p !== null)
        return;
      p = Array.from(q.querySelectorAll("details:not([open])")), p.forEach(function(E) {
        E.open = !0;
      });
    }
    function iE() {
      if (p === null)
        return;
      p.forEach(function(E) {
        E.open = !1;
      }), p = null;
    }
    window.addEventListener("beforeprint", cE), window.addEventListener("afterprint", iE);
    function JE(E, _, S) {
      qE.textContent = _;
      var G = E.querySelector(".action-label"), A = G ? G.textContent : "";
      if (G)
        G.textContent = S ? "Copied" : "Failed";
      E.classList.add(S ? "is-copied" : "is-failed"), E.disabled = !0, setTimeout(function() {
        if (E.classList.remove("is-copied", "is-failed"), E.disabled = !1, G)
          G.textContent = A;
      }, 1200);
    }
    function wE(E, _) {
      if (!navigator.clipboard) {
        JE(_, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(E).then(function() {
        JE(_, "Copied!", !0);
      }, function() {
        JE(_, "Copy failed", !1);
      });
    }
    var dE = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', sE = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', oE = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', tE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', rE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', UE = q.getElementById("pages"), j = q.querySelector(".pages-trigger"), V = null, XE = window.matchMedia("(max-width: 78rem)"), f = function() {};
    function HE() {
      return V ? V.matches(":popover-open") : !1;
    }
    function r(E) {
      if (!V || !HE())
        return;
      if (V.hidePopover(), E && j && XE.matches)
        setTimeout(function() {
          j.focus();
        }, 0);
    }
    if (UE && j) {
      var LE = UE.querySelector(".pages-list");
      if (LE) {
        var IE = q.createElement("div");
        if ("popover" in IE && typeof IE.showPopover === "function") {
          let E = function() {
            if (!j || !V)
              return;
            var _ = j.getBoundingClientRect();
            V.style.setProperty("--pages-left", Math.max(16, _.left) + "px"), V.style.setProperty("--pages-top", _.bottom + "px"), V.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, _.left) - 16) + "px");
          };
          V = IE, V.className = "pages-popover", V.id = "pages-popover", V.setAttribute("popover", "auto");
          var a = q.createElement("nav");
          a.className = "pages-popover-nav", a.setAttribute("aria-label", "Pages"), a.appendChild(LE.cloneNode(!0)), V.appendChild(a), j.setAttribute("popovertarget", V.id), j.popoverTargetElement = V, V.addEventListener("beforetoggle", function(_) {
            if (_.newState !== "open")
              return;
            f(), E();
          }), V.addEventListener("toggle", function(_) {
            var S = _.newState === "open";
            if (j.setAttribute("aria-expanded", S ? "true" : "false"), q.body.classList.toggle("pages-popover-open", S), S) {
              var G = V.querySelector('[aria-current="page"]');
              if (G)
                G.scrollIntoView({ block: "nearest" });
            }
            l();
          }), a.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", function() {
              r(!1);
            });
          }), XE.addEventListener("change", function() {
            if (!XE.matches)
              r(!1);
          }), window.addEventListener("resize", function() {
            if (HE())
              E();
          }), j.hidden = !1, j.setAttribute("aria-expanded", "false"), q.body.appendChild(V), q.body.classList.add("pages-popover-ready");
        }
      }
    }
    var c = q.getElementById("source"), AE = q.getElementById("changes"), GE = q.querySelector("[data-airplan-all-changes]"), b = q.getElementById("toc"), M = null, H = null, RE = window.matchMedia("(max-width: 78rem)");
    f = function() {
      if (H && H.open)
        H.close();
    };
    function l() {
      if (!b || !M || !H)
        return;
      var E = RE.matches && !F.hidden && !H.open && !HE();
      if (M.classList.toggle("is-visible", E), M.tabIndex = E ? 0 : -1, M.setAttribute("aria-hidden", E ? "false" : "true"), H.open && (!RE.matches || F.hidden))
        f();
    }
    function xE(E) {
      if (r(!1), f(), F.hidden = E !== "rendered", c)
        c.hidden = E !== "source";
      if (AE)
        AE.hidden = E !== "changes";
      if (b)
        b.hidden = E !== "rendered";
      q.querySelectorAll(".viewtoggle button").forEach(function(_) {
        var S = _.dataset.view === E;
        _.classList.toggle("active", S), _.setAttribute("aria-pressed", S ? "true" : "false");
      }), l();
    }
    q.querySelectorAll(".viewtoggle button").forEach(function(E) {
      E.addEventListener("click", function() {
        xE(E.dataset.view || "rendered");
      });
    });
    var OE = !1;
    q.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(E) {
      E.addEventListener("click", function() {
        OE = new URL(E.href).pathname === window.location.pathname;
      });
    });
    function TE() {
      var E = window.location.hash === "#airplan-all-changes" && !!GE;
      if (r(!1), f(), q.body.classList.toggle("all-changes-active", E), GE)
        GE.hidden = !E;
      if (E) {
        if (F.hidden = !0, c)
          c.hidden = !0;
        if (AE)
          AE.hidden = !0;
        if (b)
          b.hidden = !0;
        if (OE)
          GE.querySelector("h1")?.focus();
      } else
        xE("rendered");
      OE = !1, l();
    }
    if (window.addEventListener("hashchange", TE), TE(), b) {
      let E = function() {
        if (e.length === 0) {
          l();
          return;
        }
        var S = 0;
        if (aE.forEach(function(A, Q) {
          if (A && A.getBoundingClientRect().top <= 128)
            S = Q;
        }), window.innerHeight + window.scrollY >= q.documentElement.scrollHeight - 2)
          S = e.length - 1;
        var G = e[S].getAttribute("href");
        FE.forEach(function(A) {
          var Q = A.getAttribute("href") === G;
          if (A.classList.toggle("active", Q), Q)
            A.setAttribute("aria-current", "location");
          else
            A.removeAttribute("aria-current");
        }), l();
      }, _ = function() {
        if (VE)
          return;
        VE = !0, window.requestAnimationFrame(function() {
          VE = !1, E();
        });
      };
      var e = Array.from(b.querySelectorAll('a[href^="#"]')), jE = b.querySelector(".toc-list");
      if (jE)
        if (H = q.createElement("dialog"), typeof H.showModal === "function") {
          H.className = "toc-dialog", H.id = "toc-dialog", H.setAttribute("aria-labelledby", "toc-dialog-title");
          var BE = q.createElement("div");
          BE.className = "toc-dialog-panel";
          var KE = q.createElement("div");
          KE.className = "toc-dialog-header";
          var QE = q.createElement("h2");
          QE.className = "toc-dialog-title", QE.id = "toc-dialog-title", QE.textContent = "Contents";
          var i = q.createElement("button");
          i.className = "toc-dialog-close", i.type = "button", i.setAttribute("aria-label", "Close table of contents"), i.innerHTML = rE, KE.appendChild(QE), KE.appendChild(i);
          var EE = q.createElement("nav");
          EE.className = "toc-dialog-nav", EE.setAttribute("aria-label", "Table of contents"), EE.appendChild(jE.cloneNode(!0)), BE.appendChild(KE), BE.appendChild(EE), H.appendChild(BE), M = q.createElement("button"), M.className = "toc-trigger", M.type = "button", M.tabIndex = -1, M.setAttribute("aria-label", "Open table of contents"), M.setAttribute("aria-controls", "toc-dialog"), M.setAttribute("aria-haspopup", "dialog"), M.setAttribute("aria-hidden", "true"), M.innerHTML = tE, q.body.appendChild(M), q.body.appendChild(H), q.body.classList.add("toc-dialog-ready"), M.addEventListener("click", function() {
            r(!1), H.showModal(), q.body.classList.add("toc-dialog-open"), l();
            var S = H.querySelector("a.active");
            if (S)
              S.scrollIntoView({ block: "nearest" });
          }), i.addEventListener("click", f), H.addEventListener("click", function(S) {
            if (S.target === H)
              f();
          }), H.addEventListener("keydown", function(S) {
            if (S.key === "Escape")
              S.preventDefault(), f();
          }), H.addEventListener("close", function() {
            if (q.body.classList.remove("toc-dialog-open"), l(), M.classList.contains("is-visible"))
              setTimeout(function() {
                M.focus();
              }, 50);
          }), EE.querySelectorAll("a").forEach(function(S) {
            S.addEventListener("click", f);
          });
        } else
          H = null;
      var FE = e.slice();
      if (H)
        FE = FE.concat(Array.from(H.querySelectorAll('a[href^="#"]')));
      var aE = e.map(function(S) {
        return q.getElementById((S.getAttribute("href") || "").slice(1));
      }), VE = !1;
      q.addEventListener("scroll", _, { passive: !0 }), window.addEventListener("resize", E), E();
    }
    var YE = q.querySelector(".toolbar");
    function WE() {
      var E = YE && window.matchMedia("(max-width: 78rem)").matches ? YE.getBoundingClientRect().height : 0;
      q.documentElement.style.setProperty("--airplan-sticky-height", E + "px");
    }
    if (YE) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(WE).observe(YE);
      window.addEventListener("resize", WE), WE();
    }
    let NE = q.querySelector(".copy-source");
    if (NE && c)
      NE.addEventListener("click", function() {
        var E = c.querySelector("pre");
        wE(E ? E.textContent : "", NE);
      });
    F.querySelectorAll("pre").forEach(function(E) {
      if (E.classList.contains("mermaid"))
        return;
      var _ = q.createElement("div");
      _.className = "codewrap", E.parentNode?.insertBefore(_, E), _.appendChild(E);
      var S = q.createElement("button");
      S.className = "codecopy", S.type = "button", S.setAttribute("aria-label", "Copy code"), S.title = "Copy code", S.innerHTML = dE + sE + oE, S.addEventListener("click", function() {
        var G = E.querySelector("code");
        wE((G || E).textContent, S);
      }), _.appendChild(S);
    });
  })();
})();
