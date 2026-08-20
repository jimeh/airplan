(() => {
  function E_(q) {
    return q === "system" || q === "light" || q === "dark";
  }
  function zE(q, $) {
    try {
      return q?.getItem($) ?? null;
    } catch {
      return null;
    }
  }
  function s(q, $, F) {
    try {
      if (F === null)
        q?.removeItem($);
      else
        q?.setItem($, F);
    } catch {}
  }
  function xE(q, $, F) {
    let V = zE(F, "airplan-color-mode");
    if (V === null) {
      let U = zE(F, "airplan-theme");
      if (V = U === "light" || U === "dark" ? U : "system", V !== "system")
        s(F, "airplan-color-mode", V);
    }
    let R = E_(V) ? V : "system", C = new Set(q.themes.map((U) => U.id)), Z = zE(F, "airplan-light-theme"), M = zE(F, "airplan-dark-theme"), O = Z !== null && C.has(Z) ? Z : q.defaultLight, w = M !== null && C.has(M) ? M : q.defaultDark;
    return CE(q, R, O, w, $);
  }
  function CE(q, $, F, V, R) {
    let C = new Map(q.themes.map((y) => [y.id, y])), Z = C.has(F) ? F : q.defaultLight, M = C.has(V) ? V : q.defaultDark, O = $ === "system" ? R ? "dark" : "light" : $, w = O === "light" ? Z : M, U = C.get(w)?.variant ?? O;
    return { mode: $, resolvedMode: O, lightTheme: Z, darkTheme: M, theme: w, variant: U };
  }
  function hE(q, $) {
    if ($ === "system")
      s(q, "airplan-color-mode", null), s(q, "airplan-theme", null);
    else
      s(q, "airplan-color-mode", $), s(q, "airplan-theme", $);
  }
  function yE(q, $, F) {
    s(q, $ === "light" ? "airplan-light-theme" : "airplan-dark-theme", F);
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
    let q = document, $ = q.documentElement;
    q.querySelectorAll(".js-only").forEach((Y) => {
      Y.hidden = !1;
    });
    let F = window.__AIRPLAN_THEME_CATALOG__;
    if (!F)
      return;
    let V = F, R = window.matchMedia("(prefers-color-scheme: dark)"), C;
    try {
      C = window.localStorage;
    } catch {}
    let Z = window.__airplanThemeState ?? xE(V, R.matches, C), M = q.querySelector("[data-airplan-appearance-trigger]"), O = q.querySelector("[data-airplan-appearance-panel]"), w = q.querySelector('select[data-airplan-theme-slot="light"]'), U = q.querySelector('select[data-airplan-theme-slot="dark"]'), y = Array.from(q.querySelectorAll("[data-airplan-color-mode]"));
    function o(Y) {
      if (!Y || Y.options.length > 0)
        return;
      for (let [z, u] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let T = q.createElement("optgroup");
        T.label = u;
        for (let x of V.themes) {
          if (x.variant !== z)
            continue;
          let j = q.createElement("option");
          j.value = x.id, j.textContent = x.name, T.append(j);
        }
        if (T.children.length > 0)
          Y.append(T);
      }
    }
    o(w), o(U);
    function P(Y, z = !0) {
      if (Z = Y, window.__airplanThemeState = Z, $.dataset.airplanMode = Z.mode, $.dataset.airplanResolvedMode = Z.resolvedMode, $.dataset.airplanTheme = Z.theme, $.dataset.airplanThemeVariant = Z.variant, y.forEach((u) => {
        let T = u.dataset.airplanColorMode === Z.mode;
        u.classList.toggle("active", T), u.setAttribute("aria-pressed", String(T));
      }), w)
        w.value = Z.lightTheme;
      if (U)
        U.value = Z.darkTheme;
      if (z)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: PE(Z) }));
    }
    function v(Y = {}) {
      P(CE(V, Y.mode ?? Z.mode, Y.lightTheme ?? Z.lightTheme, Y.darkTheme ?? Z.darkTheme, R.matches));
    }
    function t(Y, z = !1) {
      if (!O || !M)
        return;
      if (O.hidden = !Y, M.setAttribute("aria-expanded", String(Y)), Y)
        O.querySelector("button,select")?.focus();
      else if (z)
        M.focus();
    }
    M?.addEventListener("click", () => t(Boolean(O?.hidden ?? !0))), y.forEach((Y) => Y.addEventListener("click", () => {
      let z = Y.dataset.airplanColorMode;
      if (!z)
        return;
      hE(C, z), v({ mode: z });
    }));
    function qE(Y, z) {
      yE(C, Y, z.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: z.value } })), v(Y === "light" ? { lightTheme: z.value } : { darkTheme: z.value });
    }
    w?.addEventListener("change", () => qE("light", w)), U?.addEventListener("change", () => qE("dark", U)), R.addEventListener("change", () => {
      if (Z.mode === "system")
        v();
    }), q.addEventListener("keydown", (Y) => {
      if (Y.key === "Escape" && O && !O.hidden)
        Y.preventDefault(), t(!1, !0);
    }), q.addEventListener("pointerdown", (Y) => {
      if (!O || O.hidden || !M)
        return;
      let z = Y.target;
      if (!(z instanceof Node) || O.contains(z) || M.contains(z))
        return;
      let T = (z instanceof Element ? z : z.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), x = O.contains(q.activeElement) && !T;
      if (t(!1), x)
        setTimeout(() => {
          if (q.activeElement === q.body || O.contains(q.activeElement))
            M.focus();
        });
    }), P(Z, !1);
  })();

  (function() {
    var q = document, $ = 262144;
    let F = q.getElementById("rendered");
    if (!F)
      return;
    let V = F;
    var R = q.querySelector('meta[name="airplan-versions"]'), C = q.querySelector('meta[name="airplan-revision-chain"]'), Z = q.querySelector('meta[name="airplan-page-path"]'), M = q.querySelector('meta[name="airplan-entrypoint"]'), O = R ? new URL(R.content, window.location.href) : null, w = O ? new URL("./", O) : null, U = w ? w.pathname.split("/").filter(Boolean) : [], y = U.slice(0, -1);
    function o(E, _) {
      if (typeof E !== "string")
        return null;
      try {
        var S = new URL(E);
        if (S.origin !== window.location.origin || S.username || S.password || S.search || S.hash)
          return null;
        var G = S.pathname.split("/").filter(Boolean);
        if (G.length !== y.length + 2 || !y.every(function(Q, J) {
          return G[J] === Q;
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
    function P(E) {
      if (typeof E !== "string" || E === "" || E.startsWith("/") || E.includes("\\"))
        return !1;
      var _ = E.split("/");
      return _.every(function(S) {
        var G = S.toLowerCase(), A = Array.from(S).some(function(Q) {
          var J = Q.codePointAt(0) || 0;
          return J < 32 || J === 127;
        });
        if (!S || S === "." || S === ".." || G.startsWith(".airplan-") || G === ".airplan.json" || A || /[. ]$/.test(S) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(S))
          return !1;
        return !0;
      });
    }
    function v(E, _) {
      if (!P(_))
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
      if (_ && /^\d+$/.test(_) && Number(_) > $) {
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
          if (A += Q.value.byteLength, A > $)
            throw await S.cancel("marker is too large"), Error("marker is too large");
          G.push(Q.value);
        }
      } finally {
        S.releaseLock();
      }
      var J = new Uint8Array(A), H = 0;
      G.forEach(function(K) {
        J.set(K, H), H += K.byteLength;
      });
      var I = new TextDecoder("utf-8", { fatal: !0 }).decode(J);
      return qE(I), JSON.parse(I);
    }
    function qE(E) {
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
          var J = E[_++];
          if (J === '"')
            return JSON.parse(E.slice(Q, _));
          if (J === "\\")
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
            var J = G();
            if (Q.has(J))
              throw Error("JSON object has a duplicate field");
            if (Q.add(J), S(), E[_++] !== ":")
              throw Error("JSON object is invalid");
            A(), S();
            var H = E[_++];
            if (H === "}")
              return;
            if (H !== ",")
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
            var H = E[_++];
            if (H === "]")
              return;
            if (H !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (E[_] === '"') {
          G();
          return;
        }
        var I = E.slice(_).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!I)
          throw Error("JSON value is invalid");
        _ += I[0].length;
      }
      if (A(), S(), _ !== E.length)
        throw Error("JSON has trailing content");
    }
    function Y(E, _, S, G) {
      if (!j(E))
        throw Error("marker is invalid");
      var A = E, Q = _.pathname.split("/").filter(Boolean), J = Q[Q.length - 1] || "";
      if (A.schema !== "airplan-upload" || A.version !== 6 || A.kind !== "document" || A.directory !== J || !/^[a-z2-7]{26}$/.test(A.directory) || !mE(A.created_at) || A.format !== "md" || !uE(A.slug) || A.entrypoint !== A.slug + ".html" || !j(A.producer) || A.producer.name !== "airplan" || typeof A.producer.version !== "string" || A.producer.version.trim() !== A.producer.version || A.producer.version === "" || !u(A.render) || !j(A.revision) || A.revision.number !== S.number || A.revision.chain_id !== G || (A.revision.number === 1 ? A.revision.previous_url !== void 0 : typeof A.revision.previous_url !== "string" || !vE(A.revision.previous_url)) || !Array.isArray(A.objects) || !Array.isArray(A.pages) || A.pages.length === 0)
        throw Error("marker identity is invalid");
      var H = v(_, A.entrypoint);
      if (H !== S.safeURL)
        throw Error("marker entrypoint is invalid");
      if (A.title !== void 0 && typeof A.title !== "string" || A.repo !== void 0 && !bE(A.repo) || A.objects.length === 0 || A.pages.length > 100)
        throw Error("marker shape is invalid");
      var I = lE(A), K = new Set, d = new Set, l = new Set, _E = new Map;
      A.pages.forEach(function(B, L) {
        if (!j(B) || !P(B.path) || K.has(B.path) || d.has(B.path.toLowerCase()) || B.format !== "md" && B.format !== "txt" || typeof B.lang !== "string" || B.title !== void 0 && typeof B.title !== "string" || !P(B.page) || !P(B.source))
          throw Error("marker page descriptor is invalid");
        var k = z(B.path, B.format), $E = B.path;
        if (L === 0) {
          if (k = A.entrypoint, $E = A.slug + ".md", B.format !== A.format)
            throw Error("marker entry format is invalid");
        }
        if (B.page !== k || B.source !== $E)
          throw Error("marker generated page mapping is invalid");
        var g = v(_, B.page);
        if (!g || l.has(g))
          throw Error("marker page object is invalid");
        if (!v(_, B.source))
          throw Error("marker source object is invalid");
        if (I.get(B.page) !== "page" || I.get(B.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var SE = A.objects.find(function(NE) {
          return NE.name === B.source;
        }).content_type;
        if (B.format === "md" && SE !== "text/markdown; charset=utf-8" || B.format === "txt" && SE !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        K.add(B.path), d.add(B.path.toLowerCase()), l.add(g), _E.set(B.path, g);
      });
      var b = Array.from(d).sort();
      for (var f = 1;f < b.length; f += 1)
        if (b[f].startsWith(b[f - 1] + "/"))
          throw Error("marker page paths conflict");
      if (!K.has(A.pages[0].path) || _E.get(A.pages[0].path) !== H)
        throw Error("marker entry page is invalid");
      if (l.size !== A.pages.length || Array.from(I.values()).filter(function(B) {
        return B === "source";
      }).length !== A.pages.length)
        throw Error("marker page inventory is invalid");
      return _E;
    }
    function z(E, _) {
      if (_ !== "md")
        return E + ".html";
      var S = E.lastIndexOf("/"), G = E.lastIndexOf(".");
      return (G > S ? E.slice(0, G) : E) + ".html";
    }
    function u(E) {
      if (!j(E) || !j(E.template) || !j(E.themes) || !Number.isInteger(E.generation) || E.generation <= 0 || typeof E.indexable !== "boolean" || typeof E.no_external_assets !== "boolean" || !E.template || E.template.kind !== "builtin" && E.template.kind !== "custom" || E.mermaid_url !== void 0 && !nE(E.mermaid_url) || !E.themes)
        return !1;
      if (E.template.kind === "builtin" && E.template.sha256 !== void 0 || E.template.kind === "custom" && !x(E.template.sha256))
        return !1;
      return T(E.themes.default_light) && T(E.themes.default_dark) && x(E.themes.catalog_sha256);
    }
    function T(E) {
      return typeof E === "string" && new TextEncoder().encode(E).byteLength <= 48 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(E);
    }
    function x(E) {
      return typeof E === "string" && /^[0-9a-f]{64}$/.test(E);
    }
    function j(E) {
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
      var S = Number(_[1]), G = Number(_[2]), A = Number(_[3]), Q = Number(_[4]), J = Number(_[5]), H = Number(_[6]), I = S % 4 === 0 && (S % 100 !== 0 || S % 400 === 0), K = [31, I ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return G >= 1 && G <= 12 && A >= 1 && A <= K[G - 1] && Q <= 23 && J <= 59 && H <= 59;
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
      var _ = new Map, S = new Set, G = 0, A = 0, Q = 0, J = 0;
      E.objects.forEach(function(K) {
        if (!j(K) || !P(K.name) && K.name !== ".airplan-changes.diff" || _.has(K.name) || S.has(K.name.toLowerCase()) || !Number.isSafeInteger(K.bytes) || K.bytes < 0 || !x(K.sha256) || !kE(K.content_type))
          throw Error("marker object inventory is invalid");
        if (K.role === "page") {
          if (A += 1, K.bytes <= 0 || K.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (K.role === "source") {
          if (Q += 1, K.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (K.role === "asset")
          J += 1;
        else if (K.role === "diff") {
          if (G += 1, K.name !== ".airplan-changes.diff" || K.bytes <= 0 || K.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        _.set(K.name, K.role), S.add(K.name.toLowerCase());
      });
      var H = Array.from(S).sort();
      for (var I = 1;I < H.length; I += 1)
        if (H[I].startsWith(H[I - 1] + "/"))
          throw Error("marker object paths conflict");
      if (A !== E.pages.length || Q !== E.pages.length || A + J > 100 || (E.revision.number === 1 ? G !== 0 : G !== 1))
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
      if (!Number.isInteger(S) || S <= 0 || E.current_revision !== S || !Number.isInteger(E.latest_revision) || !Number.isInteger(E.last_assigned_revision) || !Array.isArray(E.revisions) || E.revisions.length === 0 || E.last_assigned_revision !== E.revisions.length || !/^[a-z2-7]{26}$/.test(E.chain_id) || C && C.content !== E.chain_id)
        throw Error("revision identity is invalid");
      var G = !1, A = 0, Q = E.revisions.filter(function(B) {
        if (!B || !Number.isInteger(B.number) || B.number !== A + 1)
          return G = !0, !1;
        if (A = B.number, B.deleted)
          return !1;
        if (B.safeURL = o(B.url, !1), !B.safeURL)
          return G = !0, !1;
        if (B.number > 1) {
          var L = o(B.diff_url, !0);
          if (!L || new URL(L).pathname.replace(/[^/]+$/, "") !== new URL(B.safeURL).pathname.replace(/[^/]+$/, ""))
            return G = !0, !1;
        }
        return !0;
      });
      if (G || E.revisions[0].number !== 1 || !Q.some(function(B) {
        return B.number === S;
      }))
        throw Error("revision entries are invalid");
      var J = Q.find(function(B) {
        return B.number === S;
      }), H = new URL(window.location.href);
      if (H.search = "", H.hash = "", !J || !w || new URL(J.safeURL || "").pathname.replace(/[^/]+$/, "") !== w.pathname || !H.pathname.startsWith(w.pathname))
        throw Error("current revision URL is invalid");
      var I = Math.max.apply(null, Q.map(function(B) {
        return B.number;
      }));
      if (I !== E.latest_revision)
        throw Error("latest is invalid");
      var K = q.querySelector("[data-revision-heading]");
      if (!K) {
        K = q.createElement("p"), K.className = "revision-heading", K.setAttribute("data-revision-heading", "");
        var d = q.getElementById("rendered");
        if (!d)
          throw Error("rendered view is unavailable");
        d.prepend(K);
      }
      var l = S < I, _E = l ? "Revision " + S + " of " + I : "Revision " + S + " (Latest)", b = q.createElement("span");
      b.className = "revision-picker-label", b.textContent = _E, b.setAttribute("aria-hidden", "true");
      var f = q.createElement("select");
      f.setAttribute("aria-label", "Document revision"), Q.forEach(function(B) {
        var L = q.createElement("option");
        L.value = B.safeURL || "", L.textContent = B.number === I ? "Revision " + B.number + " (Latest)" : "Revision " + B.number + " of " + I, L.selected = B.number === S, f.appendChild(L);
      }), f.addEventListener("change", function() {
        var B = f.selectedIndex;
        if (B < 0 || B >= Q.length)
          return;
        var L = Q[B], k = L.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(k + (L.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var $E = M ? new URL(M.content, window.location.href).href : "";
        if (!Z || H.href === $E || !C) {
          window.location.assign(k);
          return;
        }
        K.setAttribute("aria-busy", "true"), f.disabled = !0;
        var g = new URL("./", k), SE = new URL(".airplan.json", g);
        SE.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(SE, { cache: "no-store", credentials: "same-origin" }).then(t).then(function(NE) {
          var eE = Y(NE, g, L, C.content);
          window.location.assign(gE(k, eE.get(Z.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(k);
        });
      }), K.replaceChildren(b, f), K.classList.add("is-picker"), K.classList.toggle("is-stale", l), q.body.classList.toggle("airplan-stale-revision", l);
    }
    if (R) {
      var wE = new URL(R.content, window.location.href);
      wE.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(wE, { cache: "no-store", credentials: "same-origin" }).then(function(E) {
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
    var AE = q.createElement("div");
    AE.className = "sr-status", AE.setAttribute("aria-live", "polite"), q.body.appendChild(AE);
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
      AE.textContent = _;
      var G = E.querySelector(".action-label"), A = G ? G.textContent : "";
      if (G)
        G.textContent = S ? "Copied" : "Failed";
      E.classList.add(S ? "is-copied" : "is-failed"), E.disabled = !0, setTimeout(function() {
        if (E.classList.remove("is-copied", "is-failed"), E.disabled = !1, G)
          G.textContent = A;
      }, 1200);
    }
    function UE(E, _) {
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
    var dE = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', sE = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', oE = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', tE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', rE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', LE = q.getElementById("pages"), D = q.querySelector(".pages-trigger"), W = null, XE = window.matchMedia("(max-width: 78rem)"), h = function() {};
    function HE() {
      return W ? W.matches(":popover-open") : !1;
    }
    function r(E) {
      if (!W || !HE())
        return;
      if (W.hidePopover(), E && D && XE.matches)
        setTimeout(function() {
          D.focus();
        }, 0);
    }
    if (LE && D) {
      var RE = LE.querySelector(".pages-list");
      if (RE) {
        var IE = q.createElement("div");
        if ("popover" in IE && typeof IE.showPopover === "function") {
          let E = function() {
            if (!D || !W)
              return;
            var _ = D.getBoundingClientRect();
            W.style.setProperty("--pages-left", Math.max(16, _.left) + "px"), W.style.setProperty("--pages-top", _.bottom + "px"), W.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, _.left) - 16) + "px");
          };
          W = IE, W.className = "pages-popover", W.id = "pages-popover", W.setAttribute("popover", "auto");
          var a = q.createElement("nav");
          a.className = "pages-popover-nav", a.setAttribute("aria-label", "Pages"), a.appendChild(RE.cloneNode(!0)), W.appendChild(a), D.setAttribute("popovertarget", W.id), D.popoverTargetElement = W, W.addEventListener("beforetoggle", function(_) {
            if (_.newState !== "open")
              return;
            h(), E();
          }), W.addEventListener("toggle", function(_) {
            var S = _.newState === "open";
            if (D.setAttribute("aria-expanded", S ? "true" : "false"), q.body.classList.toggle("pages-popover-open", S), S) {
              var G = W.querySelector('[aria-current="page"]');
              if (G)
                G.scrollIntoView({ block: "nearest" });
            }
            n();
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
          }), D.hidden = !1, D.setAttribute("aria-expanded", "false"), q.body.appendChild(W), q.body.classList.add("pages-popover-ready");
        }
      }
    }
    var c = q.getElementById("source"), GE = q.getElementById("changes"), BE = q.querySelector("[data-airplan-all-changes]"), m = q.getElementById("toc"), N = null, X = null, TE = window.matchMedia("(max-width: 78rem)");
    h = function() {
      if (X && X.open)
        X.close();
    };
    function n() {
      if (!m || !N || !X)
        return;
      var E = TE.matches && !V.hidden && !X.open && !HE();
      if (N.classList.toggle("is-visible", E), N.tabIndex = E ? 0 : -1, N.setAttribute("aria-hidden", E ? "false" : "true"), X.open && (!TE.matches || V.hidden))
        h();
    }
    function jE(E) {
      if (r(!1), h(), V.hidden = E !== "rendered", c)
        c.hidden = E !== "source";
      if (GE)
        GE.hidden = E !== "changes";
      if (m)
        m.hidden = E !== "rendered";
      q.querySelectorAll(".viewtoggle button").forEach(function(_) {
        var S = _.dataset.view === E;
        _.classList.toggle("active", S), _.setAttribute("aria-pressed", S ? "true" : "false");
      }), n();
    }
    q.querySelectorAll(".viewtoggle button").forEach(function(E) {
      E.addEventListener("click", function() {
        jE(E.dataset.view || "rendered");
      });
    });
    var OE = !1;
    q.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(E) {
      E.addEventListener("click", function() {
        OE = new URL(E.href).pathname === window.location.pathname;
      });
    });
    function fE() {
      var E = window.location.hash === "#airplan-all-changes" && !!BE;
      if (r(!1), h(), q.body.classList.toggle("all-changes-active", E), BE)
        BE.hidden = !E;
      if (E) {
        if (V.hidden = !0, c)
          c.hidden = !0;
        if (GE)
          GE.hidden = !0;
        if (m)
          m.hidden = !0;
        if (OE)
          BE.querySelector("h1")?.focus();
      } else
        jE("rendered");
      OE = !1, n();
    }
    if (window.addEventListener("hashchange", fE), fE(), m) {
      let E = function() {
        if (e.length === 0) {
          n();
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
        }), n();
      }, _ = function() {
        if (VE)
          return;
        VE = !0, window.requestAnimationFrame(function() {
          VE = !1, E();
        });
      };
      var e = Array.from(m.querySelectorAll('a[href^="#"]')), DE = m.querySelector(".toc-list");
      if (DE)
        if (X = q.createElement("dialog"), typeof X.showModal === "function") {
          X.className = "toc-dialog", X.id = "toc-dialog", X.setAttribute("aria-labelledby", "toc-dialog-title");
          var KE = q.createElement("div");
          KE.className = "toc-dialog-panel";
          var QE = q.createElement("div");
          QE.className = "toc-dialog-header";
          var YE = q.createElement("h2");
          YE.className = "toc-dialog-title", YE.id = "toc-dialog-title", YE.textContent = "Contents";
          var i = q.createElement("button");
          i.className = "toc-dialog-close", i.type = "button", i.setAttribute("aria-label", "Close table of contents"), i.innerHTML = rE, QE.appendChild(YE), QE.appendChild(i);
          var EE = q.createElement("nav");
          EE.className = "toc-dialog-nav", EE.setAttribute("aria-label", "Table of contents"), EE.appendChild(DE.cloneNode(!0)), KE.appendChild(QE), KE.appendChild(EE), X.appendChild(KE), N = q.createElement("button"), N.className = "toc-trigger", N.type = "button", N.tabIndex = -1, N.setAttribute("aria-label", "Open table of contents"), N.setAttribute("aria-controls", "toc-dialog"), N.setAttribute("aria-haspopup", "dialog"), N.setAttribute("aria-hidden", "true"), N.innerHTML = tE, q.body.appendChild(N), q.body.appendChild(X), q.body.classList.add("toc-dialog-ready"), N.addEventListener("click", function() {
            r(!1), X.showModal(), q.body.classList.add("toc-dialog-open"), n();
            var S = X.querySelector("a.active");
            if (S)
              S.scrollIntoView({ block: "nearest" });
          }), i.addEventListener("click", h), X.addEventListener("click", function(S) {
            if (S.target === X)
              h();
          }), X.addEventListener("keydown", function(S) {
            if (S.key === "Escape")
              S.preventDefault(), h();
          }), X.addEventListener("close", function() {
            if (q.body.classList.remove("toc-dialog-open"), n(), N.classList.contains("is-visible"))
              setTimeout(function() {
                N.focus();
              }, 50);
          }), EE.querySelectorAll("a").forEach(function(S) {
            S.addEventListener("click", h);
          });
        } else
          X = null;
      var FE = e.slice();
      if (X)
        FE = FE.concat(Array.from(X.querySelectorAll('a[href^="#"]')));
      var aE = e.map(function(S) {
        return q.getElementById((S.getAttribute("href") || "").slice(1));
      }), VE = !1;
      q.addEventListener("scroll", _, { passive: !0 }), window.addEventListener("resize", E), E();
    }
    var ZE = q.querySelector(".toolbar");
    function WE() {
      var E = ZE && window.matchMedia("(max-width: 48rem)").matches ? ZE.getBoundingClientRect().height : 0;
      q.documentElement.style.setProperty("--airplan-sticky-height", E + "px");
    }
    if (ZE) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(WE).observe(ZE);
      window.addEventListener("resize", WE), WE();
    }
    let ME = q.querySelector(".copy-source");
    if (ME && c)
      ME.addEventListener("click", function() {
        var E = c.querySelector("pre");
        UE(E ? E.textContent : "", ME);
      });
    V.querySelectorAll("pre").forEach(function(E) {
      if (E.classList.contains("mermaid"))
        return;
      var _ = q.createElement("div");
      _.className = "codewrap", E.parentNode?.insertBefore(_, E), _.appendChild(E);
      var S = q.createElement("button");
      S.className = "codecopy", S.type = "button", S.setAttribute("aria-label", "Copy code"), S.title = "Copy code", S.innerHTML = dE + sE + oE, S.addEventListener("click", function() {
        var G = E.querySelector("code");
        UE((G || E).textContent, S);
      }), _.appendChild(S);
    });
  })();
})();
