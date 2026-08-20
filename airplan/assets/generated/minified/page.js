(() => {
  function oE(S) {
    return S === "system" || S === "light" || S === "dark";
  }
  function zE(S, $) {
    try {
      return S?.getItem($) ?? null;
    } catch {
      return null;
    }
  }
  function d(S, $, H) {
    try {
      if (H === null)
        S?.removeItem($);
      else
        S?.setItem($, H);
    } catch {}
  }
  function DE(S, $, H) {
    let W = zE(H, "airplan-color-mode");
    if (W === null) {
      let w = zE(H, "airplan-theme");
      if (W = w === "light" || w === "dark" ? w : "system", W !== "system")
        d(H, "airplan-color-mode", W);
    }
    let L = oE(W) ? W : "system", M = new Set(S.themes.map((w) => w.id)), Y = zE(H, "airplan-light-theme"), K = zE(H, "airplan-dark-theme"), I = Y !== null && M.has(Y) ? Y : S.defaultLight, N = K !== null && M.has(K) ? K : S.defaultDark;
    return NE(S, L, I, N, $);
  }
  function NE(S, $, H, W, L) {
    let M = new Map(S.themes.map((u) => [u.id, u])), Y = M.has(H) ? H : S.defaultLight, K = M.has(W) ? W : S.defaultDark, I = $ === "system" ? L ? "dark" : "light" : $, N = I === "light" ? Y : K, w = M.get(N)?.variant ?? I;
    return { mode: $, resolvedMode: I, lightTheme: Y, darkTheme: K, theme: N, variant: w };
  }
  function jE(S, $) {
    if ($ === "system")
      d(S, "airplan-color-mode", null), d(S, "airplan-theme", null);
    else
      d(S, "airplan-color-mode", $), d(S, "airplan-theme", $);
  }
  function uE(S, $, H) {
    d(S, $ === "light" ? "airplan-light-theme" : "airplan-dark-theme", H);
  }
  function yE(S) {
    return {
      mode: S.mode,
      resolvedMode: S.resolvedMode,
      theme: S.theme,
      variant: S.variant
    };
  }

  (function() {
    let S = document, $ = S.documentElement;
    S.querySelectorAll(".js-only").forEach((Q) => {
      Q.hidden = !1;
    });
    let H = window.__AIRPLAN_THEME_CATALOG__;
    if (!H)
      return;
    let W = H, L = window.matchMedia("(prefers-color-scheme: dark)"), M;
    try {
      M = window.localStorage;
    } catch {}
    let Y = window.__airplanThemeState ?? DE(W, L.matches, M), K = S.querySelector("[data-airplan-appearance-trigger]"), I = S.querySelector("[data-airplan-appearance-panel]"), N = S.querySelector('select[data-airplan-theme-slot="light"]'), w = S.querySelector('select[data-airplan-theme-slot="dark"]'), u = Array.from(S.querySelectorAll("[data-airplan-color-mode]"));
    function s(Q) {
      if (!Q || Q.options.length > 0)
        return;
      for (let [z, h] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let F = S.createElement("optgroup");
        F.label = h;
        for (let x of W.themes) {
          if (x.variant !== z)
            continue;
          let o = S.createElement("option");
          o.value = x.id, o.textContent = x.name, F.append(o);
        }
        if (F.children.length > 0)
          Q.append(F);
      }
    }
    s(N), s(w);
    function y(Q, z = !0) {
      if (Y = Q, window.__airplanThemeState = Y, $.dataset.airplanMode = Y.mode, $.dataset.airplanResolvedMode = Y.resolvedMode, $.dataset.airplanTheme = Y.theme, $.dataset.airplanThemeVariant = Y.variant, u.forEach((h) => {
        let F = h.dataset.airplanColorMode === Y.mode;
        h.classList.toggle("active", F), h.setAttribute("aria-pressed", String(F));
      }), N)
        N.value = Y.lightTheme;
      if (w)
        w.value = Y.darkTheme;
      if (z)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: yE(Y) }));
    }
    function k(Q = {}) {
      y(NE(W, Q.mode ?? Y.mode, Q.lightTheme ?? Y.lightTheme, Q.darkTheme ?? Y.darkTheme, L.matches));
    }
    function t(Q, z = !1) {
      if (!I || !K)
        return;
      if (I.hidden = !Q, K.setAttribute("aria-expanded", String(Q)), Q)
        I.querySelector("button,select")?.focus();
      else if (z)
        K.focus();
    }
    K?.addEventListener("click", () => t(Boolean(I?.hidden ?? !0))), u.forEach((Q) => Q.addEventListener("click", () => {
      let z = Q.dataset.airplanColorMode;
      if (!z)
        return;
      jE(M, z), k({ mode: z });
    }));
    function qE(Q, z) {
      uE(M, Q, z.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: z.value } })), k(Q === "light" ? { lightTheme: z.value } : { darkTheme: z.value });
    }
    N?.addEventListener("change", () => qE("light", N)), w?.addEventListener("change", () => qE("dark", w)), L.addEventListener("change", () => {
      if (Y.mode === "system")
        k();
    }), S.addEventListener("keydown", (Q) => {
      if (Q.key === "Escape" && I && !I.hidden)
        Q.preventDefault(), t(!1, !0);
    }), S.addEventListener("pointerdown", (Q) => {
      if (!I || I.hidden || !K)
        return;
      let z = Q.target;
      if (!(z instanceof Node) || I.contains(z) || K.contains(z))
        return;
      let F = (z instanceof Element ? z : z.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), x = I.contains(S.activeElement) && !F;
      if (t(!1), x)
        setTimeout(() => {
          if (S.activeElement === S.body || I.contains(S.activeElement))
            K.focus();
        });
    }), y(Y, !1);
  })();

  (function() {
    var S = document, $ = 262144;
    let H = S.getElementById("rendered");
    if (!H)
      return;
    let W = H;
    var L = S.querySelector('meta[name="airplan-versions"]'), M = S.querySelector('meta[name="airplan-revision-chain"]'), Y = S.querySelector('meta[name="airplan-page-path"]'), K = S.querySelector('meta[name="airplan-entrypoint"]'), I = L ? new URL(L.content, window.location.href) : null, N = I ? new URL("./", I) : null, w = N ? N.pathname.split("/").filter(Boolean) : [], u = w.slice(0, -1);
    function s(E, A) {
      if (typeof E !== "string")
        return null;
      try {
        var _ = new URL(E);
        if (_.origin !== window.location.origin || _.username || _.password || _.search || _.hash)
          return null;
        var B = _.pathname.split("/").filter(Boolean);
        if (B.length !== u.length + 2 || !u.every(function(Z, C) {
          return B[C] === Z;
        }) || !/^[a-z2-7]{26}$/.test(B[B.length - 2]))
          return null;
        var q = B[B.length - 1];
        if (A ? q !== ".airplan-changes.diff" : !q.endsWith(".html"))
          return null;
        return _.href;
      } catch {
        return null;
      }
    }
    function y(E) {
      if (typeof E !== "string" || E === "" || E.startsWith("/") || E.includes("\\"))
        return !1;
      var A = E.split("/");
      return A.every(function(_) {
        var B = _.toLowerCase(), q = Array.from(_).some(function(Z) {
          var C = Z.codePointAt(0) || 0;
          return C < 32 || C === 127;
        });
        if (!_ || _ === "." || _ === ".." || B.startsWith(".airplan-") || B === ".airplan.json" || q || /[. ]$/.test(_) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(_))
          return !1;
        return !0;
      });
    }
    function k(E, A) {
      if (!y(A))
        return null;
      var _ = String(A).split("/").map(function(q) {
        return encodeURIComponent(q);
      }).join("/"), B = new URL(_, E);
      if (B.origin !== E.origin || B.username || B.password || B.search || B.hash || !B.pathname.startsWith(E.pathname))
        return null;
      return B.href;
    }
    async function t(E) {
      if (!E.ok)
        throw Error("marker request failed");
      var A = E.headers.get("content-length");
      if (A && /^\d+$/.test(A) && Number(A) > $) {
        if (E.body)
          await E.body.cancel("marker is too large");
        throw Error("marker is too large");
      }
      if (!E.body || typeof E.body.getReader !== "function")
        throw Error("bounded marker stream is unavailable");
      var _ = E.body.getReader(), B = [], q = 0;
      try {
        for (;; ) {
          var Z = await _.read();
          if (Z.done)
            break;
          if (q += Z.value.byteLength, q > $)
            throw await _.cancel("marker is too large"), Error("marker is too large");
          B.push(Z.value);
        }
      } finally {
        _.releaseLock();
      }
      var C = new Uint8Array(q), U = 0;
      return B.forEach(function(O) {
        C.set(O, U), U += O.byteLength;
      }), JSON.parse(new TextDecoder("utf-8", { fatal: !0 }).decode(C));
    }
    function qE(E, A, _, B) {
      if (!F(E) || !x(E, [
        "schema",
        "version",
        "directory",
        "created_at",
        "kind",
        "slug",
        "format",
        "objects",
        "title",
        "repo",
        "producer",
        "render",
        "revision",
        "entrypoint",
        "pages"
      ]))
        throw Error("marker is invalid");
      var q = E, Z = A.pathname.split("/").filter(Boolean), C = Z[Z.length - 1] || "";
      if (q.schema !== "airplan-upload" || q.version !== 6 || q.kind !== "document" || q.directory !== C || !/^[a-z2-7]{26}$/.test(q.directory) || typeof q.created_at !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z$/.test(q.created_at) || Number.isNaN(Date.parse(q.created_at)) || q.format !== "md" || typeof q.slug !== "string" || q.slug === "" || q.entrypoint !== q.slug + ".html" || !F(q.producer) || !x(q.producer, ["name", "version"]) || q.producer.name !== "airplan" || typeof q.producer.version !== "string" || q.producer.version.trim() !== q.producer.version || q.producer.version === "" || !z(q.render) || !F(q.revision) || !x(q.revision, ["chain_id", "number", "previous_url"]) || q.revision.number !== _.number || q.revision.chain_id !== B || (q.revision.number === 1 ? q.revision.previous_url !== void 0 : typeof q.revision.previous_url !== "string" || !PE(q.revision.previous_url)) || !Array.isArray(q.objects) || !Array.isArray(q.pages) || q.pages.length === 0)
        throw Error("marker identity is invalid");
      var U = k(A, q.entrypoint);
      if (U !== _.safeURL)
        throw Error("marker entrypoint is invalid");
      if (q.title !== void 0 && typeof q.title !== "string" || q.repo !== void 0 && typeof q.repo !== "string" || q.objects.length === 0 || q.pages.length > 100)
        throw Error("marker shape is invalid");
      var O = bE(q), J = new Set, i = new Set, n = new Set, SE = new Map;
      q.pages.forEach(function(G, R) {
        if (!F(G) || !x(G, ["path", "page", "source", "format", "title", "lang"]) || !y(G.path) || J.has(G.path) || i.has(G.path.toLowerCase()) || G.format !== "md" && G.format !== "txt" || typeof G.lang !== "string" || G.title !== void 0 && typeof G.title !== "string" || !y(G.page) || !y(G.source))
          throw Error("marker page descriptor is invalid");
        var b = Q(G.path, G.format), $E = G.path;
        if (R === 0) {
          if (b = q.entrypoint, $E = q.slug + ".md", G.format !== q.format)
            throw Error("marker entry format is invalid");
        }
        if (G.page !== b || G.source !== $E)
          throw Error("marker generated page mapping is invalid");
        var l = k(A, G.page);
        if (!l || n.has(l))
          throw Error("marker page object is invalid");
        if (!k(A, G.source))
          throw Error("marker source object is invalid");
        if (O.get(G.page) !== "page" || O.get(G.source) !== "source")
          throw Error("marker page object relationship is invalid");
        var _E = q.objects.find(function(ME) {
          return ME.name === G.source;
        }).content_type;
        if (G.format === "md" && _E !== "text/markdown; charset=utf-8" || G.format === "txt" && _E !== "text/plain; charset=utf-8")
          throw Error("marker source content type is invalid");
        J.add(G.path), i.add(G.path.toLowerCase()), n.add(l), SE.set(G.path, l);
      });
      var m = Array.from(i).sort();
      for (var T = 1;T < m.length; T += 1)
        if (m[T].startsWith(m[T - 1] + "/"))
          throw Error("marker page paths conflict");
      if (!J.has(q.pages[0].path) || SE.get(q.pages[0].path) !== U)
        throw Error("marker entry page is invalid");
      if (n.size !== q.pages.length || Array.from(O.values()).filter(function(G) {
        return G === "source";
      }).length !== q.pages.length)
        throw Error("marker page inventory is invalid");
      return SE;
    }
    function Q(E, A) {
      if (A !== "md")
        return E + ".html";
      var _ = E.lastIndexOf("/"), B = E.lastIndexOf(".");
      return (B > _ ? E.slice(0, B) : E) + ".html";
    }
    function z(E) {
      if (!F(E) || !x(E, [
        "generation",
        "template",
        "indexable",
        "no_external_assets",
        "mermaid_url",
        "themes"
      ]) || !F(E.template) || !x(E.template, ["kind", "sha256"]) || !F(E.themes) || !x(E.themes, ["default_light", "default_dark", "catalog_sha256"]) || !Number.isInteger(E.generation) || E.generation <= 0 || typeof E.indexable !== "boolean" || typeof E.no_external_assets !== "boolean" || !E.template || E.template.kind !== "builtin" && E.template.kind !== "custom" || E.mermaid_url !== void 0 && !mE(E.mermaid_url) || !E.themes)
        return !1;
      if (E.template.kind === "builtin" && E.template.sha256 !== void 0 || E.template.kind === "custom" && !h(E.template.sha256))
        return !1;
      return /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(E.themes.default_light) && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(E.themes.default_dark) && h(E.themes.catalog_sha256);
    }
    function h(E) {
      return typeof E === "string" && /^[0-9a-f]{64}$/.test(E);
    }
    function F(E) {
      return !!E && typeof E === "object" && !Array.isArray(E);
    }
    function x(E, A) {
      return Object.keys(E).every(function(_) {
        return A.includes(_);
      });
    }
    function o(E) {
      return typeof E === "string" && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(E);
    }
    function PE(E) {
      try {
        var A = new URL(E);
        return (A.protocol === "https:" || A.protocol === "http:") && !A.username && !A.password && !A.search && !A.hash && A.pathname.endsWith(".html");
      } catch {
        return !1;
      }
    }
    function mE(E) {
      if (typeof E !== "string")
        return !1;
      try {
        var A = new URL(E);
        return A.protocol === "https:" && !!A.host && !A.username && !A.password && !A.hash;
      } catch {
        return !1;
      }
    }
    function bE(E) {
      var A = new Map, _ = new Set, B = 0, q = 0, Z = 0, C = 0;
      E.objects.forEach(function(J) {
        if (!F(J) || !x(J, ["name", "role", "bytes", "content_type", "sha256"]) || !y(J.name) && J.name !== ".airplan-changes.diff" || A.has(J.name) || _.has(J.name.toLowerCase()) || !Number.isSafeInteger(J.bytes) || J.bytes < 0 || !h(J.sha256) || !o(J.content_type))
          throw Error("marker object inventory is invalid");
        if (J.role === "page") {
          if (q += 1, J.bytes <= 0 || J.content_type !== "text/html; charset=utf-8")
            throw Error("marker page object is invalid");
        } else if (J.role === "source") {
          if (Z += 1, J.bytes <= 0)
            throw Error("marker source object is invalid");
        } else if (J.role === "asset")
          C += 1;
        else if (J.role === "diff") {
          if (B += 1, J.name !== ".airplan-changes.diff" || J.bytes <= 0 || J.content_type !== "text/plain; charset=utf-8")
            throw Error("marker diff object is invalid");
        } else
          throw Error("marker object role is invalid");
        A.set(J.name, J.role), _.add(J.name.toLowerCase());
      });
      var U = Array.from(_).sort();
      for (var O = 1;O < U.length; O += 1)
        if (U[O].startsWith(U[O - 1] + "/"))
          throw Error("marker object paths conflict");
      if (q !== E.pages.length || Z !== E.pages.length || q + C > 100 || (E.revision.number === 1 ? B !== 0 : B !== 1))
        throw Error("marker object counts are invalid");
      return A;
    }
    function kE(E, A) {
      var _ = window.location.hash;
      if (_ === "#airplan-all-changes")
        return E + _;
      if (!A)
        return E;
      return A + (_ && _ !== "#airplan-all-changes" ? _ : "");
    }
    function vE(E) {
      var A = S.querySelector('meta[name="airplan-revision"]'), _ = A ? Number(A.content) : Number(E.current_revision);
      if (!Number.isInteger(_) || _ <= 0 || E.current_revision !== _ || !Number.isInteger(E.latest_revision) || !Number.isInteger(E.last_assigned_revision) || !Array.isArray(E.revisions) || E.revisions.length === 0 || E.last_assigned_revision !== E.revisions.length || !/^[a-z2-7]{26}$/.test(E.chain_id) || M && M.content !== E.chain_id)
        throw Error("revision identity is invalid");
      var B = !1, q = 0, Z = E.revisions.filter(function(G) {
        if (!G || !Number.isInteger(G.number) || G.number !== q + 1)
          return B = !0, !1;
        if (q = G.number, G.deleted)
          return !1;
        if (G.safeURL = s(G.url, !1), !G.safeURL)
          return B = !0, !1;
        if (G.number > 1) {
          var R = s(G.diff_url, !0);
          if (!R || new URL(R).pathname.replace(/[^/]+$/, "") !== new URL(G.safeURL).pathname.replace(/[^/]+$/, ""))
            return B = !0, !1;
        }
        return !0;
      });
      if (B || E.revisions[0].number !== 1 || !Z.some(function(G) {
        return G.number === _;
      }))
        throw Error("revision entries are invalid");
      var C = Z.find(function(G) {
        return G.number === _;
      }), U = new URL(window.location.href);
      if (U.search = "", U.hash = "", !C || !N || new URL(C.safeURL || "").pathname.replace(/[^/]+$/, "") !== N.pathname || !U.pathname.startsWith(N.pathname))
        throw Error("current revision URL is invalid");
      var O = Math.max.apply(null, Z.map(function(G) {
        return G.number;
      }));
      if (O !== E.latest_revision)
        throw Error("latest is invalid");
      var J = S.querySelector("[data-revision-heading]");
      if (!J) {
        J = S.createElement("p"), J.className = "revision-heading", J.setAttribute("data-revision-heading", "");
        var i = S.getElementById("rendered");
        if (!i)
          throw Error("rendered view is unavailable");
        i.prepend(J);
      }
      var n = _ < O, SE = n ? "Revision " + _ + " of " + O : "Revision " + _ + " (Latest)", m = S.createElement("span");
      m.className = "revision-picker-label", m.textContent = SE, m.setAttribute("aria-hidden", "true");
      var T = S.createElement("select");
      T.setAttribute("aria-label", "Document revision"), Z.forEach(function(G) {
        var R = S.createElement("option");
        R.value = G.safeURL || "", R.textContent = G.number === O ? "Revision " + G.number + " (Latest)" : "Revision " + G.number + " of " + O, R.selected = G.number === _, T.appendChild(R);
      }), T.addEventListener("change", function() {
        var G = T.selectedIndex;
        if (G < 0 || G >= Z.length)
          return;
        var R = Z[G], b = R.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(b + (R.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var $E = K ? new URL(K.content, window.location.href).href : "";
        if (!Y || U.href === $E || !M) {
          window.location.assign(b);
          return;
        }
        J.setAttribute("aria-busy", "true"), T.disabled = !0;
        var l = new URL("./", b), _E = new URL(".airplan.json", l);
        _E.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(_E, { cache: "no-store", credentials: "same-origin" }).then(t).then(function(ME) {
          var tE = qE(ME, l, R, M.content);
          window.location.assign(kE(b, tE.get(Y.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(b);
        });
      }), J.replaceChildren(m, T), J.classList.add("is-picker"), J.classList.toggle("is-stale", n), S.body.classList.toggle("airplan-stale-revision", n);
    }
    if (L) {
      var wE = new URL(L.content, window.location.href);
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
        vE(E), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: E
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var AE = S.createElement("div");
    AE.className = "sr-status", AE.setAttribute("aria-live", "polite"), S.body.appendChild(AE);
    var p = null;
    function nE() {
      if (p !== null)
        return;
      p = Array.from(S.querySelectorAll("details:not([open])")), p.forEach(function(E) {
        E.open = !0;
      });
    }
    function lE() {
      if (p === null)
        return;
      p.forEach(function(E) {
        E.open = !1;
      }), p = null;
    }
    window.addEventListener("beforeprint", nE), window.addEventListener("afterprint", lE);
    function XE(E, A, _) {
      AE.textContent = A;
      var B = E.querySelector(".action-label"), q = B ? B.textContent : "";
      if (B)
        B.textContent = _ ? "Copied" : "Failed";
      E.classList.add(_ ? "is-copied" : "is-failed"), E.disabled = !0, setTimeout(function() {
        if (E.classList.remove("is-copied", "is-failed"), E.disabled = !1, B)
          B.textContent = q;
      }, 1200);
    }
    function CE(E, A) {
      if (!navigator.clipboard) {
        XE(A, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(E).then(function() {
        XE(A, "Copied!", !0);
      }, function() {
        XE(A, "Copy failed", !1);
      });
    }
    var pE = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', cE = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', gE = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', iE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', dE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', xE = S.getElementById("pages"), D = S.querySelector(".pages-trigger"), V = null, IE = window.matchMedia("(max-width: 78rem)"), j = function() {};
    function HE() {
      return V ? V.matches(":popover-open") : !1;
    }
    function a(E) {
      if (!V || !HE())
        return;
      if (V.hidePopover(), E && D && IE.matches)
        setTimeout(function() {
          D.focus();
        }, 0);
    }
    if (xE && D) {
      var UE = xE.querySelector(".pages-list");
      if (UE) {
        var WE = S.createElement("div");
        if ("popover" in WE && typeof WE.showPopover === "function") {
          let E = function() {
            if (!D || !V)
              return;
            var A = D.getBoundingClientRect();
            V.style.setProperty("--pages-left", Math.max(16, A.left) + "px"), V.style.setProperty("--pages-top", A.bottom + "px"), V.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, A.left) - 16) + "px");
          };
          V = WE, V.className = "pages-popover", V.id = "pages-popover", V.setAttribute("popover", "auto");
          var r = S.createElement("nav");
          r.className = "pages-popover-nav", r.setAttribute("aria-label", "Pages"), r.appendChild(UE.cloneNode(!0)), V.appendChild(r), D.setAttribute("popovertarget", V.id), D.popoverTargetElement = V, V.addEventListener("beforetoggle", function(A) {
            if (A.newState !== "open")
              return;
            j(), E();
          }), V.addEventListener("toggle", function(A) {
            var _ = A.newState === "open";
            if (D.setAttribute("aria-expanded", _ ? "true" : "false"), S.body.classList.toggle("pages-popover-open", _), _) {
              var B = V.querySelector('[aria-current="page"]');
              if (B)
                B.scrollIntoView({ block: "nearest" });
            }
            v();
          }), r.querySelectorAll("a").forEach(function(A) {
            A.addEventListener("click", function() {
              a(!1);
            });
          }), IE.addEventListener("change", function() {
            if (!IE.matches)
              a(!1);
          }), window.addEventListener("resize", function() {
            if (HE())
              E();
          }), D.hidden = !1, D.setAttribute("aria-expanded", "false"), S.body.appendChild(V), S.body.classList.add("pages-popover-ready");
        }
      }
    }
    var c = S.getElementById("source"), GE = S.getElementById("changes"), BE = S.querySelector("[data-airplan-all-changes]"), P = S.getElementById("toc"), f = null, X = null, RE = window.matchMedia("(max-width: 78rem)");
    j = function() {
      if (X && X.open)
        X.close();
    };
    function v() {
      if (!P || !f || !X)
        return;
      var E = RE.matches && !W.hidden && !X.open && !HE();
      if (f.classList.toggle("is-visible", E), f.tabIndex = E ? 0 : -1, f.setAttribute("aria-hidden", E ? "false" : "true"), X.open && (!RE.matches || W.hidden))
        j();
    }
    function LE(E) {
      if (a(!1), j(), W.hidden = E !== "rendered", c)
        c.hidden = E !== "source";
      if (GE)
        GE.hidden = E !== "changes";
      if (P)
        P.hidden = E !== "rendered";
      S.querySelectorAll(".viewtoggle button").forEach(function(A) {
        var _ = A.dataset.view === E;
        A.classList.toggle("active", _), A.setAttribute("aria-pressed", _ ? "true" : "false");
      }), v();
    }
    S.querySelectorAll(".viewtoggle button").forEach(function(E) {
      E.addEventListener("click", function() {
        LE(E.dataset.view || "rendered");
      });
    });
    var VE = !1;
    S.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(E) {
      E.addEventListener("click", function() {
        VE = new URL(E.href).pathname === window.location.pathname;
      });
    });
    function TE() {
      var E = window.location.hash === "#airplan-all-changes" && !!BE;
      if (a(!1), j(), S.body.classList.toggle("all-changes-active", E), BE)
        BE.hidden = !E;
      if (E) {
        if (W.hidden = !0, c)
          c.hidden = !0;
        if (GE)
          GE.hidden = !0;
        if (P)
          P.hidden = !0;
        if (VE)
          BE.querySelector("h1")?.focus();
      } else
        LE("rendered");
      VE = !1, v();
    }
    if (window.addEventListener("hashchange", TE), TE(), P) {
      let E = function() {
        if (e.length === 0) {
          v();
          return;
        }
        var _ = 0;
        if (sE.forEach(function(q, Z) {
          if (q && q.getBoundingClientRect().top <= 128)
            _ = Z;
        }), window.innerHeight + window.scrollY >= S.documentElement.scrollHeight - 2)
          _ = e.length - 1;
        var B = e[_].getAttribute("href");
        FE.forEach(function(q) {
          var Z = q.getAttribute("href") === B;
          if (q.classList.toggle("active", Z), Z)
            q.setAttribute("aria-current", "location");
          else
            q.removeAttribute("aria-current");
        }), v();
      }, A = function() {
        if (KE)
          return;
        KE = !0, window.requestAnimationFrame(function() {
          KE = !1, E();
        });
      };
      var e = Array.from(P.querySelectorAll('a[href^="#"]')), hE = P.querySelector(".toc-list");
      if (hE)
        if (X = S.createElement("dialog"), typeof X.showModal === "function") {
          X.className = "toc-dialog", X.id = "toc-dialog", X.setAttribute("aria-labelledby", "toc-dialog-title");
          var JE = S.createElement("div");
          JE.className = "toc-dialog-panel";
          var QE = S.createElement("div");
          QE.className = "toc-dialog-header";
          var YE = S.createElement("h2");
          YE.className = "toc-dialog-title", YE.id = "toc-dialog-title", YE.textContent = "Contents";
          var g = S.createElement("button");
          g.className = "toc-dialog-close", g.type = "button", g.setAttribute("aria-label", "Close table of contents"), g.innerHTML = dE, QE.appendChild(YE), QE.appendChild(g);
          var EE = S.createElement("nav");
          EE.className = "toc-dialog-nav", EE.setAttribute("aria-label", "Table of contents"), EE.appendChild(hE.cloneNode(!0)), JE.appendChild(QE), JE.appendChild(EE), X.appendChild(JE), f = S.createElement("button"), f.className = "toc-trigger", f.type = "button", f.tabIndex = -1, f.setAttribute("aria-label", "Open table of contents"), f.setAttribute("aria-controls", "toc-dialog"), f.setAttribute("aria-haspopup", "dialog"), f.setAttribute("aria-hidden", "true"), f.innerHTML = iE, S.body.appendChild(f), S.body.appendChild(X), S.body.classList.add("toc-dialog-ready"), f.addEventListener("click", function() {
            a(!1), X.showModal(), S.body.classList.add("toc-dialog-open"), v();
            var _ = X.querySelector("a.active");
            if (_)
              _.scrollIntoView({ block: "nearest" });
          }), g.addEventListener("click", j), X.addEventListener("click", function(_) {
            if (_.target === X)
              j();
          }), X.addEventListener("keydown", function(_) {
            if (_.key === "Escape")
              _.preventDefault(), j();
          }), X.addEventListener("close", function() {
            if (S.body.classList.remove("toc-dialog-open"), v(), f.classList.contains("is-visible"))
              setTimeout(function() {
                f.focus();
              }, 50);
          }), EE.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", j);
          });
        } else
          X = null;
      var FE = e.slice();
      if (X)
        FE = FE.concat(Array.from(X.querySelectorAll('a[href^="#"]')));
      var sE = e.map(function(_) {
        return S.getElementById((_.getAttribute("href") || "").slice(1));
      }), KE = !1;
      S.addEventListener("scroll", A, { passive: !0 }), window.addEventListener("resize", E), E();
    }
    var ZE = S.querySelector(".toolbar");
    function fE() {
      var E = ZE && window.matchMedia("(max-width: 48rem)").matches ? ZE.getBoundingClientRect().height : 0;
      S.documentElement.style.setProperty("--airplan-sticky-height", E + "px");
    }
    if (ZE) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver(fE).observe(ZE);
      window.addEventListener("resize", fE), fE();
    }
    let OE = S.querySelector(".copy-source");
    if (OE && c)
      OE.addEventListener("click", function() {
        var E = c.querySelector("pre");
        CE(E ? E.textContent : "", OE);
      });
    W.querySelectorAll("pre").forEach(function(E) {
      if (E.classList.contains("mermaid"))
        return;
      var A = S.createElement("div");
      A.className = "codewrap", E.parentNode?.insertBefore(A, E), A.appendChild(E);
      var _ = S.createElement("button");
      _.className = "codecopy", _.type = "button", _.setAttribute("aria-label", "Copy code"), _.title = "Copy code", _.innerHTML = pE + cE + gE, _.addEventListener("click", function() {
        var B = E.querySelector("code");
        CE((B || E).textContent, _);
      }), A.appendChild(_);
    });
  })();
})();
