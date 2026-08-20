(() => {
  function PE(S) {
    return S === "system" || S === "light" || S === "dark";
  }
  function IE(S, J) {
    try {
      return S?.getItem(J) ?? null;
    } catch {
      return null;
    }
  }
  function g(S, J, Q) {
    try {
      if (Q === null)
        S?.removeItem(J);
      else
        S?.setItem(J, Q);
    } catch {}
  }
  function CE(S, J, Q) {
    let $ = IE(Q, "airplan-color-mode");
    if ($ === null) {
      let O = IE(Q, "airplan-theme");
      if ($ = O === "light" || O === "dark" ? O : "system", $ !== "system")
        g(Q, "airplan-color-mode", $);
    }
    let x = PE($) ? $ : "system", V = new Set(S.themes.map((O) => O.id)), I = IE(Q, "airplan-light-theme"), X = IE(Q, "airplan-dark-theme"), K = I !== null && V.has(I) ? I : S.defaultLight, w = X !== null && V.has(X) ? X : S.defaultDark;
    return HE(S, x, K, w, J);
  }
  function HE(S, J, Q, $, x) {
    let V = new Map(S.themes.map((U) => [U.id, U])), I = V.has(Q) ? Q : S.defaultLight, X = V.has($) ? $ : S.defaultDark, K = J === "system" ? x ? "dark" : "light" : J, w = K === "light" ? I : X, O = V.get(w)?.variant ?? K;
    return { mode: J, resolvedMode: K, lightTheme: I, darkTheme: X, theme: w, variant: O };
  }
  function uE(S, J) {
    if (J === "system")
      g(S, "airplan-color-mode", null), g(S, "airplan-theme", null);
    else
      g(S, "airplan-color-mode", J), g(S, "airplan-theme", J);
  }
  function RE(S, J, Q) {
    g(S, J === "light" ? "airplan-light-theme" : "airplan-dark-theme", Q);
  }
  function hE(S) {
    return {
      mode: S.mode,
      resolvedMode: S.resolvedMode,
      theme: S.theme,
      variant: S.variant
    };
  }

  (function() {
    let S = document, J = S.documentElement;
    S.querySelectorAll(".js-only").forEach((B) => {
      B.hidden = !1;
    });
    let Q = window.__AIRPLAN_THEME_CATALOG__;
    if (!Q)
      return;
    let $ = Q, x = window.matchMedia("(prefers-color-scheme: dark)"), V;
    try {
      V = window.localStorage;
    } catch {}
    let I = window.__airplanThemeState ?? CE($, x.matches, V), X = S.querySelector("[data-airplan-appearance-trigger]"), K = S.querySelector("[data-airplan-appearance-panel]"), w = S.querySelector('select[data-airplan-theme-slot="light"]'), O = S.querySelector('select[data-airplan-theme-slot="dark"]'), U = Array.from(S.querySelectorAll("[data-airplan-color-mode]"));
    function i(B) {
      if (!B || B.options.length > 0)
        return;
      for (let [z, F] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let N = S.createElement("optgroup");
        N.label = F;
        for (let b of $.themes) {
          if (b.variant !== z)
            continue;
          let k = S.createElement("option");
          k.value = b.id, k.textContent = b.name, N.append(k);
        }
        if (N.children.length > 0)
          B.append(N);
      }
    }
    i(w), i(O);
    function v(B, z = !0) {
      if (I = B, window.__airplanThemeState = I, J.dataset.airplanMode = I.mode, J.dataset.airplanResolvedMode = I.resolvedMode, J.dataset.airplanTheme = I.theme, J.dataset.airplanThemeVariant = I.variant, U.forEach((F) => {
        let N = F.dataset.airplanColorMode === I.mode;
        F.classList.toggle("active", N), F.setAttribute("aria-pressed", String(N));
      }), w)
        w.value = I.lightTheme;
      if (O)
        O.value = I.darkTheme;
      if (z)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: hE(I) }));
    }
    function s(B = {}) {
      v(HE($, B.mode ?? I.mode, B.lightTheme ?? I.lightTheme, B.darkTheme ?? I.darkTheme, x.matches));
    }
    function t(B, z = !1) {
      if (!K || !X)
        return;
      if (K.hidden = !B, X.setAttribute("aria-expanded", String(B)), B)
        K.querySelector("button,select")?.focus();
      else if (z)
        X.focus();
    }
    X?.addEventListener("click", () => t(Boolean(K?.hidden ?? !0))), U.forEach((B) => B.addEventListener("click", () => {
      let z = B.dataset.airplanColorMode;
      if (!z)
        return;
      uE(V, z), s({ mode: z });
    }));
    function EE(B, z) {
      RE(V, B, z.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: z.value } })), s(B === "light" ? { lightTheme: z.value } : { darkTheme: z.value });
    }
    w?.addEventListener("change", () => EE("light", w)), O?.addEventListener("change", () => EE("dark", O)), x.addEventListener("change", () => {
      if (I.mode === "system")
        s();
    }), S.addEventListener("keydown", (B) => {
      if (B.key === "Escape" && K && !K.hidden)
        B.preventDefault(), t(!1, !0);
    }), S.addEventListener("pointerdown", (B) => {
      if (!K || K.hidden || !X)
        return;
      let z = B.target;
      if (!(z instanceof Node) || K.contains(z) || X.contains(z))
        return;
      let N = (z instanceof Element ? z : z.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), b = K.contains(S.activeElement) && !N;
      if (t(!1), b)
        setTimeout(() => {
          if (S.activeElement === S.body || K.contains(S.activeElement))
            X.focus();
        });
    }), v(I, !1);
  })();

  (function() {
    var S = document;
    let J = S.getElementById("rendered");
    if (!J)
      return;
    let Q = J;
    var $ = S.querySelector('meta[name="airplan-versions"]'), x = S.querySelector('meta[name="airplan-revision-chain"]'), V = S.querySelector('meta[name="airplan-page-path"]'), I = S.querySelector('meta[name="airplan-entrypoint"]'), X = $ ? new URL($.content, window.location.href) : null, K = X ? new URL("./", X) : null, w = K ? K.pathname.split("/").filter(Boolean) : [], O = w.slice(0, -1);
    function U(E, A) {
      if (typeof E !== "string")
        return null;
      try {
        var _ = new URL(E);
        if (_.origin !== window.location.origin || _.username || _.password || _.search || _.hash)
          return null;
        var f = _.pathname.split("/").filter(Boolean);
        if (f.length !== O.length + 2 || !O.every(function(Z, L) {
          return f[L] === Z;
        }) || !/^[a-z2-7]{26}$/.test(f[f.length - 2]))
          return null;
        var q = f[f.length - 1];
        if (A ? q !== ".airplan-changes.diff" : !q.endsWith(".html"))
          return null;
        return _.href;
      } catch {
        return null;
      }
    }
    function i(E) {
      if (typeof E !== "string" || E === "" || E.startsWith("/") || E.includes("\\"))
        return !1;
      var A = E.split("/");
      return A.every(function(_) {
        var f = Array.from(_).some(function(q) {
          var Z = q.codePointAt(0) || 0;
          return Z < 32 || Z === 127;
        });
        if (!_ || _ === "." || _ === ".." || _.startsWith(".airplan-") || f || /[. ]$/.test(_) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(_))
          return !1;
        return !0;
      });
    }
    function v(E, A) {
      if (!i(A))
        return null;
      var _ = String(A);
      if (_.split("/", 1)[0].includes(":"))
        _ = "./" + _;
      var f = new URL(_, E);
      if (f.origin !== E.origin || f.username || f.password || f.search || f.hash || !f.pathname.startsWith(E.pathname))
        return null;
      return f.href;
    }
    function s(E, A, _, f) {
      if (!E || typeof E !== "object")
        throw Error("marker is invalid");
      var q = E;
      if (q.schema !== "airplan-upload" || q.version !== 6 || q.kind !== "document" || !q.revision || q.revision.number !== _.number || q.revision.chain_id !== f || !Array.isArray(q.pages) || q.pages.length === 0)
        throw Error("marker identity is invalid");
      var Z = v(A, q.entrypoint);
      if (Z !== _.safeURL)
        throw Error("marker entrypoint is invalid");
      var L = new Set, T = new Set, y = new Set, M = new Map;
      q.pages.forEach(function(W) {
        if (!W || !i(W.path) || L.has(W.path) || T.has(W.path.toLowerCase()) || W.format !== "md" && W.format !== "txt" || typeof W.lang !== "string")
          throw Error("marker page descriptor is invalid");
        var h = v(A, W.page);
        if (!h || y.has(h))
          throw Error("marker page object is invalid");
        if (W.source && !v(A, W.source))
          throw Error("marker source object is invalid");
        L.add(W.path), T.add(W.path.toLowerCase()), y.add(h), M.set(W.path, h);
      });
      var c = Array.from(T).sort();
      for (var D = 1;D < c.length; D += 1)
        if (c[D].startsWith(c[D - 1] + "/"))
          throw Error("marker page paths conflict");
      if (!L.has(q.pages[0].path) || M.get(q.pages[0].path) !== Z)
        throw Error("marker entry page is invalid");
      return M;
    }
    function t(E, A) {
      var _ = window.location.hash;
      if (_ === "#airplan-all-changes")
        return E + _;
      if (!A)
        return E;
      return A + (_ && _ !== "#airplan-all-changes" ? _ : "");
    }
    function EE(E) {
      var A = S.querySelector('meta[name="airplan-revision"]'), _ = A ? Number(A.content) : Number(E.current_revision);
      if (!Number.isInteger(_) || _ <= 0 || E.current_revision !== _ || !Number.isInteger(E.latest_revision) || !Number.isInteger(E.last_assigned_revision) || !Array.isArray(E.revisions) || E.revisions.length === 0 || E.last_assigned_revision !== E.revisions.length || !/^[a-z2-7]{26}$/.test(E.chain_id) || x && x.content !== E.chain_id)
        throw Error("revision identity is invalid");
      var f = !1, q = 0, Z = E.revisions.filter(function(G) {
        if (!G || !Number.isInteger(G.number) || G.number !== q + 1)
          return f = !0, !1;
        if (q = G.number, G.deleted)
          return !1;
        if (G.safeURL = U(G.url, !1), !G.safeURL)
          return f = !0, !1;
        if (G.number > 1) {
          var C = U(G.diff_url, !0);
          if (!C || new URL(C).pathname.replace(/[^/]+$/, "") !== new URL(G.safeURL).pathname.replace(/[^/]+$/, ""))
            return f = !0, !1;
        }
        return !0;
      });
      if (f || E.revisions[0].number !== 1 || !Z.some(function(G) {
        return G.number === _;
      }))
        throw Error("revision entries are invalid");
      var L = Z.find(function(G) {
        return G.number === _;
      }), T = new URL(window.location.href);
      if (T.search = "", T.hash = "", !L || !K || new URL(L.safeURL || "").pathname.replace(/[^/]+$/, "") !== K.pathname || !T.pathname.startsWith(K.pathname))
        throw Error("current revision URL is invalid");
      var y = Math.max.apply(null, Z.map(function(G) {
        return G.number;
      }));
      if (y !== E.latest_revision)
        throw Error("latest is invalid");
      var M = S.querySelector("[data-revision-heading]");
      if (!M) {
        M = S.createElement("p"), M.className = "revision-heading", M.setAttribute("data-revision-heading", "");
        var c = S.getElementById("rendered");
        if (!c)
          throw Error("rendered view is unavailable");
        c.prepend(M);
      }
      var D = _ < y, W = D ? "Revision " + _ + " of " + y : "Revision " + _ + " (Latest)", h = S.createElement("span");
      h.className = "revision-picker-label", h.textContent = W, h.setAttribute("aria-hidden", "true");
      var d = S.createElement("select");
      d.setAttribute("aria-label", "Document revision"), Z.forEach(function(G) {
        var C = S.createElement("option");
        C.value = G.safeURL || "", C.textContent = G.number === y ? "Revision " + G.number + " (Latest)" : "Revision " + G.number + " of " + y, C.selected = G.number === _, d.appendChild(C);
      }), d.addEventListener("change", function() {
        var G = d.selectedIndex;
        if (G < 0 || G >= Z.length)
          return;
        var C = Z[G], e = C.safeURL || "";
        if (window.location.hash === "#airplan-all-changes") {
          window.location.assign(e + (C.number > 1 ? "#airplan-all-changes" : ""));
          return;
        }
        var bE = I ? new URL(I.content, window.location.href).href : "";
        if (!V || T.href === bE || !x) {
          window.location.assign(e);
          return;
        }
        M.setAttribute("aria-busy", "true"), d.disabled = !0;
        var wE = new URL("./", e), NE = new URL(".airplan.json", wE);
        NE.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(NE, { cache: "no-store", credentials: "same-origin" }).then(function(zE) {
          if (!zE.ok)
            throw Error("marker request failed");
          return zE.json();
        }).then(function(zE) {
          var kE = s(zE, wE, C, x.content);
          window.location.assign(t(e, kE.get(V.content) || null));
        }).catch(function() {
          console.warn("airplan: selected revision page map is unavailable or invalid"), window.location.assign(e);
        });
      }), M.replaceChildren(h, d), M.classList.add("is-picker"), M.classList.toggle("is-stale", D), S.body.classList.toggle("airplan-stale-revision", D);
    }
    if ($) {
      var B = new URL($.content, window.location.href);
      B.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(B, { cache: "no-store", credentials: "same-origin" }).then(function(E) {
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
        EE(E), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: E
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var z = S.createElement("div");
    z.className = "sr-status", z.setAttribute("aria-live", "polite"), S.body.appendChild(z);
    var F = null;
    function N() {
      if (F !== null)
        return;
      F = Array.from(S.querySelectorAll("details:not([open])")), F.forEach(function(E) {
        E.open = !0;
      });
    }
    function b() {
      if (F === null)
        return;
      F.forEach(function(E) {
        E.open = !1;
      }), F = null;
    }
    window.addEventListener("beforeprint", N), window.addEventListener("afterprint", b);
    function k(E, A, _) {
      z.textContent = A;
      var f = E.querySelector(".action-label"), q = f ? f.textContent : "";
      if (f)
        f.textContent = _ ? "Copied" : "Failed";
      E.classList.add(_ ? "is-copied" : "is-failed"), E.disabled = !0, setTimeout(function() {
        if (E.classList.remove("is-copied", "is-failed"), E.disabled = !1, f)
          f.textContent = q;
      }, 1200);
    }
    function jE(E, A) {
      if (!navigator.clipboard) {
        k(A, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(E).then(function() {
        k(A, "Copied!", !0);
      }, function() {
        k(A, "Copy failed", !1);
      });
    }
    var UE = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', mE = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', LE = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', TE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', yE = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', OE = S.getElementById("pages"), u = S.querySelector(".pages-trigger"), H = null, JE = window.matchMedia("(max-width: 78rem)"), R = function() {};
    function SE() {
      return H ? H.matches(":popover-open") : !1;
    }
    function n(E) {
      if (!H || !SE())
        return;
      if (H.hidePopover(), E && u && JE.matches)
        setTimeout(function() {
          u.focus();
        }, 0);
    }
    if (OE && u) {
      var WE = OE.querySelector(".pages-list");
      if (WE) {
        var KE = S.createElement("div");
        if ("popover" in KE && typeof KE.showPopover === "function") {
          let E = function() {
            if (!u || !H)
              return;
            var A = u.getBoundingClientRect();
            H.style.setProperty("--pages-left", Math.max(16, A.left) + "px"), H.style.setProperty("--pages-top", A.bottom + "px"), H.style.setProperty("--pages-width", Math.min(480, window.innerWidth - Math.max(16, A.left) - 16) + "px");
          };
          H = KE, H.className = "pages-popover", H.id = "pages-popover", H.setAttribute("popover", "auto");
          var o = S.createElement("nav");
          o.className = "pages-popover-nav", o.setAttribute("aria-label", "Pages"), o.appendChild(WE.cloneNode(!0)), H.appendChild(o), u.addEventListener("click", function() {
            if (R(), E(), SE())
              n(!0);
            else {
              H.showPopover();
              var A = H.querySelector('[aria-current="page"]');
              if (A)
                A.scrollIntoView({ block: "nearest" });
            }
          }), H.addEventListener("toggle", function(A) {
            var _ = A.newState === "open";
            u.setAttribute("aria-expanded", _ ? "true" : "false"), S.body.classList.toggle("pages-popover-open", _), P();
          }), o.querySelectorAll("a").forEach(function(A) {
            A.addEventListener("click", function() {
              n(!1);
            });
          }), JE.addEventListener("change", function() {
            if (!JE.matches)
              n(!1);
          }), window.addEventListener("resize", function() {
            if (SE())
              E();
          }), u.hidden = !1, u.setAttribute("aria-expanded", "false"), S.body.appendChild(H), S.body.classList.add("pages-popover-ready");
        }
      }
    }
    var l = S.getElementById("source"), _E = S.getElementById("changes"), AE = S.querySelector("[data-airplan-all-changes]"), m = S.getElementById("toc"), j = null, Y = null, VE = window.matchMedia("(max-width: 78rem)");
    R = function() {
      if (Y && Y.open)
        Y.close();
    };
    function P() {
      if (!m || !j || !Y)
        return;
      var E = VE.matches && !Q.hidden && !Y.open && !SE();
      if (j.classList.toggle("is-visible", E), j.tabIndex = E ? 0 : -1, j.setAttribute("aria-hidden", E ? "false" : "true"), Y.open && (!VE.matches || Q.hidden))
        R();
    }
    function FE(E) {
      if (n(!1), R(), Q.hidden = E !== "rendered", l)
        l.hidden = E !== "source";
      if (_E)
        _E.hidden = E !== "changes";
      if (m)
        m.hidden = E !== "rendered";
      S.querySelectorAll(".viewtoggle button").forEach(function(A) {
        var _ = A.dataset.view === E;
        A.classList.toggle("active", _), A.setAttribute("aria-pressed", _ ? "true" : "false");
      }), P();
    }
    S.querySelectorAll(".viewtoggle button").forEach(function(E) {
      E.addEventListener("click", function() {
        FE(E.dataset.view || "rendered");
      });
    });
    var QE = !1;
    S.querySelectorAll('.all-changes-link[href$="#airplan-all-changes"]').forEach(function(E) {
      E.addEventListener("click", function() {
        QE = new URL(E.href).pathname === window.location.pathname;
      });
    });
    function ME() {
      var E = window.location.hash === "#airplan-all-changes" && !!AE;
      if (n(!1), R(), S.body.classList.toggle("all-changes-active", E), AE)
        AE.hidden = !E;
      if (E) {
        if (Q.hidden = !0, l)
          l.hidden = !0;
        if (_E)
          _E.hidden = !0;
        if (m)
          m.hidden = !0;
        if (QE)
          AE.querySelector("h1")?.focus();
      } else
        FE("rendered");
      QE = !1, P();
    }
    if (window.addEventListener("hashchange", ME), ME(), m) {
      let E = function() {
        if (r.length === 0) {
          P();
          return;
        }
        var _ = 0;
        if (DE.forEach(function(q, Z) {
          if (q && q.getBoundingClientRect().top <= 128)
            _ = Z;
        }), window.innerHeight + window.scrollY >= S.documentElement.scrollHeight - 2)
          _ = r.length - 1;
        var f = r[_].getAttribute("href");
        YE.forEach(function(q) {
          var Z = q.getAttribute("href") === f;
          if (q.classList.toggle("active", Z), Z)
            q.setAttribute("aria-current", "location");
          else
            q.removeAttribute("aria-current");
        }), P();
      }, A = function() {
        if (ZE)
          return;
        ZE = !0, window.requestAnimationFrame(function() {
          ZE = !1, E();
        });
      };
      var r = Array.from(m.querySelectorAll('a[href^="#"]')), xE = m.querySelector(".toc-list");
      if (xE)
        if (Y = S.createElement("dialog"), typeof Y.showModal === "function") {
          Y.className = "toc-dialog", Y.id = "toc-dialog", Y.setAttribute("aria-labelledby", "toc-dialog-title");
          var fE = S.createElement("div");
          fE.className = "toc-dialog-panel";
          var qE = S.createElement("div");
          qE.className = "toc-dialog-header";
          var BE = S.createElement("h2");
          BE.className = "toc-dialog-title", BE.id = "toc-dialog-title", BE.textContent = "Contents";
          var p = S.createElement("button");
          p.className = "toc-dialog-close", p.type = "button", p.setAttribute("aria-label", "Close table of contents"), p.innerHTML = yE, qE.appendChild(BE), qE.appendChild(p);
          var a = S.createElement("nav");
          a.className = "toc-dialog-nav", a.setAttribute("aria-label", "Table of contents"), a.appendChild(xE.cloneNode(!0)), fE.appendChild(qE), fE.appendChild(a), Y.appendChild(fE), j = S.createElement("button"), j.className = "toc-trigger", j.type = "button", j.tabIndex = -1, j.setAttribute("aria-label", "Open table of contents"), j.setAttribute("aria-controls", "toc-dialog"), j.setAttribute("aria-haspopup", "dialog"), j.setAttribute("aria-hidden", "true"), j.innerHTML = TE, S.body.appendChild(j), S.body.appendChild(Y), S.body.classList.add("toc-dialog-ready"), j.addEventListener("click", function() {
            n(!1), Y.showModal(), S.body.classList.add("toc-dialog-open"), P();
            var _ = Y.querySelector("a.active");
            if (_)
              _.scrollIntoView({ block: "nearest" });
          }), p.addEventListener("click", R), Y.addEventListener("click", function(_) {
            if (_.target === Y)
              R();
          }), Y.addEventListener("keydown", function(_) {
            if (_.key === "Escape")
              _.preventDefault(), R();
          }), Y.addEventListener("close", function() {
            if (S.body.classList.remove("toc-dialog-open"), P(), j.classList.contains("is-visible"))
              setTimeout(function() {
                j.focus();
              }, 50);
          }), a.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", R);
          });
        } else
          Y = null;
      var YE = r.slice();
      if (Y)
        YE = YE.concat(Array.from(Y.querySelectorAll('a[href^="#"]')));
      var DE = r.map(function(_) {
        return S.getElementById((_.getAttribute("href") || "").slice(1));
      }), ZE = !1;
      S.addEventListener("scroll", A, { passive: !0 }), window.addEventListener("resize", E), E();
    }
    var GE = S.querySelector(".toolbar");
    function $E() {
      var E = GE && window.matchMedia("(max-width: 48rem)").matches ? GE.getBoundingClientRect().height : 0;
      S.documentElement.style.setProperty("--airplan-sticky-height", E + "px");
    }
    if (GE) {
      if (typeof ResizeObserver === "function")
        new ResizeObserver($E).observe(GE);
      window.addEventListener("resize", $E), $E();
    }
    let XE = S.querySelector(".copy-source");
    if (XE && l)
      XE.addEventListener("click", function() {
        var E = l.querySelector("pre");
        jE(E ? E.textContent : "", XE);
      });
    Q.querySelectorAll("pre").forEach(function(E) {
      if (E.classList.contains("mermaid"))
        return;
      var A = S.createElement("div");
      A.className = "codewrap", E.parentNode?.insertBefore(A, E), A.appendChild(E);
      var _ = S.createElement("button");
      _.className = "codecopy", _.type = "button", _.setAttribute("aria-label", "Copy code"), _.title = "Copy code", _.innerHTML = UE + mE + LE, _.addEventListener("click", function() {
        var f = E.querySelector("code");
        jE((f || E).textContent, _);
      }), A.appendChild(_);
    });
  })();
})();
