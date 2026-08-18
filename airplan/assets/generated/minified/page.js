(() => {
  function BE(E) {
    return E === "system" || E === "light" || E === "dark";
  }
  function r(E, u) {
    try {
      return E?.getItem(u) ?? null;
    } catch {
      return null;
    }
  }
  function L(E, u, G) {
    try {
      if (G === null)
        E?.removeItem(u);
      else
        E?.setItem(u, G);
    } catch {}
  }
  function _E(E, u, G) {
    let H = r(G, "airplan-color-mode");
    if (H === null) {
      let I = r(G, "airplan-theme");
      if (H = I === "light" || I === "dark" ? I : "system", H !== "system")
        L(G, "airplan-color-mode", H);
    }
    let V = BE(H) ? H : "system", Y = new Set(E.themes.map((I) => I.id)), M = r(G, "airplan-light-theme"), J = r(G, "airplan-dark-theme"), j = M !== null && Y.has(M) ? M : E.defaultLight, K = J !== null && Y.has(J) ? J : E.defaultDark;
    return a(E, V, j, K, u);
  }
  function a(E, u, G, H, V) {
    let Y = new Map(E.themes.map((w) => [w.id, w])), M = Y.has(G) ? G : E.defaultLight, J = Y.has(H) ? H : E.defaultDark, j = u === "system" ? V ? "dark" : "light" : u, K = j === "light" ? M : J, I = Y.get(K)?.variant ?? j;
    return { mode: u, resolvedMode: j, lightTheme: M, darkTheme: J, theme: K, variant: I };
  }
  function hE(E, u) {
    if (u === "system")
      L(E, "airplan-color-mode", null), L(E, "airplan-theme", null);
    else
      L(E, "airplan-color-mode", u), L(E, "airplan-theme", u);
  }
  function AE(E, u, G) {
    L(E, u === "light" ? "airplan-light-theme" : "airplan-dark-theme", G);
  }
  function ME(E) {
    return {
      mode: E.mode,
      resolvedMode: E.resolvedMode,
      theme: E.theme,
      variant: E.variant
    };
  }

  (function() {
    let E = document, u = E.documentElement, G = window.__AIRPLAN_THEME_CATALOG__;
    if (!G)
      return;
    let H = G, V = window.matchMedia("(prefers-color-scheme: dark)"), Y;
    try {
      Y = window.localStorage;
    } catch {}
    let M = window.__airplanThemeState ?? _E(H, V.matches, Y);
    E.querySelectorAll(".js-only").forEach((h) => {
      h.hidden = !1;
    });
    let J = E.querySelector("[data-airplan-appearance-trigger]"), j = E.querySelector("[data-airplan-appearance-panel]"), K = E.querySelector('select[data-airplan-theme-slot="light"]'), I = E.querySelector('select[data-airplan-theme-slot="dark"]'), w = Array.from(E.querySelectorAll("[data-airplan-color-mode]"));
    function b(h) {
      if (!h || h.options.length > 0)
        return;
      for (let [O, C] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let $ = E.createElement("optgroup");
        $.label = C;
        for (let x of H.themes) {
          if (x.variant !== O)
            continue;
          let X = E.createElement("option");
          X.value = x.id, X.textContent = x.name, $.append(X);
        }
        h.append($);
      }
    }
    b(K), b(I);
    function F(h, O = !0) {
      if (M = h, window.__airplanThemeState = M, u.dataset.airplanMode = M.mode, u.dataset.airplanResolvedMode = M.resolvedMode, u.dataset.airplanTheme = M.theme, u.dataset.airplanThemeVariant = M.variant, w.forEach((C) => {
        let $ = C.dataset.airplanColorMode === M.mode;
        C.classList.toggle("active", $), C.setAttribute("aria-pressed", String($));
      }), K)
        K.value = M.lightTheme;
      if (I)
        I.value = M.darkTheme;
      if (O)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: ME(M) }));
    }
    function U(h = {}) {
      F(a(H, h.mode ?? M.mode, h.lightTheme ?? M.lightTheme, h.darkTheme ?? M.darkTheme, V.matches));
    }
    function y(h, O = !1) {
      if (!j || !J)
        return;
      if (j.hidden = !h, J.setAttribute("aria-expanded", String(h)), h)
        j.querySelector("button,select")?.focus();
      else if (O)
        J.focus();
    }
    J?.addEventListener("click", () => y(Boolean(j?.hidden ?? !0))), w.forEach((h) => h.addEventListener("click", () => {
      let O = h.dataset.airplanColorMode;
      if (!O)
        return;
      hE(Y, O), U({ mode: O });
    }));
    function v(h, O) {
      AE(Y, h, O.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: O.value } })), U(h === "light" ? { lightTheme: O.value } : { darkTheme: O.value });
    }
    K?.addEventListener("change", () => v("light", K)), I?.addEventListener("change", () => v("dark", I)), V.addEventListener("change", () => {
      if (M.mode === "system")
        U();
    }), E.addEventListener("keydown", (h) => {
      if (h.key === "Escape" && j && !j.hidden)
        h.preventDefault(), y(!1, !0);
    }), E.addEventListener("pointerdown", (h) => {
      if (!j || j.hidden || !J)
        return;
      let O = h.target;
      if (!j.contains(O) && !J.contains(O))
        y(!1);
    }), F(M, !1);
  })();

  (function() {
    var E = document;
    let u = E.getElementById("rendered");
    if (!u)
      return;
    let G = u;
    var H = E.querySelector('meta[name="airplan-versions"]'), V = window.location.pathname.split("/").filter(Boolean), Y = V.slice(0, -2);
    function M(S, f) {
      if (typeof S !== "string")
        return null;
      try {
        var _ = new URL(S);
        if (_.origin !== window.location.origin || _.username || _.password || _.search || _.hash)
          return null;
        var B = _.pathname.split("/").filter(Boolean);
        if (B.length !== Y.length + 2 || !Y.every(function(Z, c) {
          return B[c] === Z;
        }) || !/^[a-z2-7]{26}$/.test(B[B.length - 2]))
          return null;
        var Q = B[B.length - 1];
        if (f ? Q !== ".airplan-changes.diff" : !Q.endsWith(".html"))
          return null;
        return _.href;
      } catch {
        return null;
      }
    }
    function J(S) {
      var f = E.querySelector('meta[name="airplan-revision"]'), _ = f ? Number(f.content) : Number(S.current_revision);
      if (!Number.isInteger(_) || _ <= 0 || S.current_revision !== _ || !Number.isInteger(S.latest_revision) || !Number.isInteger(S.last_assigned_revision) || !Array.isArray(S.revisions) || S.revisions.length === 0 || S.last_assigned_revision !== S.revisions.length || !/^[a-z2-7]{26}$/.test(S.chain_id))
        throw Error("revision identity is invalid");
      var B = !1, Q = 0, Z = S.revisions.filter(function(A) {
        if (!A || !Number.isInteger(A.number) || A.number !== Q + 1)
          return B = !0, !1;
        if (Q = A.number, A.deleted)
          return !1;
        if (A.safeURL = M(A.url, !1), !A.safeURL)
          return B = !0, !1;
        if (A.number > 1) {
          var R = M(A.diff_url, !0);
          if (!R || new URL(R).pathname.replace(/[^/]+$/, "") !== new URL(A.safeURL).pathname.replace(/[^/]+$/, ""))
            return B = !0, !1;
        }
        return !0;
      });
      if (B || S.revisions[0].number !== 1 || !Z.some(function(A) {
        return A.number === _;
      }))
        throw Error("revision entries are invalid");
      var c = Z.find(function(A) {
        return A.number === _;
      }), qE = window.location.origin + window.location.pathname;
      if (!c || c.safeURL !== qE)
        throw Error("current revision URL is invalid");
      var k = Math.max.apply(null, Z.map(function(A) {
        return A.number;
      }));
      if (k !== S.latest_revision)
        throw Error("latest is invalid");
      var W = E.querySelector("[data-revision-heading]");
      if (!W) {
        W = E.createElement("p"), W.className = "revision-heading", W.setAttribute("data-revision-heading", "");
        var SE = E.getElementById("rendered");
        if (!SE)
          throw Error("rendered view is unavailable");
        SE.prepend(W);
      }
      var o = _ < k, fE = o ? "Revision " + _ + " of " + k : "Revision " + _ + " (Latest)", d = E.createElement("span");
      d.className = "revision-picker-label", d.textContent = fE, d.setAttribute("aria-hidden", "true");
      var D = E.createElement("select");
      D.setAttribute("aria-label", "Document revision"), Z.forEach(function(A) {
        var R = E.createElement("option");
        R.value = A.safeURL || "", R.textContent = A.number === k ? "Revision " + A.number + " (Latest)" : "Revision " + A.number + " of " + k, R.selected = A.number === _, D.appendChild(R);
      }), D.addEventListener("change", function() {
        var A = D.selectedIndex;
        if (A < 0 || A >= Z.length)
          return;
        window.location.assign(Z[A].safeURL || "");
      }), W.replaceChildren(d, D), W.classList.add("is-picker"), W.classList.toggle("is-stale", o), E.body.classList.toggle("airplan-stale-revision", o);
    }
    if (H) {
      var j = new URL(H.content, window.location.href);
      j.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(j, { cache: "no-store", credentials: "same-origin" }).then(function(S) {
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
        J(S), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: S
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var K = E.createElement("div");
    K.className = "sr-status", K.setAttribute("aria-live", "polite"), E.body.appendChild(K);
    var I = null;
    function w() {
      if (I !== null)
        return;
      I = Array.from(E.querySelectorAll("details:not([open])")), I.forEach(function(S) {
        S.open = !0;
      });
    }
    function b() {
      if (I === null)
        return;
      I.forEach(function(S) {
        S.open = !1;
      }), I = null;
    }
    window.addEventListener("beforeprint", w), window.addEventListener("afterprint", b);
    function F(S, f, _) {
      K.textContent = f;
      var B = S.querySelector(".action-label"), Q = B ? B.textContent : "";
      if (B)
        B.textContent = _ ? "Copied" : "Failed";
      S.classList.add(_ ? "is-copied" : "is-failed"), S.disabled = !0, setTimeout(function() {
        if (S.classList.remove("is-copied", "is-failed"), S.disabled = !1, B)
          B.textContent = Q;
      }, 1200);
    }
    function U(S, f) {
      if (!navigator.clipboard) {
        F(f, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(S).then(function() {
        F(f, "Copied!", !0);
      }, function() {
        F(f, "Copy failed", !1);
      });
    }
    var y = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', v = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', h = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', O = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', C = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', $ = E.getElementById("source"), x = E.getElementById("changes"), X = E.getElementById("toc"), z = null, q = null, e = window.matchMedia("(max-width: 78rem)");
    function P() {
      if (q && q.open)
        q.close();
    }
    function p() {
      if (!X || !z || !q)
        return;
      var S = e.matches && !G.hidden && X.getBoundingClientRect().bottom < 0 && !q.open;
      if (z.classList.toggle("is-visible", S), z.tabIndex = S ? 0 : -1, z.setAttribute("aria-hidden", S ? "false" : "true"), q.open && (!e.matches || G.hidden))
        P();
    }
    if (E.querySelectorAll(".viewtoggle button").forEach(function(S) {
      S.addEventListener("click", function() {
        var f = S.dataset.view;
        if (G.hidden = f !== "rendered", $)
          $.hidden = f !== "source";
        if (x)
          x.hidden = f !== "changes";
        if (X)
          X.hidden = f !== "rendered";
        E.querySelectorAll(".viewtoggle button").forEach(function(_) {
          _.classList.toggle("active", _ === S), _.setAttribute("aria-pressed", _ === S ? "true" : "false");
        }), p();
      });
    }), X) {
      let S = function() {
        if (m.length === 0) {
          p();
          return;
        }
        var _ = 0;
        if (uE.forEach(function(Q, Z) {
          if (Q && Q.getBoundingClientRect().top <= 128)
            _ = Z;
        }), window.innerHeight + window.scrollY >= E.documentElement.scrollHeight - 2)
          _ = m.length - 1;
        var B = m[_].getAttribute("href");
        i.forEach(function(Q) {
          var Z = Q.getAttribute("href") === B;
          if (Q.classList.toggle("active", Z), Z)
            Q.setAttribute("aria-current", "location");
          else
            Q.removeAttribute("aria-current");
        }), p();
      }, f = function() {
        if (s)
          return;
        s = !0, window.requestAnimationFrame(function() {
          s = !1, S();
        });
      };
      var m = Array.from(X.querySelectorAll('a[href^="#"]')), EE = X.querySelector(".toc-list");
      if (EE)
        if (q = E.createElement("dialog"), typeof q.showModal === "function") {
          q.className = "toc-dialog", q.id = "toc-dialog", q.setAttribute("aria-labelledby", "toc-dialog-title");
          var n = E.createElement("div");
          n.className = "toc-dialog-panel";
          var l = E.createElement("div");
          l.className = "toc-dialog-header";
          var g = E.createElement("h2");
          g.className = "toc-dialog-title", g.id = "toc-dialog-title", g.textContent = "Contents";
          var N = E.createElement("button");
          N.className = "toc-dialog-close", N.type = "button", N.setAttribute("aria-label", "Close table of contents"), N.innerHTML = C, l.appendChild(g), l.appendChild(N);
          var T = E.createElement("nav");
          T.className = "toc-dialog-nav", T.setAttribute("aria-label", "Table of contents"), T.appendChild(EE.cloneNode(!0)), n.appendChild(l), n.appendChild(T), q.appendChild(n), z = E.createElement("button"), z.className = "toc-trigger", z.type = "button", z.tabIndex = -1, z.setAttribute("aria-label", "Open table of contents"), z.setAttribute("aria-controls", "toc-dialog"), z.setAttribute("aria-haspopup", "dialog"), z.setAttribute("aria-hidden", "true"), z.innerHTML = O, E.body.appendChild(z), E.body.appendChild(q), z.addEventListener("click", function() {
            q.showModal(), E.body.classList.add("toc-dialog-open"), p();
            var _ = q.querySelector("a.active");
            if (_)
              _.scrollIntoView({ block: "nearest" });
          }), N.addEventListener("click", P), q.addEventListener("click", function(_) {
            if (_.target === q)
              P();
          }), q.addEventListener("keydown", function(_) {
            if (_.key === "Escape")
              _.preventDefault(), P();
          }), q.addEventListener("close", function() {
            if (E.body.classList.remove("toc-dialog-open"), p(), z.classList.contains("is-visible"))
              setTimeout(function() {
                z.focus();
              }, 50);
          }), T.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", P);
          });
        } else
          q = null;
      var i = m.slice();
      if (q)
        i = i.concat(Array.from(q.querySelectorAll('a[href^="#"]')));
      var uE = m.map(function(_) {
        return E.getElementById((_.getAttribute("href") || "").slice(1));
      }), s = !1;
      E.addEventListener("scroll", f, { passive: !0 }), window.addEventListener("resize", S), S();
    }
    let t = E.querySelector(".copy-source");
    if (t && $)
      t.addEventListener("click", function() {
        var S = $.querySelector("pre");
        U(S ? S.textContent : "", t);
      });
    G.querySelectorAll("pre").forEach(function(S) {
      if (S.classList.contains("mermaid"))
        return;
      var f = E.createElement("div");
      f.className = "codewrap", S.parentNode?.insertBefore(f, S), f.appendChild(S);
      var _ = E.createElement("button");
      _.className = "codecopy", _.type = "button", _.setAttribute("aria-label", "Copy code"), _.title = "Copy code", _.innerHTML = y + v + h, _.addEventListener("click", function() {
        var B = S.querySelector("code");
        U((B || S).textContent, _);
      }), f.appendChild(_);
    });
  })();
})();
