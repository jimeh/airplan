(() => {
  function eE(q) {
    return q === "system" || q === "light" || q === "dark";
  }
  function zE(q, $) {
    try {
      return q?.getItem($) ?? null;
    } catch {
      return null;
    }
  }
  function d(q, $, F) {
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
        d(F, "airplan-color-mode", V);
    }
    let j = eE(V) ? V : "system", C = new Set(q.themes.map((U) => U.id)), Z = zE(F, "airplan-light-theme"), M = zE(F, "airplan-dark-theme"), O = Z !== null && C.has(Z) ? Z : q.defaultLight, w = M !== null && C.has(M) ? M : q.defaultDark;
    return CE(q, j, O, w, $);
  }
  function CE(q, $, F, V, j) {
    let C = new Map(q.themes.map((h) => [h.id, h])), Z = C.has(F) ? F : q.defaultLight, M = C.has(V) ? V : q.defaultDark, O = $ === "system" ? j ? "dark" : "light" : $, w = O === "light" ? Z : M, U = C.get(w)?.variant ?? O;
    return { mode: $, resolvedMode: O, lightTheme: Z, darkTheme: M, theme: w, variant: U };
  }
  function hE(q, $) {
    if ($ === "system")
      d(q, "airplan-color-mode", null), d(q, "airplan-theme", null);
    else
      d(q, "airplan-color-mode", $), d(q, "airplan-theme", $);
  }
  function yE(q, $, F) {
    d(q, $ === "light" ? "airplan-light-theme" : "airplan-dark-theme", F);
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
    let V = F, j = window.matchMedia("(prefers-color-scheme: dark)"), C;
    try {
      C = window.localStorage;
    } catch {}
    let Z = window.__airplanThemeState ?? xE(V, j.matches, C), M = q.querySelector("[data-airplan-appearance-trigger]"), O = q.querySelector("[data-airplan-appearance-panel]"), w = q.querySelector('select[data-airplan-theme-slot="light"]'), U = q.querySelector('select[data-airplan-theme-slot="dark"]'), h = Array.from(q.querySelectorAll("[data-airplan-color-mode]"));
    function s(Y) {
      if (!Y || Y.options.length > 0)
        return;
      for (let [z, P] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let R = q.createElement("optgroup");
        R.label = P;
        for (let L of V.themes) {
          if (L.variant !== z)
            continue;
          let t = q.createElement("option");
          t.value = L.id, t.textContent = L.name, R.append(t);
        }
        if (R.children.length > 0)
          Y.append(R);
      }
    }
    s(w), s(U);
    function y(Y, z = !0) {
      if (Z = Y, window.__airplanThemeState = Z, $.dataset.airplanMode = Z.mode, $.dataset.airplanResolvedMode = Z.resolvedMode, $.dataset.airplanTheme = Z.theme, $.dataset.airplanThemeVariant = Z.variant, h.forEach((P) => {
        let R = P.dataset.airplanColorMode === Z.mode;
        P.classList.toggle("active", R), P.setAttribute("aria-pressed", String(R));
      }), w)
        w.value = Z.lightTheme;
      if (U)
        U.value = Z.darkTheme;
      if (z)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: PE(Z) }));
    }
    function k(Y = {}) {
      y(CE(V, Y.mode ?? Z.mode, Y.lightTheme ?? Z.lightTheme, Y.darkTheme ?? Z.darkTheme, j.matches));
    }
    function o(Y, z = !1) {
      if (!O || !M)
        return;
      if (O.hidden = !Y, M.setAttribute("aria-expanded", String(Y)), Y)
        O.querySelector("button,select")?.focus();
      else if (z)
        M.focus();
    }
    M?.addEventListener("click", () => o(Boolean(O?.hidden ?? !0))), h.forEach((Y) => Y.addEventListener("click", () => {
      let z = Y.dataset.airplanColorMode;
      if (!z)
        return;
      hE(C, z), k({ mode: z });
    }));
    function qE(Y, z) {
      yE(C, Y, z.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: z.value } })), k(Y === "light" ? { lightTheme: z.value } : { darkTheme: z.value });
    }
    w?.addEventListener("change", () => qE("light", w)), U?.addEventListener("change", () => qE("dark", U)), j.addEventListener("change", () => {
      if (Z.mode === "system")
        k();
    }), q.addEventListener("keydown", (Y) => {
      if (Y.key === "Escape" && O && !O.hidden)
        Y.preventDefault(), o(!1, !0);
    }), q.addEventListener("pointerdown", (Y) => {
      if (!O || O.hidden || !M)
        return;
      let z = Y.target;
      if (!(z instanceof Node) || O.contains(z) || M.contains(z))
        return;
      let R = (z instanceof Element ? z : z.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), L = O.contains(q.activeElement) && !R;
      if (o(!1), L)
        setTimeout(() => {
          if (q.activeElement === q.body || O.contains(q.activeElement))
            M.focus();
        });
    }), y(Z, !1);
  })();

  (function() {
    var q = document, $ = 262144;
    let F = q.getElementById("rendered");
    if (!F)
      return;
    let V = F;
    var j = q.querySelector('meta[name="airplan-versions"]'), C = q.querySelector('meta[name="airplan-revision-chain"]'), Z = q.querySelector('meta[name="airplan-page-path"]'), M = q.querySelector('meta[name="airplan-entrypoint"]'), O = j ? new URL(j.content, window.location.href) : null, w = O ? new URL("./", O) : null, U = w ? w.pathname.split("/").filter(Boolean) : [], h = U.slice(0, -1);
    function s(E, _) {
      if (typeof E !== "string")
        return null;
      try {
        var S = new URL(E);
        if (S.origin !== window.location.origin || S.username || S.password || S.search || S.hash)
          return null;
        var G = S.pathname.split("/").filter(Boolean);
        if (G.length !== h.length + 2 || !h.every(function(Q, J) {
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
    function y(E) {
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
    function k(E, _) {
      if (!y(_))
        return null;
      var S = String(_).split("/").map(function(A) {
        return encodeURIComponent(A);
      }).join("/"), G = new URL(S, E);
      if (G.origin !== E.origin || G.username || G.password || G.search || G.hash || !G.pathname.startsWith(E.pathname))
        return null;
      return G.href;
    }
    async function o(E) {
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
      var J = new Uint8Array(A), I = 0;
      G.forEach(function(K) {
        J.set(K, I), I += K.byteLength;
      });
      var H = new TextDecoder("utf-8", { fatal: !0 }).decode(J);
      return qE(H), JSON.parse(H);
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
            var I = E[_++];
            if (I === "}")
              return;
            if (I !== ",")
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
            var I = E[_++];
            if (I === "]")
              return;
            if (I !== ",")
              throw Error("JSON array is invalid");
          }
        }
        if (E[_] === '"') {
          G();
          return;
        }
        var H = E.slice(_).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
        if (!H)
          throw Error("JSON value is invalid");
        _ += H[0].length;
      }
      if (A(), S(), _ !== E.length)
        throw Error("JSON has trailing content");
    }
    function Y(E, _, S, G) {
      if (!L(E))
        throw Error("marker is invalid");
      var A = E, Q = _.pathname.split("/").filter(Boolean), J = Q[Q.length - 1] || "";
      if (A.schema !== "airplan-upload" || A.version !== 6 || A.kind !== "document" || A.directory !== J || !/^[a-z2-7]{26}$/.test(A.directory) || !uE(A.created_at) || A.format !== "md" || !t(A.slug) || A.entrypoint !== A.slug + ".html" || !L(A.producer) || A.producer.name !== "airplan" || typeof A.producer.version !== "string" || A.producer.version.trim() !== A.producer.version || A.producer.version === "" || !P(A.render) || !L(A.revision) || A.revision.number !== S.number || A.revision.chain_id !== G || (A.revision.number === 1 ? A.revision.previous_url !== void 0 : typeof A.revision.previous_url !== "string" || !kE(A.revision.previous_url)) || !Array.isArray(A.objects) || !Array.isArray(A.pages) || A.pages.length === 0)
        throw Error("marker identity is invalid");
      var I = k(_, A.entrypoint);
      if (I !== S.safeURL)
        throw Error("marker entrypoint is invalid");
      if (A.title !== void 0 && typeof A.title !== "string" || A.repo !== void 0 && !mE(A.repo) || A.objects.length === 0 || A.pages.length > 100)
        throw Error("marker shape is invalid");
      var H = nE(A), K = new Set, i = new Set, n = new Set, _E = new Map;
      A.pages.forEach(function(B, T) {
        if (!L(B) || !y(B.path) || K.has(B.path) || i.has(B.path.toLowerCase()) || B.format !== "md" && B.format !== "txt" || typeof B.lang !== "string" || B.title !== void 0 && typeof B.title !== "string" || !y(B.page) || !y(B.source))
          throw Error("marker page descriptor is invalid");
        var b = z(B.path, B.format), $E = B.path;
        if (T === 0) {
          if (b = A.entrypoint, $E = A.slug + ".md", B.format !== A.format)
            throw Error("marker entry format is invalid");
        }
        if (B.page !== b || B.source !== $E)
          throw Error("marker generated page mapping is invalid");
        var l = k(_, B.page);
        if (!l || n.has(l))
          throw Error("marker page object is invalid");
        if (!k(_, B.source))
          throw Error("marker source object is invalid");
        if (H.get(B.page) !== "page" || H.get(B.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var SE = A.objects.find(function(NE) {
          return NE.name === B.source;
        }).content_type;
        if (B.format === "md" && SE !== "text/markdown; charset=utf-8" || B.format === "txt" && SE !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        K.add(B.path), i.add(B.path.toLowerCase()), n.add(l), _E.set(B.path, l);
      });
      var m = Array.from(i).sort();
      for (var D = 1;D < m.length; D += 1)
        if (m[D].startsWith(m[D - 1] + "/"))
          throw Error("marker page paths conflict");
      if (!K.has(A.pages[0].path) || _E.get(A.pages[0].path) !== I)
        throw Error("marker entry page is invalid");
      if (n.size !== A.pages.length || Array.from(H.values()).filter(function(B) {
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
    function P(E) {
      if (!L(E) || !L(E.template) || !L(E.themes) || !Number.isInteger(E.generation) || E.generation <= 0 || typeof E.indexable !== "boolean" || typeof E.no_external_assets !== "boolean" || !E.template || E.template.kind !== "builtin" && E.template.kind !== "custom" || E.mermaid_url !== void 0 && !vE(E.mermaid_url) || !E.themes)
        return !1;
      if (E.template.kind === "builtin" && E.template.sha256 !== void 0 || E.template.kind === "custom" && !R(E.template.sha256))
        return !1;
      return /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(E.themes.default_light) && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(E.themes.default_dark) && R(E.themes.catalog_sha256);
    }
    function R(E) {
      return typeof E === "string" && /^[0-9a-f]{64}$/.test(E);
    }
    function L(E) {
      return !!E && typeof E === "object" && !Array.isArray(E);
    }
    function t(E) {
      return typeof E === "string" && E.length <= 64 && /^[a-z0-9-]+$/.test(E);
    }
    function uE(E) {
      if (typeof E !== "string")
        return !1;
      var _ = E.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/);
      if (!_)
        return !1;
      var S = Number(_[1]), G = Number(_[2]), A = Number(_[3]), Q = Number(_[4]), J = Number(_[5]), I = Number(_[6]), H = S % 4 === 0 && (S % 100 !== 0 || S % 400 === 0), K = [31, H ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
      return G >= 1 && G <= 12 && A >= 1 && A <= K[G - 1] && Q <= 23 && J <= 59 && I <= 59;
    }
    function mE(E) {
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
    function bE(E) {
      return typeof E === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(E);
    }
    function kE(E) {
      try {
        var _ = new URL(E);
        return (_.protocol === "https:" || _.protocol === "http:") && !_.username && !_.password && !_.search && !_.hash && _.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function vE(E) {
      if (typeof E !== "string")
        return !1;
      try {
        var _ = new URL(E);
        return _.protocol === "https:" && !!_.host && !_.username && !_.password && !_.hash;
      } catch {
        return !1;
      }
    }
    function nE(E) {
      var _ = new Map, S = new Set, G = 0, A = 0, Q = 0, J = 0;
      E.objects.forEach(function(K) {
        if (!L(K) || !y(K.name) && K.name !== ".airplan-changes.diff" || _.has(K.name) || S.has(K.name.toLowerCase()) || !Number.isSafeInteger(K.bytes) || K.bytes < 0 || !R(K.sha256) || !bE(K.content_type))
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
      var I = Array.from(S).sort();
      for (var H = 1;H < I.length; H += 1)
        if (I[H].startsWith(I[H - 1] + "/"))
          throw Error("marker object paths conflict");
      if (A !== E.pages.length || Q !== E.pages.length || A + J > 100 || (E.revision.number === 1 ? G !== 0 : G !== 1))
        throw Error("marker object counts are invalid");
      return _;
    }
    function lE(E, _) {
      var S = window.location.hash;
      if (S === "#airplan-all-changes")
        return E + S;
      if (!_)
        return E;
      return _ + (S && S !== "#airplan-all-changes" ? S : "");
    }
    function gE(E) {
      var _ = q.querySelector('meta[name="airplan-revision"]'), S = _ ? Number(_.content) : Number(E.current_revision);
      if (!Number.isInteger(S) || S <= 0 || E.current_revision !== S || !Number.isInteger(E.latest_revision) || !Number.isInteger(E.last_assigned_revision) || !Array.isArray(E.revisions) || E.revisions.length === 0 || E.last_assigned_revision !== E.revisions.length || !/^[a-z2-7]{26}$/.test(E.chain_id) || C && C.content !== E.chain_id)
        throw Error("revision identity is invalid");
      var G = !1, A = 0, Q = E.revisions.filter(function(B) {
        if (!B || !Number.isInteger(B.number) || B.number !== A + 1)
          return G = !0, !1;
        if (A = B.number, B.deleted)
          return !1;
        if (B.safeURL = s(B.url, !1), !B.safeURL)
          return G = !0, !1;
        if (B.number > 1) {
          var T = s(B.diff_url, !0);
          if (!T || new URL(T).pathname.replace(/[^/]+$/, "") !== new URL(B.safeURL).pathname.replace(/[^/]+$/, ""))
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
      }), I = new URL(window.location.href);
      if (I.search = "", I.hash = "", !J || !w || new URL(J.safeURL || "").pathname.replace(/[^/]+$/, "") !== w.pathname || !I.pathname.startsWith(w.pathname))
        throw Error("current revision URL is invalid");
      var H = Math.max.apply(null, Q.map(function(B) {
        return B.number;
      }));
      if (H !== E.latest_revision)
        throw Error("latest is invalid");
      var K = q.querySelector("[data-revision-heading]");
      if (!K) {
        K = q.createElement("p"), K.className = "revision-heading", K.setAttribute("data-revision-heading", "");
        var i = q.getElementById("rendered");
        if (!i)
          throw Error("rendered view is unavailable");
        i.prepend(K);
      }
      var n = S < H, _E = n ? "Revision " + S + " of " + H : "Revision " + S + " (Latest)", m = q.createElement("span");
      m.className = "revision-picker-label", m.textContent = _E, m.setAttribute("aria-hidden", "true");
      var D = q.createElement("select");
      D.setAttribute("aria-label", "Document revision"), Q.forEach(function(B) {
        var T = q.createElement("option");
        T.value = B.safeURL || "", T.textContent = B.number === H ? "Revision " + B.number + " (Latest)" : "Revision " + B.number + " of " + H, T.selected = B.number === S, D.appendChild(T);
      }), D.addEventListener("change", function() {
        var B = D.selectedIndex;
        if (B < 0 || B >= Q.length)
          return;
        var T = Q[B], b = T.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(b + (T.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var $E = M ? new URL(M.content, window.location.href).href : "";
        if (!Z || I.href === $E || !C) {
          window.location.assign(b);
          return;
        }
        K.setAttribute("aria-busy", "true"), D.disabled = !0;
        var l = new URL("./", b), SE = new URL(".airplan.json", l);
        SE.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(SE, { cache: "no-store", credentials: "same-origin" }).then(o).then(function(NE) {
          var aE = Y(NE, l, T, C.content);
          window.location.assign(lE(b, aE.get(Z.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(b);
        });
      }), K.replaceChildren(m, D), K.classList.add("is-picker"), K.classList.toggle("is-stale", n), q.body.classList.toggle("airplan-stale-revision", n);
    }
    if (j) {
      var wE = new URL(j.content, window.location.href);
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
        gE(E), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: E
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var AE = q.createElement("div");
    AE.className = "sr-status", AE.setAttribute("aria-live", "polite"), q.body.appendChild(AE);
    var g = null;
    function pE() {
      if (g !== null)
        return;
      g = Array.from(q.querySelectorAll("details:not([open])")), g.forEach(function(E) {
        E.open = !0;
      });
    }
    function cE() {
      if (g === null)
        return;
      g.forEach(function(E) {
        E.open = !1;
      }), g = null;
    }
    window.addEventListener("beforeprint", pE), window.addEventListener("afterprint", cE);
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
    var iE = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', dE = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', sE = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', oE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', tE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', LE = q.getElementById("pages"), f = q.querySelector(".pages-trigger"), W = null, XE = window.matchMedia("(max-width: 78rem)"), x = function() {};
    function IE() {
      return W ? W.matches(":popover-open") : !1;
    }
    function r(E) {
      if (!W || !IE())
        return;
      if (W.hidePopover(), E && f && XE.matches)
        setTimeout(function() {
          f.focus();
        }, 0);
    }
    if (LE && f) {
      var RE = LE.querySelector(".pages-list");
      if (RE) {
        var HE = q.createElement("div");
        if ("popover" in HE && typeof HE.showPopover === "function") {
          let E = function() {
            if (!f || !W)
              return;
            var _ = f.getBoundingClientRect();
            W.style.setProperty("--pages-left", Math.max(16, _.left) + "px"), W.style.setProperty("--pages-top", _.bottom + "px"), W.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, _.left) - 16) + "px");
          };
          W = HE, W.className = "pages-popover", W.id = "pages-popover", W.setAttribute("popover", "auto");
          var a = q.createElement("nav");
          a.className = "pages-popover-nav", a.setAttribute("aria-label", "Pages"), a.appendChild(RE.cloneNode(!0)), W.appendChild(a), f.setAttribute("popovertarget", W.id), f.popoverTargetElement = W, W.addEventListener("beforetoggle", function(_) {
            if (_.newState !== "open")
              return;
            x(), E();
          }), W.addEventListener("toggle", function(_) {
            var S = _.newState === "open";
            if (f.setAttribute("aria-expanded", S ? "true" : "false"), q.body.classList.toggle("pages-popover-open", S), S) {
              var G = W.querySelector('[aria-current="page"]');
              if (G)
                G.scrollIntoView({ block: "nearest" });
            }
            v();
          }), a.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", function() {
              r(!1);
            });
          }), XE.addEventListener("change", function() {
            if (!XE.matches)
              r(!1);
          }), window.addEventListener("resize", function() {
            if (IE())
              E();
          }), f.hidden = !1, f.setAttribute("aria-expanded", "false"), q.body.appendChild(W), q.body.classList.add("pages-popover-ready");
        }
      }
    }
    var p = q.getElementById("source"), GE = q.getElementById("changes"), BE = q.querySelector("[data-airplan-all-changes]"), u = q.getElementById("toc"), N = null, X = null, TE = window.matchMedia("(max-width: 78rem)");
    x = function() {
      if (X && X.open)
        X.close();
    };
    function v() {
      if (!u || !N || !X)
        return;
      var E = TE.matches && !V.hidden && !X.open && !IE();
      if (N.classList.toggle("is-visible", E), N.tabIndex = E ? 0 : -1, N.setAttribute("aria-hidden", E ? "false" : "true"), X.open && (!TE.matches || V.hidden))
        x();
    }
    function jE(E) {
      if (r(!1), x(), V.hidden = E !== "rendered", p)
        p.hidden = E !== "source";
      if (GE)
        GE.hidden = E !== "changes";
      if (u)
        u.hidden = E !== "rendered";
      q.querySelectorAll(".viewtoggle button").forEach(function(_) {
        var S = _.dataset.view === E;
        _.classList.toggle("active", S), _.setAttribute("aria-pressed", S ? "true" : "false");
      }), v();
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
    function DE() {
      var E = window.location.hash === "#airplan-all-changes" && !!BE;
      if (r(!1), x(), q.body.classList.toggle("all-changes-active", E), BE)
        BE.hidden = !E;
      if (E) {
        if (V.hidden = !0, p)
          p.hidden = !0;
        if (GE)
          GE.hidden = !0;
        if (u)
          u.hidden = !0;
        if (OE)
          BE.querySelector("h1")?.focus();
      } else
        jE("rendered");
      OE = !1, v();
    }
    if (window.addEventListener("hashchange", DE), DE(), u) {
      let E = function() {
        if (e.length === 0) {
          v();
          return;
        }
        var S = 0;
        if (rE.forEach(function(A, Q) {
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
        }), v();
      }, _ = function() {
        if (VE)
          return;
        VE = !0, window.requestAnimationFrame(function() {
          VE = !1, E();
        });
      };
      var e = Array.from(u.querySelectorAll('a[href^="#"]')), fE = u.querySelector(".toc-list");
      if (fE)
        if (X = q.createElement("dialog"), typeof X.showModal === "function") {
          X.className = "toc-dialog", X.id = "toc-dialog", X.setAttribute("aria-labelledby", "toc-dialog-title");
          var KE = q.createElement("div");
          KE.className = "toc-dialog-panel";
          var QE = q.createElement("div");
          QE.className = "toc-dialog-header";
          var YE = q.createElement("h2");
          YE.className = "toc-dialog-title", YE.id = "toc-dialog-title", YE.textContent = "Contents";
          var c = q.createElement("button");
          c.className = "toc-dialog-close", c.type = "button", c.setAttribute("aria-label", "Close table of contents"), c.innerHTML = tE, QE.appendChild(YE), QE.appendChild(c);
          var EE = q.createElement("nav");
          EE.className = "toc-dialog-nav", EE.setAttribute("aria-label", "Table of contents"), EE.appendChild(fE.cloneNode(!0)), KE.appendChild(QE), KE.appendChild(EE), X.appendChild(KE), N = q.createElement("button"), N.className = "toc-trigger", N.type = "button", N.tabIndex = -1, N.setAttribute("aria-label", "Open table of contents"), N.setAttribute("aria-controls", "toc-dialog"), N.setAttribute("aria-haspopup", "dialog"), N.setAttribute("aria-hidden", "true"), N.innerHTML = oE, q.body.appendChild(N), q.body.appendChild(X), q.body.classList.add("toc-dialog-ready"), N.addEventListener("click", function() {
            r(!1), X.showModal(), q.body.classList.add("toc-dialog-open"), v();
            var S = X.querySelector("a.active");
            if (S)
              S.scrollIntoView({ block: "nearest" });
          }), c.addEventListener("click", x), X.addEventListener("click", function(S) {
            if (S.target === X)
              x();
          }), X.addEventListener("keydown", function(S) {
            if (S.key === "Escape")
              S.preventDefault(), x();
          }), X.addEventListener("close", function() {
            if (q.body.classList.remove("toc-dialog-open"), v(), N.classList.contains("is-visible"))
              setTimeout(function() {
                N.focus();
              }, 50);
          }), EE.querySelectorAll("a").forEach(function(S) {
            S.addEventListener("click", x);
          });
        } else
          X = null;
      var FE = e.slice();
      if (X)
        FE = FE.concat(Array.from(X.querySelectorAll('a[href^="#"]')));
      var rE = e.map(function(S) {
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
    if (ME && p)
      ME.addEventListener("click", function() {
        var E = p.querySelector("pre");
        UE(E ? E.textContent : "", ME);
      });
    V.querySelectorAll("pre").forEach(function(E) {
      if (E.classList.contains("mermaid"))
        return;
      var _ = q.createElement("div");
      _.className = "codewrap", E.parentNode?.insertBefore(_, E), _.appendChild(E);
      var S = q.createElement("button");
      S.className = "codecopy", S.type = "button", S.setAttribute("aria-label", "Copy code"), S.title = "Copy code", S.innerHTML = iE + dE + sE, S.addEventListener("click", function() {
        var G = E.querySelector("code");
        UE((G || E).textContent, S);
      }), _.appendChild(S);
    });
  })();
})();
