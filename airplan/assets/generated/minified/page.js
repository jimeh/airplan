(() => {
  function $E(E) {
    return E === "system" || E === "light" || E === "dark";
  }
  function EE(E, M) {
    try {
      return E?.getItem(M) ?? null;
    } catch {
      return null;
    }
  }
  function N(E, M, I) {
    try {
      if (I === null)
        E?.removeItem(M);
      else
        E?.setItem(M, I);
    } catch {}
  }
  function OE(E, M, I) {
    let K = EE(I, "airplan-color-mode");
    if (K === null) {
      let j = EE(I, "airplan-theme");
      if (K = j === "light" || j === "dark" ? j : "system", K !== "system")
        N(I, "airplan-color-mode", K);
    }
    let w = $E(K) ? K : "system", $ = new Set(E.themes.map((j) => j.id)), A = EE(I, "airplan-light-theme"), Q = EE(I, "airplan-dark-theme"), O = A !== null && $.has(A) ? A : E.defaultLight, Y = Q !== null && $.has(Q) ? Q : E.defaultDark;
    return fE(E, w, O, Y, M);
  }
  function fE(E, M, I, K, w) {
    let $ = new Map(E.themes.map((R) => [R.id, R])), A = $.has(I) ? I : E.defaultLight, Q = $.has(K) ? K : E.defaultDark, O = M === "system" ? w ? "dark" : "light" : M, Y = O === "light" ? A : Q, j = $.get(Y)?.variant ?? O;
    return { mode: M, resolvedMode: O, lightTheme: A, darkTheme: Q, theme: Y, variant: j };
  }
  function jE(E, M) {
    if (M === "system")
      N(E, "airplan-color-mode", null), N(E, "airplan-theme", null);
    else
      N(E, "airplan-color-mode", M), N(E, "airplan-theme", M);
  }
  function zE(E, M, I) {
    N(E, M === "light" ? "airplan-light-theme" : "airplan-dark-theme", I);
  }
  function JE(E) {
    return {
      mode: E.mode,
      resolvedMode: E.resolvedMode,
      theme: E.theme,
      variant: E.variant
    };
  }

  (function() {
    let E = document, M = E.documentElement;
    E.querySelectorAll(".js-only").forEach((u) => {
      u.hidden = !1;
    });
    let I = window.__AIRPLAN_THEME_CATALOG__;
    if (!I)
      return;
    let K = I, w = window.matchMedia("(prefers-color-scheme: dark)"), $;
    try {
      $ = window.localStorage;
    } catch {}
    let A = window.__airplanThemeState ?? OE(K, w.matches, $), Q = E.querySelector("[data-airplan-appearance-trigger]"), O = E.querySelector("[data-airplan-appearance-panel]"), Y = E.querySelector('select[data-airplan-theme-slot="light"]'), j = E.querySelector('select[data-airplan-theme-slot="dark"]'), R = Array.from(E.querySelectorAll("[data-airplan-color-mode]"));
    function c(u) {
      if (!u || u.options.length > 0)
        return;
      for (let [q, F] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let W = E.createElement("optgroup");
        W.label = F;
        for (let X of K.themes) {
          if (X.variant !== q)
            continue;
          let z = E.createElement("option");
          z.value = X.id, z.textContent = X.name, W.append(z);
        }
        if (W.children.length > 0)
          u.append(W);
      }
    }
    c(Y), c(j);
    function m(u, q = !0) {
      if (A = u, window.__airplanThemeState = A, M.dataset.airplanMode = A.mode, M.dataset.airplanResolvedMode = A.resolvedMode, M.dataset.airplanTheme = A.theme, M.dataset.airplanThemeVariant = A.variant, R.forEach((F) => {
        let W = F.dataset.airplanColorMode === A.mode;
        F.classList.toggle("active", W), F.setAttribute("aria-pressed", String(W));
      }), Y)
        Y.value = A.lightTheme;
      if (j)
        j.value = A.darkTheme;
      if (q)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: JE(A) }));
    }
    function y(u = {}) {
      m(fE(K, u.mode ?? A.mode, u.lightTheme ?? A.lightTheme, u.darkTheme ?? A.darkTheme, w.matches));
    }
    function P(u, q = !1) {
      if (!O || !Q)
        return;
      if (O.hidden = !u, Q.setAttribute("aria-expanded", String(u)), u)
        O.querySelector("button,select")?.focus();
      else if (q)
        Q.focus();
    }
    Q?.addEventListener("click", () => P(Boolean(O?.hidden ?? !0))), R.forEach((u) => u.addEventListener("click", () => {
      let q = u.dataset.airplanColorMode;
      if (!q)
        return;
      jE($, q), y({ mode: q });
    }));
    function l(u, q) {
      zE($, u, q.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: q.value } })), y(u === "light" ? { lightTheme: q.value } : { darkTheme: q.value });
    }
    Y?.addEventListener("change", () => l("light", Y)), j?.addEventListener("change", () => l("dark", j)), w.addEventListener("change", () => {
      if (A.mode === "system")
        y();
    }), E.addEventListener("keydown", (u) => {
      if (u.key === "Escape" && O && !O.hidden)
        u.preventDefault(), P(!1, !0);
    }), E.addEventListener("pointerdown", (u) => {
      if (!O || O.hidden || !Q)
        return;
      let q = u.target;
      if (!(q instanceof Node) || O.contains(q) || Q.contains(q))
        return;
      let W = (q instanceof Element ? q : q.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), X = O.contains(E.activeElement) && !W;
      if (P(!1), X)
        setTimeout(() => {
          if (E.activeElement === E.body || O.contains(E.activeElement))
            Q.focus();
        });
    }), m(A, !1);
  })();

  (function() {
    var E = document;
    let M = E.getElementById("rendered");
    if (!M)
      return;
    let I = M;
    var K = E.querySelector('meta[name="airplan-versions"]'), w = window.location.pathname.split("/").filter(Boolean), $ = w.slice(0, -2);
    function A(S, h) {
      if (typeof S !== "string")
        return null;
      try {
        var _ = new URL(S);
        if (_.origin !== window.location.origin || _.username || _.password || _.search || _.hash)
          return null;
        var G = _.pathname.split("/").filter(Boolean);
        if (G.length !== $.length + 2 || !$.every(function(V, a) {
          return G[a] === V;
        }) || !/^[a-z2-7]{26}$/.test(G[G.length - 2]))
          return null;
        var Z = G[G.length - 1];
        if (h ? Z !== ".airplan-changes.diff" : !Z.endsWith(".html"))
          return null;
        return _.href;
      } catch {
        return null;
      }
    }
    function Q(S) {
      var h = E.querySelector('meta[name="airplan-revision"]'), _ = h ? Number(h.content) : Number(S.current_revision);
      if (!Number.isInteger(_) || _ <= 0 || S.current_revision !== _ || !Number.isInteger(S.latest_revision) || !Number.isInteger(S.last_assigned_revision) || !Array.isArray(S.revisions) || S.revisions.length === 0 || S.last_assigned_revision !== S.revisions.length || !/^[a-z2-7]{26}$/.test(S.chain_id))
        throw Error("revision identity is invalid");
      var G = !1, Z = 0, V = S.revisions.filter(function(f) {
        if (!f || !Number.isInteger(f.number) || f.number !== Z + 1)
          return G = !0, !1;
        if (Z = f.number, f.deleted)
          return !1;
        if (f.safeURL = A(f.url, !1), !f.safeURL)
          return G = !0, !1;
        if (f.number > 1) {
          var H = A(f.diff_url, !0);
          if (!H || new URL(H).pathname.replace(/[^/]+$/, "") !== new URL(f.safeURL).pathname.replace(/[^/]+$/, ""))
            return G = !0, !1;
        }
        return !0;
      });
      if (G || S.revisions[0].number !== 1 || !V.some(function(f) {
        return f.number === _;
      }))
        throw Error("revision entries are invalid");
      var a = V.find(function(f) {
        return f.number === _;
      }), YE = window.location.origin + window.location.pathname;
      if (!a || a.safeURL !== YE)
        throw Error("current revision URL is invalid");
      var v = Math.max.apply(null, V.map(function(f) {
        return f.number;
      }));
      if (v !== S.latest_revision)
        throw Error("latest is invalid");
      var x = E.querySelector("[data-revision-heading]");
      if (!x) {
        x = E.createElement("p"), x.className = "revision-heading", x.setAttribute("data-revision-heading", "");
        var IE = E.getElementById("rendered");
        if (!IE)
          throw Error("rendered view is unavailable");
        IE.prepend(x);
      }
      var uE = _ < v, ZE = uE ? "Revision " + _ + " of " + v : "Revision " + _ + " (Latest)", e = E.createElement("span");
      e.className = "revision-picker-label", e.textContent = ZE, e.setAttribute("aria-hidden", "true");
      var p = E.createElement("select");
      p.setAttribute("aria-label", "Document revision"), V.forEach(function(f) {
        var H = E.createElement("option");
        H.value = f.safeURL || "", H.textContent = f.number === v ? "Revision " + f.number + " (Latest)" : "Revision " + f.number + " of " + v, H.selected = f.number === _, p.appendChild(H);
      }), p.addEventListener("change", function() {
        var f = p.selectedIndex;
        if (f < 0 || f >= V.length)
          return;
        window.location.assign(V[f].safeURL || "");
      }), x.replaceChildren(e, p), x.classList.add("is-picker"), x.classList.toggle("is-stale", uE), E.body.classList.toggle("airplan-stale-revision", uE);
    }
    if (K) {
      var O = new URL(K.content, window.location.href);
      O.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(O, { cache: "no-store", credentials: "same-origin" }).then(function(S) {
        if (S.status === 404)
          return null;
        if (!S.ok)
          throw Error("metadata request failed");
        return S.json();
      }).then(function(S) {
        if (S === null)
          return;
        if (!S || S.schema !== "airplan-versions" || S.version !== 1 || !Array.isArray(S.revisions) || S.revisions.length < 2)
          throw Error("metadata is invalid");
        Q(S), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: S
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var Y = E.createElement("div");
    Y.className = "sr-status", Y.setAttribute("aria-live", "polite"), E.body.appendChild(Y);
    var j = null;
    function R() {
      if (j !== null)
        return;
      j = Array.from(E.querySelectorAll("details:not([open])")), j.forEach(function(S) {
        S.open = !0;
      });
    }
    function c() {
      if (j === null)
        return;
      j.forEach(function(S) {
        S.open = !1;
      }), j = null;
    }
    window.addEventListener("beforeprint", R), window.addEventListener("afterprint", c);
    function m(S, h, _) {
      Y.textContent = h;
      var G = S.querySelector(".action-label"), Z = G ? G.textContent : "";
      if (G)
        G.textContent = _ ? "Copied" : "Failed";
      S.classList.add(_ ? "is-copied" : "is-failed"), S.disabled = !0, setTimeout(function() {
        if (S.classList.remove("is-copied", "is-failed"), S.disabled = !1, G)
          G.textContent = Z;
      }, 1200);
    }
    function y(S, h) {
      if (!navigator.clipboard) {
        m(h, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(S).then(function() {
        m(h, "Copied!", !0);
      }, function() {
        m(h, "Copy failed", !1);
      });
    }
    var P = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', l = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', u = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', q = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', F = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', W = E.getElementById("pages"), X = E.querySelector(".pages-trigger"), z = null, KE = window.matchMedia("(max-width: 78rem)");
    if (W && X) {
      var AE = W.querySelector(".pages-list");
      if (AE) {
        var ME = E.createElement("dialog");
        if (typeof ME.showModal === "function") {
          let S = function() {
            if (z && z.open)
              z.close();
          };
          z = ME, z.className = "pages-dialog", z.id = "pages-dialog", z.setAttribute("aria-labelledby", "pages-dialog-title");
          var d = E.createElement("div");
          d.className = "pages-dialog-panel";
          var r = E.createElement("div");
          r.className = "pages-dialog-header";
          var t = E.createElement("h2");
          t.className = "pages-dialog-title", t.id = "pages-dialog-title", t.textContent = "Pages";
          var L = E.createElement("button");
          L.className = "pages-dialog-close", L.type = "button", L.setAttribute("aria-label", "Close pages"), L.innerHTML = F, r.appendChild(t), r.appendChild(L);
          var T = E.createElement("nav");
          T.className = "pages-dialog-nav", T.setAttribute("aria-label", "Pages"), T.appendChild(AE.cloneNode(!0)), d.appendChild(r), d.appendChild(T), z.appendChild(d), X.addEventListener("click", function() {
            z.showModal(), X.setAttribute("aria-expanded", "true"), E.body.classList.add("pages-dialog-open");
            var h = z.querySelector('[aria-current="page"]');
            if (h)
              h.scrollIntoView({ block: "nearest" });
          }), L.addEventListener("click", S), z.addEventListener("click", function(h) {
            if (h.target === z)
              S();
          }), z.addEventListener("keydown", function(h) {
            if (h.key === "Escape")
              h.preventDefault(), S();
          }), z.addEventListener("close", function() {
            if (X.setAttribute("aria-expanded", "false"), E.body.classList.remove("pages-dialog-open"), KE.matches)
              setTimeout(function() {
                X.focus();
              }, 50);
          }), T.querySelectorAll("a").forEach(function(h) {
            h.addEventListener("click", S);
          }), X.hidden = !1, X.setAttribute("aria-expanded", "false"), E.body.appendChild(z), E.body.classList.add("pages-dialog-ready");
        }
      }
    }
    var g = E.getElementById("source"), qE = E.getElementById("changes"), U = E.getElementById("toc"), J = null, B = null, BE = window.matchMedia("(max-width: 78rem)");
    function k() {
      if (B && B.open)
        B.close();
    }
    function b() {
      if (!U || !J || !B)
        return;
      var S = BE.matches && !I.hidden && U.getBoundingClientRect().bottom < 0 && !B.open;
      if (J.classList.toggle("is-visible", S), J.tabIndex = S ? 0 : -1, J.setAttribute("aria-hidden", S ? "false" : "true"), B.open && (!BE.matches || I.hidden))
        k();
    }
    if (E.querySelectorAll(".viewtoggle button").forEach(function(S) {
      S.addEventListener("click", function() {
        var h = S.dataset.view;
        if (I.hidden = h !== "rendered", g)
          g.hidden = h !== "source";
        if (qE)
          qE.hidden = h !== "changes";
        if (U)
          U.hidden = h !== "rendered";
        E.querySelectorAll(".viewtoggle button").forEach(function(_) {
          _.classList.toggle("active", _ === S), _.setAttribute("aria-pressed", _ === S ? "true" : "false");
        }), b();
      });
    }), U) {
      let S = function() {
        if (D.length === 0) {
          b();
          return;
        }
        var _ = 0;
        if (QE.forEach(function(Z, V) {
          if (Z && Z.getBoundingClientRect().top <= 128)
            _ = V;
        }), window.innerHeight + window.scrollY >= E.documentElement.scrollHeight - 2)
          _ = D.length - 1;
        var G = D[_].getAttribute("href");
        SE.forEach(function(Z) {
          var V = Z.getAttribute("href") === G;
          if (Z.classList.toggle("active", V), V)
            Z.setAttribute("aria-current", "location");
          else
            Z.removeAttribute("aria-current");
        }), b();
      }, h = function() {
        if (_E)
          return;
        _E = !0, window.requestAnimationFrame(function() {
          _E = !1, S();
        });
      };
      var D = Array.from(U.querySelectorAll('a[href^="#"]')), GE = U.querySelector(".toc-list");
      if (GE)
        if (B = E.createElement("dialog"), typeof B.showModal === "function") {
          B.className = "toc-dialog", B.id = "toc-dialog", B.setAttribute("aria-labelledby", "toc-dialog-title");
          var i = E.createElement("div");
          i.className = "toc-dialog-panel";
          var o = E.createElement("div");
          o.className = "toc-dialog-header";
          var s = E.createElement("h2");
          s.className = "toc-dialog-title", s.id = "toc-dialog-title", s.textContent = "Contents";
          var C = E.createElement("button");
          C.className = "toc-dialog-close", C.type = "button", C.setAttribute("aria-label", "Close table of contents"), C.innerHTML = F, o.appendChild(s), o.appendChild(C);
          var n = E.createElement("nav");
          n.className = "toc-dialog-nav", n.setAttribute("aria-label", "Table of contents"), n.appendChild(GE.cloneNode(!0)), i.appendChild(o), i.appendChild(n), B.appendChild(i), J = E.createElement("button"), J.className = "toc-trigger", J.type = "button", J.tabIndex = -1, J.setAttribute("aria-label", "Open table of contents"), J.setAttribute("aria-controls", "toc-dialog"), J.setAttribute("aria-haspopup", "dialog"), J.setAttribute("aria-hidden", "true"), J.innerHTML = q, E.body.appendChild(J), E.body.appendChild(B), J.addEventListener("click", function() {
            B.showModal(), E.body.classList.add("toc-dialog-open"), b();
            var _ = B.querySelector("a.active");
            if (_)
              _.scrollIntoView({ block: "nearest" });
          }), C.addEventListener("click", k), B.addEventListener("click", function(_) {
            if (_.target === B)
              k();
          }), B.addEventListener("keydown", function(_) {
            if (_.key === "Escape")
              _.preventDefault(), k();
          }), B.addEventListener("close", function() {
            if (E.body.classList.remove("toc-dialog-open"), b(), J.classList.contains("is-visible"))
              setTimeout(function() {
                J.focus();
              }, 50);
          }), n.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", k);
          });
        } else
          B = null;
      var SE = D.slice();
      if (B)
        SE = SE.concat(Array.from(B.querySelectorAll('a[href^="#"]')));
      var QE = D.map(function(_) {
        return E.getElementById((_.getAttribute("href") || "").slice(1));
      }), _E = !1;
      E.addEventListener("scroll", h, { passive: !0 }), window.addEventListener("resize", S), S();
    }
    let hE = E.querySelector(".copy-source");
    if (hE && g)
      hE.addEventListener("click", function() {
        var S = g.querySelector("pre");
        y(S ? S.textContent : "", hE);
      });
    I.querySelectorAll("pre").forEach(function(S) {
      if (S.classList.contains("mermaid"))
        return;
      var h = E.createElement("div");
      h.className = "codewrap", S.parentNode?.insertBefore(h, S), h.appendChild(S);
      var _ = E.createElement("button");
      _.className = "codecopy", _.type = "button", _.setAttribute("aria-label", "Copy code"), _.title = "Copy code", _.innerHTML = P + l + u, _.addEventListener("click", function() {
        var G = S.querySelector("code");
        y((G || S).textContent, _);
      }), h.appendChild(_);
    });
  })();
})();
