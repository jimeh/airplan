(() => {
  function $E(E) {
    return E === "system" || E === "light" || E === "dark";
  }
  function EE(E, A) {
    try {
      return E?.getItem(A) ?? null;
    } catch {
      return null;
    }
  }
  function N(E, A, I) {
    try {
      if (I === null)
        E?.removeItem(A);
      else
        E?.setItem(A, I);
    } catch {}
  }
  function jE(E, A, I) {
    let K = EE(I, "airplan-color-mode");
    if (K === null) {
      let j = EE(I, "airplan-theme");
      if (K = j === "light" || j === "dark" ? j : "system", K !== "system")
        N(I, "airplan-color-mode", K);
    }
    let w = $E(K) ? K : "system", $ = new Set(E.themes.map((j) => j.id)), f = EE(I, "airplan-light-theme"), Q = EE(I, "airplan-dark-theme"), O = f !== null && $.has(f) ? f : E.defaultLight, Y = Q !== null && $.has(Q) ? Q : E.defaultDark;
    return AE(E, w, O, Y, A);
  }
  function AE(E, A, I, K, w) {
    let $ = new Map(E.themes.map((R) => [R.id, R])), f = $.has(I) ? I : E.defaultLight, Q = $.has(K) ? K : E.defaultDark, O = A === "system" ? w ? "dark" : "light" : A, Y = O === "light" ? f : Q, j = $.get(Y)?.variant ?? O;
    return { mode: A, resolvedMode: O, lightTheme: f, darkTheme: Q, theme: Y, variant: j };
  }
  function zE(E, A) {
    if (A === "system")
      N(E, "airplan-color-mode", null), N(E, "airplan-theme", null);
    else
      N(E, "airplan-color-mode", A), N(E, "airplan-theme", A);
  }
  function JE(E, A, I) {
    N(E, A === "light" ? "airplan-light-theme" : "airplan-dark-theme", I);
  }
  function KE(E) {
    return {
      mode: E.mode,
      resolvedMode: E.resolvedMode,
      theme: E.theme,
      variant: E.variant
    };
  }

  (function() {
    let E = document, A = E.documentElement;
    E.querySelectorAll(".js-only").forEach((h) => {
      h.hidden = !1;
    });
    let I = window.__AIRPLAN_THEME_CATALOG__;
    if (!I)
      return;
    let K = I, w = window.matchMedia("(prefers-color-scheme: dark)"), $;
    try {
      $ = window.localStorage;
    } catch {}
    let f = window.__airplanThemeState ?? jE(K, w.matches, $), Q = E.querySelector("[data-airplan-appearance-trigger]"), O = E.querySelector("[data-airplan-appearance-panel]"), Y = E.querySelector('select[data-airplan-theme-slot="light"]'), j = E.querySelector('select[data-airplan-theme-slot="dark"]'), R = Array.from(E.querySelectorAll("[data-airplan-color-mode]"));
    function c(h) {
      if (!h || h.options.length > 0)
        return;
      for (let [M, F] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let W = E.createElement("optgroup");
        W.label = F;
        for (let X of K.themes) {
          if (X.variant !== M)
            continue;
          let z = E.createElement("option");
          z.value = X.id, z.textContent = X.name, W.append(z);
        }
        if (W.children.length > 0)
          h.append(W);
      }
    }
    c(Y), c(j);
    function m(h, M = !0) {
      if (f = h, window.__airplanThemeState = f, A.dataset.airplanMode = f.mode, A.dataset.airplanResolvedMode = f.resolvedMode, A.dataset.airplanTheme = f.theme, A.dataset.airplanThemeVariant = f.variant, R.forEach((F) => {
        let W = F.dataset.airplanColorMode === f.mode;
        F.classList.toggle("active", W), F.setAttribute("aria-pressed", String(W));
      }), Y)
        Y.value = f.lightTheme;
      if (j)
        j.value = f.darkTheme;
      if (M)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: KE(f) }));
    }
    function L(h = {}) {
      m(AE(K, h.mode ?? f.mode, h.lightTheme ?? f.lightTheme, h.darkTheme ?? f.darkTheme, w.matches));
    }
    function P(h, M = !1) {
      if (!O || !Q)
        return;
      if (O.hidden = !h, Q.setAttribute("aria-expanded", String(h)), h)
        O.querySelector("button,select")?.focus();
      else if (M)
        Q.focus();
    }
    Q?.addEventListener("click", () => P(Boolean(O?.hidden ?? !0))), R.forEach((h) => h.addEventListener("click", () => {
      let M = h.dataset.airplanColorMode;
      if (!M)
        return;
      zE($, M), L({ mode: M });
    }));
    function l(h, M) {
      JE($, h, M.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: M.value } })), L(h === "light" ? { lightTheme: M.value } : { darkTheme: M.value });
    }
    Y?.addEventListener("change", () => l("light", Y)), j?.addEventListener("change", () => l("dark", j)), w.addEventListener("change", () => {
      if (f.mode === "system")
        L();
    }), E.addEventListener("keydown", (h) => {
      if (h.key === "Escape" && O && !O.hidden)
        h.preventDefault(), P(!1, !0);
    }), E.addEventListener("pointerdown", (h) => {
      if (!O || O.hidden || !Q)
        return;
      let M = h.target;
      if (!(M instanceof Node) || O.contains(M) || Q.contains(M))
        return;
      let W = (M instanceof Element ? M : M.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), X = O.contains(E.activeElement) && !W;
      if (P(!1), X)
        setTimeout(() => {
          if (E.activeElement === E.body || O.contains(E.activeElement))
            Q.focus();
        });
    }), m(f, !1);
  })();

  (function() {
    var E = document;
    let A = E.getElementById("rendered");
    if (!A)
      return;
    let I = A;
    var K = E.querySelector('meta[name="airplan-versions"]'), w = window.location.pathname.split("/").filter(Boolean), $ = w.slice(0, -2);
    function f(S, q) {
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
        if (q ? Z !== ".airplan-changes.diff" : !Z.endsWith(".html"))
          return null;
        return _.href;
      } catch {
        return null;
      }
    }
    function Q(S) {
      var q = E.querySelector('meta[name="airplan-revision"]'), _ = q ? Number(q.content) : Number(S.current_revision);
      if (!Number.isInteger(_) || _ <= 0 || S.current_revision !== _ || !Number.isInteger(S.latest_revision) || !Number.isInteger(S.last_assigned_revision) || !Array.isArray(S.revisions) || S.revisions.length === 0 || S.last_assigned_revision !== S.revisions.length || !/^[a-z2-7]{26}$/.test(S.chain_id))
        throw Error("revision identity is invalid");
      var G = !1, Z = 0, V = S.revisions.filter(function(u) {
        if (!u || !Number.isInteger(u.number) || u.number !== Z + 1)
          return G = !0, !1;
        if (Z = u.number, u.deleted)
          return !1;
        if (u.safeURL = f(u.url, !1), !u.safeURL)
          return G = !0, !1;
        if (u.number > 1) {
          var H = f(u.diff_url, !0);
          if (!H || new URL(H).pathname.replace(/[^/]+$/, "") !== new URL(u.safeURL).pathname.replace(/[^/]+$/, ""))
            return G = !0, !1;
        }
        return !0;
      });
      if (G || S.revisions[0].number !== 1 || !V.some(function(u) {
        return u.number === _;
      }))
        throw Error("revision entries are invalid");
      var a = V.find(function(u) {
        return u.number === _;
      }), YE = window.location.origin + window.location.pathname;
      if (!a || a.safeURL !== YE)
        throw Error("current revision URL is invalid");
      var v = Math.max.apply(null, V.map(function(u) {
        return u.number;
      }));
      if (v !== S.latest_revision)
        throw Error("latest is invalid");
      var x = E.querySelector("[data-revision-heading]");
      if (!x) {
        x = E.createElement("p"), x.className = "revision-heading", x.setAttribute("data-revision-heading", "");
        var OE = E.getElementById("rendered");
        if (!OE)
          throw Error("rendered view is unavailable");
        OE.prepend(x);
      }
      var fE = _ < v, ZE = fE ? "Revision " + _ + " of " + v : "Revision " + _ + " (Latest)", e = E.createElement("span");
      e.className = "revision-picker-label", e.textContent = ZE, e.setAttribute("aria-hidden", "true");
      var p = E.createElement("select");
      p.setAttribute("aria-label", "Document revision"), V.forEach(function(u) {
        var H = E.createElement("option");
        H.value = u.safeURL || "", H.textContent = u.number === v ? "Revision " + u.number + " (Latest)" : "Revision " + u.number + " of " + v, H.selected = u.number === _, p.appendChild(H);
      }), p.addEventListener("change", function() {
        var u = p.selectedIndex;
        if (u < 0 || u >= V.length)
          return;
        window.location.assign(V[u].safeURL || "");
      }), x.replaceChildren(e, p), x.classList.add("is-picker"), x.classList.toggle("is-stale", fE), E.body.classList.toggle("airplan-stale-revision", fE);
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
    function m(S, q, _) {
      Y.textContent = q;
      var G = S.querySelector(".action-label"), Z = G ? G.textContent : "";
      if (G)
        G.textContent = _ ? "Copied" : "Failed";
      S.classList.add(_ ? "is-copied" : "is-failed"), S.disabled = !0, setTimeout(function() {
        if (S.classList.remove("is-copied", "is-failed"), S.disabled = !1, G)
          G.textContent = Z;
      }, 1200);
    }
    function L(S, q) {
      if (!navigator.clipboard) {
        m(q, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(S).then(function() {
        m(q, "Copied!", !0);
      }, function() {
        m(q, "Copy failed", !1);
      });
    }
    var P = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', l = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', h = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', M = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', F = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', W = E.getElementById("pages"), X = E.querySelector(".pages-trigger"), z = null, SE = window.matchMedia("(max-width: 78rem)");
    if (W && X) {
      var qE = W.querySelector(".pages-list");
      if (qE) {
        var ME = E.createElement("dialog");
        if (typeof ME.showModal === "function") {
          let S = function() {
            if (z && z.open)
              z.close();
          }, q = function() {
            if (!SE.matches)
              S();
          };
          z = ME, z.className = "pages-dialog", z.id = "pages-dialog", z.setAttribute("aria-labelledby", "pages-dialog-title");
          var d = E.createElement("div");
          d.className = "pages-dialog-panel";
          var r = E.createElement("div");
          r.className = "pages-dialog-header";
          var t = E.createElement("h2");
          t.className = "pages-dialog-title", t.id = "pages-dialog-title", t.textContent = "Pages";
          var y = E.createElement("button");
          y.className = "pages-dialog-close", y.type = "button", y.setAttribute("aria-label", "Close pages"), y.innerHTML = F, r.appendChild(t), r.appendChild(y);
          var T = E.createElement("nav");
          T.className = "pages-dialog-nav", T.setAttribute("aria-label", "Pages"), T.appendChild(qE.cloneNode(!0)), d.appendChild(r), d.appendChild(T), z.appendChild(d), X.addEventListener("click", function() {
            z.showModal(), X.setAttribute("aria-expanded", "true"), E.body.classList.add("pages-dialog-open");
            var _ = z.querySelector('[aria-current="page"]');
            if (_)
              _.scrollIntoView({ block: "nearest" });
          }), y.addEventListener("click", S), z.addEventListener("click", function(_) {
            if (_.target === z)
              S();
          }), z.addEventListener("keydown", function(_) {
            if (_.key === "Escape")
              _.preventDefault(), S();
          }), z.addEventListener("close", function() {
            if (X.setAttribute("aria-expanded", "false"), E.body.classList.remove("pages-dialog-open"), SE.matches)
              setTimeout(function() {
                X.focus();
              }, 50);
          }), T.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", S);
          }), SE.addEventListener("change", q), X.hidden = !1, X.setAttribute("aria-expanded", "false"), E.body.appendChild(z), E.body.classList.add("pages-dialog-ready");
        }
      }
    }
    var i = E.getElementById("source"), BE = E.getElementById("changes"), U = E.getElementById("toc"), J = null, B = null, GE = window.matchMedia("(max-width: 78rem)");
    function k() {
      if (B && B.open)
        B.close();
    }
    function b() {
      if (!U || !J || !B)
        return;
      var S = GE.matches && !I.hidden && U.getBoundingClientRect().bottom < 0 && !B.open;
      if (J.classList.toggle("is-visible", S), J.tabIndex = S ? 0 : -1, J.setAttribute("aria-hidden", S ? "false" : "true"), B.open && (!GE.matches || I.hidden))
        k();
    }
    if (E.querySelectorAll(".viewtoggle button").forEach(function(S) {
      S.addEventListener("click", function() {
        var q = S.dataset.view;
        if (I.hidden = q !== "rendered", i)
          i.hidden = q !== "source";
        if (BE)
          BE.hidden = q !== "changes";
        if (U)
          U.hidden = q !== "rendered";
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
        _E.forEach(function(Z) {
          var V = Z.getAttribute("href") === G;
          if (Z.classList.toggle("active", V), V)
            Z.setAttribute("aria-current", "location");
          else
            Z.removeAttribute("aria-current");
        }), b();
      }, q = function() {
        if (hE)
          return;
        hE = !0, window.requestAnimationFrame(function() {
          hE = !1, S();
        });
      };
      var D = Array.from(U.querySelectorAll('a[href^="#"]')), IE = U.querySelector(".toc-list");
      if (IE)
        if (B = E.createElement("dialog"), typeof B.showModal === "function") {
          B.className = "toc-dialog", B.id = "toc-dialog", B.setAttribute("aria-labelledby", "toc-dialog-title");
          var g = E.createElement("div");
          g.className = "toc-dialog-panel";
          var o = E.createElement("div");
          o.className = "toc-dialog-header";
          var s = E.createElement("h2");
          s.className = "toc-dialog-title", s.id = "toc-dialog-title", s.textContent = "Contents";
          var C = E.createElement("button");
          C.className = "toc-dialog-close", C.type = "button", C.setAttribute("aria-label", "Close table of contents"), C.innerHTML = F, o.appendChild(s), o.appendChild(C);
          var n = E.createElement("nav");
          n.className = "toc-dialog-nav", n.setAttribute("aria-label", "Table of contents"), n.appendChild(IE.cloneNode(!0)), g.appendChild(o), g.appendChild(n), B.appendChild(g), J = E.createElement("button"), J.className = "toc-trigger", J.type = "button", J.tabIndex = -1, J.setAttribute("aria-label", "Open table of contents"), J.setAttribute("aria-controls", "toc-dialog"), J.setAttribute("aria-haspopup", "dialog"), J.setAttribute("aria-hidden", "true"), J.innerHTML = M, E.body.appendChild(J), E.body.appendChild(B), J.addEventListener("click", function() {
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
      var _E = D.slice();
      if (B)
        _E = _E.concat(Array.from(B.querySelectorAll('a[href^="#"]')));
      var QE = D.map(function(_) {
        return E.getElementById((_.getAttribute("href") || "").slice(1));
      }), hE = !1;
      E.addEventListener("scroll", q, { passive: !0 }), window.addEventListener("resize", S), S();
    }
    let uE = E.querySelector(".copy-source");
    if (uE && i)
      uE.addEventListener("click", function() {
        var S = i.querySelector("pre");
        L(S ? S.textContent : "", uE);
      });
    I.querySelectorAll("pre").forEach(function(S) {
      if (S.classList.contains("mermaid"))
        return;
      var q = E.createElement("div");
      q.className = "codewrap", S.parentNode?.insertBefore(q, S), q.appendChild(S);
      var _ = E.createElement("button");
      _.className = "codecopy", _.type = "button", _.setAttribute("aria-label", "Copy code"), _.title = "Copy code", _.innerHTML = P + l + h, _.addEventListener("click", function() {
        var G = S.querySelector("code");
        L((G || S).textContent, _);
      }), q.appendChild(_);
    });
  })();
})();
