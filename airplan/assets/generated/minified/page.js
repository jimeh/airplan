(() => {
  function BE(E) {
    return E === "system" || E === "light" || E === "dark";
  }
  function i(E, A) {
    try {
      return E?.getItem(A) ?? null;
    } catch {
      return null;
    }
  }
  function L(E, A, I) {
    try {
      if (I === null)
        E?.removeItem(A);
      else
        E?.setItem(A, I);
    } catch {}
  }
  function _E(E, A, I) {
    let H = i(I, "airplan-color-mode");
    if (H === null) {
      let j = i(I, "airplan-theme");
      if (H = j === "light" || j === "dark" ? j : "system", H !== "system")
        L(I, "airplan-color-mode", H);
    }
    let V = BE(H) ? H : "system", Y = new Set(E.themes.map((j) => j.id)), f = i(I, "airplan-light-theme"), J = i(I, "airplan-dark-theme"), O = f !== null && Y.has(f) ? f : E.defaultLight, K = J !== null && Y.has(J) ? J : E.defaultDark;
    return a(E, V, O, K, A);
  }
  function a(E, A, I, H, V) {
    let Y = new Map(E.themes.map((x) => [x.id, x])), f = Y.has(I) ? I : E.defaultLight, J = Y.has(H) ? H : E.defaultDark, O = A === "system" ? V ? "dark" : "light" : A, K = O === "light" ? f : J, j = Y.get(K)?.variant ?? O;
    return { mode: A, resolvedMode: O, lightTheme: f, darkTheme: J, theme: K, variant: j };
  }
  function hE(E, A) {
    if (A === "system")
      L(E, "airplan-color-mode", null), L(E, "airplan-theme", null);
    else
      L(E, "airplan-color-mode", A), L(E, "airplan-theme", A);
  }
  function uE(E, A, I) {
    L(E, A === "light" ? "airplan-light-theme" : "airplan-dark-theme", I);
  }
  function fE(E) {
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
    let H = I, V = window.matchMedia("(prefers-color-scheme: dark)"), Y;
    try {
      Y = window.localStorage;
    } catch {}
    let f = window.__airplanThemeState ?? _E(H, V.matches, Y), J = E.querySelector("[data-airplan-appearance-trigger]"), O = E.querySelector("[data-airplan-appearance-panel]"), K = E.querySelector('select[data-airplan-theme-slot="light"]'), j = E.querySelector('select[data-airplan-theme-slot="dark"]'), x = Array.from(E.querySelectorAll("[data-airplan-color-mode]"));
    function n(h) {
      if (!h || h.options.length > 0)
        return;
      for (let [M, W] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let Z = E.createElement("optgroup");
        Z.label = W;
        for (let w of H.themes) {
          if (w.variant !== M)
            continue;
          let X = E.createElement("option");
          X.value = w.id, X.textContent = w.name, Z.append(X);
        }
        if (Z.children.length > 0)
          h.append(Z);
      }
    }
    n(K), n(j);
    function R(h, M = !0) {
      if (f = h, window.__airplanThemeState = f, A.dataset.airplanMode = f.mode, A.dataset.airplanResolvedMode = f.resolvedMode, A.dataset.airplanTheme = f.theme, A.dataset.airplanThemeVariant = f.variant, x.forEach((W) => {
        let Z = W.dataset.airplanColorMode === f.mode;
        W.classList.toggle("active", Z), W.setAttribute("aria-pressed", String(Z));
      }), K)
        K.value = f.lightTheme;
      if (j)
        j.value = f.darkTheme;
      if (M)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: fE(f) }));
    }
    function U(h = {}) {
      R(a(H, h.mode ?? f.mode, h.lightTheme ?? f.lightTheme, h.darkTheme ?? f.darkTheme, V.matches));
    }
    function m(h, M = !1) {
      if (!O || !J)
        return;
      if (O.hidden = !h, J.setAttribute("aria-expanded", String(h)), h)
        O.querySelector("button,select")?.focus();
      else if (M)
        J.focus();
    }
    J?.addEventListener("click", () => m(Boolean(O?.hidden ?? !0))), x.forEach((h) => h.addEventListener("click", () => {
      let M = h.dataset.airplanColorMode;
      if (!M)
        return;
      hE(Y, M), U({ mode: M });
    }));
    function b(h, M) {
      uE(Y, h, M.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: M.value } })), U(h === "light" ? { lightTheme: M.value } : { darkTheme: M.value });
    }
    K?.addEventListener("change", () => b("light", K)), j?.addEventListener("change", () => b("dark", j)), V.addEventListener("change", () => {
      if (f.mode === "system")
        U();
    }), E.addEventListener("keydown", (h) => {
      if (h.key === "Escape" && O && !O.hidden)
        h.preventDefault(), m(!1, !0);
    }), E.addEventListener("pointerdown", (h) => {
      if (!O || O.hidden || !J)
        return;
      let M = h.target;
      if (!(M instanceof Node) || O.contains(M) || J.contains(M))
        return;
      let Z = (M instanceof Element ? M : M.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), w = O.contains(E.activeElement) && !Z;
      if (m(!1), w)
        setTimeout(() => {
          if (E.activeElement === E.body || O.contains(E.activeElement))
            J.focus();
        });
    }), R(f, !1);
  })();

  (function() {
    var E = document;
    let A = E.getElementById("rendered");
    if (!A)
      return;
    let I = A;
    var H = E.querySelector('meta[name="airplan-versions"]'), V = window.location.pathname.split("/").filter(Boolean), Y = V.slice(0, -2);
    function f(S, B) {
      if (typeof S !== "string")
        return null;
      try {
        var _ = new URL(S);
        if (_.origin !== window.location.origin || _.username || _.password || _.search || _.hash)
          return null;
        var G = _.pathname.split("/").filter(Boolean);
        if (G.length !== Y.length + 2 || !Y.every(function($, c) {
          return G[c] === $;
        }) || !/^[a-z2-7]{26}$/.test(G[G.length - 2]))
          return null;
        var Q = G[G.length - 1];
        if (B ? Q !== ".airplan-changes.diff" : !Q.endsWith(".html"))
          return null;
        return _.href;
      } catch {
        return null;
      }
    }
    function J(S) {
      var B = E.querySelector('meta[name="airplan-revision"]'), _ = B ? Number(B.content) : Number(S.current_revision);
      if (!Number.isInteger(_) || _ <= 0 || S.current_revision !== _ || !Number.isInteger(S.latest_revision) || !Number.isInteger(S.last_assigned_revision) || !Array.isArray(S.revisions) || S.revisions.length === 0 || S.last_assigned_revision !== S.revisions.length || !/^[a-z2-7]{26}$/.test(S.chain_id))
        throw Error("revision identity is invalid");
      var G = !1, Q = 0, $ = S.revisions.filter(function(u) {
        if (!u || !Number.isInteger(u.number) || u.number !== Q + 1)
          return G = !0, !1;
        if (Q = u.number, u.deleted)
          return !1;
        if (u.safeURL = f(u.url, !1), !u.safeURL)
          return G = !0, !1;
        if (u.number > 1) {
          var C = f(u.diff_url, !0);
          if (!C || new URL(C).pathname.replace(/[^/]+$/, "") !== new URL(u.safeURL).pathname.replace(/[^/]+$/, ""))
            return G = !0, !1;
        }
        return !0;
      });
      if (G || S.revisions[0].number !== 1 || !$.some(function(u) {
        return u.number === _;
      }))
        throw Error("revision entries are invalid");
      var c = $.find(function(u) {
        return u.number === _;
      }), ME = window.location.origin + window.location.pathname;
      if (!c || c.safeURL !== ME)
        throw Error("current revision URL is invalid");
      var k = Math.max.apply(null, $.map(function(u) {
        return u.number;
      }));
      if (k !== S.latest_revision)
        throw Error("latest is invalid");
      var F = E.querySelector("[data-revision-heading]");
      if (!F) {
        F = E.createElement("p"), F.className = "revision-heading", F.setAttribute("data-revision-heading", "");
        var SE = E.getElementById("rendered");
        if (!SE)
          throw Error("rendered view is unavailable");
        SE.prepend(F);
      }
      var o = _ < k, qE = o ? "Revision " + _ + " of " + k : "Revision " + _ + " (Latest)", d = E.createElement("span");
      d.className = "revision-picker-label", d.textContent = qE, d.setAttribute("aria-hidden", "true");
      var D = E.createElement("select");
      D.setAttribute("aria-label", "Document revision"), $.forEach(function(u) {
        var C = E.createElement("option");
        C.value = u.safeURL || "", C.textContent = u.number === k ? "Revision " + u.number + " (Latest)" : "Revision " + u.number + " of " + k, C.selected = u.number === _, D.appendChild(C);
      }), D.addEventListener("change", function() {
        var u = D.selectedIndex;
        if (u < 0 || u >= $.length)
          return;
        window.location.assign($[u].safeURL || "");
      }), F.replaceChildren(d, D), F.classList.add("is-picker"), F.classList.toggle("is-stale", o), E.body.classList.toggle("airplan-stale-revision", o);
    }
    if (H) {
      var O = new URL(H.content, window.location.href);
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
        J(S), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: S
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var K = E.createElement("div");
    K.className = "sr-status", K.setAttribute("aria-live", "polite"), E.body.appendChild(K);
    var j = null;
    function x() {
      if (j !== null)
        return;
      j = Array.from(E.querySelectorAll("details:not([open])")), j.forEach(function(S) {
        S.open = !0;
      });
    }
    function n() {
      if (j === null)
        return;
      j.forEach(function(S) {
        S.open = !1;
      }), j = null;
    }
    window.addEventListener("beforeprint", x), window.addEventListener("afterprint", n);
    function R(S, B, _) {
      K.textContent = B;
      var G = S.querySelector(".action-label"), Q = G ? G.textContent : "";
      if (G)
        G.textContent = _ ? "Copied" : "Failed";
      S.classList.add(_ ? "is-copied" : "is-failed"), S.disabled = !0, setTimeout(function() {
        if (S.classList.remove("is-copied", "is-failed"), S.disabled = !1, G)
          G.textContent = Q;
      }, 1200);
    }
    function U(S, B) {
      if (!navigator.clipboard) {
        R(B, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(S).then(function() {
        R(B, "Copied!", !0);
      }, function() {
        R(B, "Copy failed", !1);
      });
    }
    var m = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', b = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', h = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', M = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', W = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', Z = E.getElementById("source"), w = E.getElementById("changes"), X = E.getElementById("toc"), z = null, q = null, e = window.matchMedia("(max-width: 78rem)");
    function p() {
      if (q && q.open)
        q.close();
    }
    function y() {
      if (!X || !z || !q)
        return;
      var S = e.matches && !I.hidden && X.getBoundingClientRect().bottom < 0 && !q.open;
      if (z.classList.toggle("is-visible", S), z.tabIndex = S ? 0 : -1, z.setAttribute("aria-hidden", S ? "false" : "true"), q.open && (!e.matches || I.hidden))
        p();
    }
    if (E.querySelectorAll(".viewtoggle button").forEach(function(S) {
      S.addEventListener("click", function() {
        var B = S.dataset.view;
        if (I.hidden = B !== "rendered", Z)
          Z.hidden = B !== "source";
        if (w)
          w.hidden = B !== "changes";
        if (X)
          X.hidden = B !== "rendered";
        E.querySelectorAll(".viewtoggle button").forEach(function(_) {
          _.classList.toggle("active", _ === S), _.setAttribute("aria-pressed", _ === S ? "true" : "false");
        }), y();
      });
    }), X) {
      let S = function() {
        if (P.length === 0) {
          y();
          return;
        }
        var _ = 0;
        if (AE.forEach(function(Q, $) {
          if (Q && Q.getBoundingClientRect().top <= 128)
            _ = $;
        }), window.innerHeight + window.scrollY >= E.documentElement.scrollHeight - 2)
          _ = P.length - 1;
        var G = P[_].getAttribute("href");
        r.forEach(function(Q) {
          var $ = Q.getAttribute("href") === G;
          if (Q.classList.toggle("active", $), $)
            Q.setAttribute("aria-current", "location");
          else
            Q.removeAttribute("aria-current");
        }), y();
      }, B = function() {
        if (s)
          return;
        s = !0, window.requestAnimationFrame(function() {
          s = !1, S();
        });
      };
      var P = Array.from(X.querySelectorAll('a[href^="#"]')), EE = X.querySelector(".toc-list");
      if (EE)
        if (q = E.createElement("dialog"), typeof q.showModal === "function") {
          q.className = "toc-dialog", q.id = "toc-dialog", q.setAttribute("aria-labelledby", "toc-dialog-title");
          var v = E.createElement("div");
          v.className = "toc-dialog-panel";
          var l = E.createElement("div");
          l.className = "toc-dialog-header";
          var g = E.createElement("h2");
          g.className = "toc-dialog-title", g.id = "toc-dialog-title", g.textContent = "Contents";
          var N = E.createElement("button");
          N.className = "toc-dialog-close", N.type = "button", N.setAttribute("aria-label", "Close table of contents"), N.innerHTML = W, l.appendChild(g), l.appendChild(N);
          var T = E.createElement("nav");
          T.className = "toc-dialog-nav", T.setAttribute("aria-label", "Table of contents"), T.appendChild(EE.cloneNode(!0)), v.appendChild(l), v.appendChild(T), q.appendChild(v), z = E.createElement("button"), z.className = "toc-trigger", z.type = "button", z.tabIndex = -1, z.setAttribute("aria-label", "Open table of contents"), z.setAttribute("aria-controls", "toc-dialog"), z.setAttribute("aria-haspopup", "dialog"), z.setAttribute("aria-hidden", "true"), z.innerHTML = M, E.body.appendChild(z), E.body.appendChild(q), z.addEventListener("click", function() {
            q.showModal(), E.body.classList.add("toc-dialog-open"), y();
            var _ = q.querySelector("a.active");
            if (_)
              _.scrollIntoView({ block: "nearest" });
          }), N.addEventListener("click", p), q.addEventListener("click", function(_) {
            if (_.target === q)
              p();
          }), q.addEventListener("keydown", function(_) {
            if (_.key === "Escape")
              _.preventDefault(), p();
          }), q.addEventListener("close", function() {
            if (E.body.classList.remove("toc-dialog-open"), y(), z.classList.contains("is-visible"))
              setTimeout(function() {
                z.focus();
              }, 50);
          }), T.querySelectorAll("a").forEach(function(_) {
            _.addEventListener("click", p);
          });
        } else
          q = null;
      var r = P.slice();
      if (q)
        r = r.concat(Array.from(q.querySelectorAll('a[href^="#"]')));
      var AE = P.map(function(_) {
        return E.getElementById((_.getAttribute("href") || "").slice(1));
      }), s = !1;
      E.addEventListener("scroll", B, { passive: !0 }), window.addEventListener("resize", S), S();
    }
    let t = E.querySelector(".copy-source");
    if (t && Z)
      t.addEventListener("click", function() {
        var S = Z.querySelector("pre");
        U(S ? S.textContent : "", t);
      });
    I.querySelectorAll("pre").forEach(function(S) {
      if (S.classList.contains("mermaid"))
        return;
      var B = E.createElement("div");
      B.className = "codewrap", S.parentNode?.insertBefore(B, S), B.appendChild(S);
      var _ = E.createElement("button");
      _.className = "codecopy", _.type = "button", _.setAttribute("aria-label", "Copy code"), _.title = "Copy code", _.innerHTML = m + b + h, _.addEventListener("click", function() {
        var G = S.querySelector("code");
        U((G || S).textContent, _);
      }), B.appendChild(_);
    });
  })();
})();
