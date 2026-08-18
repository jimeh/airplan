(() => {
  (function() {
    var z = document;
    z.querySelectorAll(".js-only").forEach(function(Z) {
      Z.hidden = !1;
    });
    var S = z.documentElement, $ = window.matchMedia("(prefers-color-scheme: dark)");
    function V() {
      return S.dataset.theme || "system";
    }
    function u() {
      var Z = V();
      if (Z !== "system")
        return Z;
      return $.matches ? "dark" : "light";
    }
    function U() {
      window.dispatchEvent(new CustomEvent("airplan:themechange", {
        detail: { theme: u() }
      }));
    }
    function M() {
      var Z = V();
      z.querySelectorAll(".themetoggle button").forEach(function(Y) {
        var A = Y.dataset.theme === Z;
        Y.classList.toggle("active", A), Y.setAttribute("aria-pressed", A ? "true" : "false");
      });
    }
    z.querySelectorAll(".themetoggle button").forEach(function(Z) {
      Z.addEventListener("click", function() {
        var Y = Z.dataset.theme;
        if (!Y)
          return;
        if (Y === "system")
          delete S.dataset.theme;
        else
          S.dataset.theme = Y;
        try {
          if (Y === "system")
            localStorage.removeItem("airplan-theme");
          else
            localStorage.setItem("airplan-theme", Y);
        } catch {}
        M(), U();
      });
    }), $.addEventListener("change", function() {
      if (V() === "system")
        U();
    }), M();
  })();

  (function() {
    var z = document;
    let S = z.getElementById("rendered");
    if (!S)
      return;
    let $ = S;
    var V = z.querySelector('meta[name="airplan-versions"]'), u = window.location.pathname.split("/").filter(Boolean), U = u.slice(0, -2);
    function M(j, J) {
      if (typeof j !== "string")
        return null;
      try {
        var q = new URL(j);
        if (q.origin !== window.location.origin || q.username || q.password || q.search || q.hash)
          return null;
        var K = q.pathname.split("/").filter(Boolean);
        if (K.length !== U.length + 2 || !U.every(function(W, k) {
          return K[k] === W;
        }) || !/^[a-z2-7]{26}$/.test(K[K.length - 2]))
          return null;
        var Q = K[K.length - 1];
        if (J ? Q !== ".airplan-changes.diff" : !Q.endsWith(".html"))
          return null;
        return q.href;
      } catch {
        return null;
      }
    }
    function Z(j) {
      var J = z.querySelector('meta[name="airplan-revision"]'), q = J ? Number(J.content) : Number(j.current_revision);
      if (!Number.isInteger(q) || q <= 0 || j.current_revision !== q || !Number.isInteger(j.latest_revision) || !Number.isInteger(j.last_assigned_revision) || !Array.isArray(j.revisions) || j.revisions.length === 0 || j.last_assigned_revision !== j.revisions.length || !/^[a-z2-7]{26}$/.test(j.chain_id))
        throw Error("revision identity is invalid");
      var K = !1, Q = 0, W = j.revisions.filter(function(G) {
        if (!G || !Number.isInteger(G.number) || G.number !== Q + 1)
          return K = !0, !1;
        if (Q = G.number, G.deleted)
          return !1;
        if (G.safeURL = M(G.url, !1), !G.safeURL)
          return K = !0, !1;
        if (G.number > 1) {
          var B = M(G.diff_url, !0);
          if (!B || new URL(B).pathname.replace(/[^/]+$/, "") !== new URL(G.safeURL).pathname.replace(/[^/]+$/, ""))
            return K = !0, !1;
        }
        return !0;
      });
      if (K || j.revisions[0].number !== 1 || !W.some(function(G) {
        return G.number === q;
      }))
        throw Error("revision entries are invalid");
      var k = W.find(function(G) {
        return G.number === q;
      }), jj = window.location.origin + window.location.pathname;
      if (!k || k.safeURL !== jj)
        throw Error("current revision URL is invalid");
      var H = Math.max.apply(null, W.map(function(G) {
        return G.number;
      }));
      if (H !== j.latest_revision)
        throw Error("latest is invalid");
      var _ = z.querySelector("[data-revision-heading]");
      if (!_) {
        _ = z.createElement("p"), _.className = "revision-heading", _.setAttribute("data-revision-heading", "");
        var d = z.getElementById("rendered");
        if (!d)
          throw Error("rendered view is unavailable");
        d.prepend(_);
      }
      var m = q < H, qj = m ? "Revision " + q + " of " + H : "Revision " + q + " (Latest)", D = z.createElement("span");
      D.className = "revision-picker-label", D.textContent = qj, D.setAttribute("aria-hidden", "true");
      var N = z.createElement("select");
      N.setAttribute("aria-label", "Document revision"), W.forEach(function(G) {
        var B = z.createElement("option");
        B.value = G.safeURL || "", B.textContent = G.number === H ? "Revision " + G.number + " (Latest)" : "Revision " + G.number + " of " + H, B.selected = G.number === q, N.appendChild(B);
      }), N.addEventListener("change", function() {
        var G = N.selectedIndex;
        if (G < 0 || G >= W.length)
          return;
        window.location.assign(W[G].safeURL || "");
      }), _.replaceChildren(D, N), _.classList.add("is-picker"), _.classList.toggle("is-stale", m), z.body.classList.toggle("airplan-stale-revision", m);
    }
    if (V) {
      var Y = new URL(V.content, window.location.href);
      Y.searchParams.set("_airplan", Date.now().toString(36) + Math.random().toString(36).slice(2)), fetch(Y, { cache: "no-store", credentials: "same-origin" }).then(function(j) {
        if (j.status === 404)
          return null;
        if (!j.ok)
          throw Error("metadata request failed");
        return j.json();
      }).then(function(j) {
        if (j === null)
          return;
        if (!j || j.schema !== "airplan-versions" || j.version !== 1 || !Array.isArray(j.revisions) || j.revisions.length < 2)
          throw Error("metadata is invalid");
        Z(j), window.dispatchEvent(new CustomEvent("airplan:versions", {
          detail: j
        }));
      }).catch(function() {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
    }
    var A = z.createElement("div");
    A.className = "sr-status", A.setAttribute("aria-live", "polite"), z.body.appendChild(A);
    var E = null;
    function s() {
      if (E !== null)
        return;
      E = Array.from(z.querySelectorAll("details:not([open])")), E.forEach(function(j) {
        j.open = !0;
      });
    }
    function r() {
      if (E === null)
        return;
      E.forEach(function(j) {
        j.open = !1;
      }), E = null;
    }
    window.addEventListener("beforeprint", s), window.addEventListener("afterprint", r);
    function h(j, J, q) {
      A.textContent = J;
      var K = j.querySelector(".action-label"), Q = K ? K.textContent : "";
      if (K)
        K.textContent = q ? "Copied" : "Failed";
      j.classList.add(q ? "is-copied" : "is-failed"), j.disabled = !0, setTimeout(function() {
        if (j.classList.remove("is-copied", "is-failed"), j.disabled = !1, K)
          K.textContent = Q;
      }, 1200);
    }
    function g(j, J) {
      if (!navigator.clipboard) {
        h(J, "Copy failed", !1);
        return;
      }
      navigator.clipboard.writeText(j).then(function() {
        h(J, "Copied!", !0);
      }, function() {
        h(J, "Copy failed", !1);
      });
    }
    var n = '<svg class="icon icon-copy" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25Z"/></svg>', i = '<svg class="icon icon-check" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>', o = '<svg class="icon icon-x" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', a = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M5 4h9M5 8h9M5 12h9"/><circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/><circle cx="2" cy="12" r=".75" fill="currentColor" stroke="none"/></svg>', t = '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>', y = z.getElementById("source"), v = z.getElementById("changes"), X = z.getElementById("toc"), O = null, I = null, l = window.matchMedia("(max-width: 78rem)");
    function R() {
      if (I && I.open)
        I.close();
    }
    function w() {
      if (!X || !O || !I)
        return;
      var j = l.matches && !$.hidden && X.getBoundingClientRect().bottom < 0 && !I.open;
      if (O.classList.toggle("is-visible", j), O.tabIndex = j ? 0 : -1, O.setAttribute("aria-hidden", j ? "false" : "true"), I.open && (!l.matches || $.hidden))
        R();
    }
    if (z.querySelectorAll(".viewtoggle button").forEach(function(j) {
      j.addEventListener("click", function() {
        var J = j.dataset.view;
        if ($.hidden = J !== "rendered", y)
          y.hidden = J !== "source";
        if (v)
          v.hidden = J !== "changes";
        if (X)
          X.hidden = J !== "rendered";
        z.querySelectorAll(".viewtoggle button").forEach(function(q) {
          q.classList.toggle("active", q === j), q.setAttribute("aria-pressed", q === j ? "true" : "false");
        }), w();
      });
    }), X) {
      let j = function() {
        var q = 0;
        if (e.forEach(function(Q, W) {
          if (Q && Q.getBoundingClientRect().top <= 128)
            q = W;
        }), window.innerHeight + window.scrollY >= z.documentElement.scrollHeight - 2)
          q = C.length - 1;
        var K = C[q].getAttribute("href");
        T.forEach(function(Q) {
          var W = Q.getAttribute("href") === K;
          if (Q.classList.toggle("active", W), W)
            Q.setAttribute("aria-current", "location");
          else
            Q.removeAttribute("aria-current");
        }), w();
      }, J = function() {
        if (b)
          return;
        b = !0, window.requestAnimationFrame(function() {
          b = !1, j();
        });
      };
      var C = Array.from(X.querySelectorAll('a[href^="#"]')), c = X.querySelector(".toc-list");
      if (c)
        if (I = z.createElement("dialog"), typeof I.showModal === "function") {
          I.className = "toc-dialog", I.id = "toc-dialog", I.setAttribute("aria-labelledby", "toc-dialog-title");
          var P = z.createElement("div");
          P.className = "toc-dialog-panel";
          var f = z.createElement("div");
          f.className = "toc-dialog-header";
          var L = z.createElement("h2");
          L.className = "toc-dialog-title", L.id = "toc-dialog-title", L.textContent = "Contents";
          var F = z.createElement("button");
          F.className = "toc-dialog-close", F.type = "button", F.setAttribute("aria-label", "Close table of contents"), F.innerHTML = t, f.appendChild(L), f.appendChild(F);
          var x = z.createElement("nav");
          x.className = "toc-dialog-nav", x.setAttribute("aria-label", "Table of contents"), x.appendChild(c.cloneNode(!0)), P.appendChild(f), P.appendChild(x), I.appendChild(P), O = z.createElement("button"), O.className = "toc-trigger", O.type = "button", O.tabIndex = -1, O.setAttribute("aria-label", "Open table of contents"), O.setAttribute("aria-controls", "toc-dialog"), O.setAttribute("aria-haspopup", "dialog"), O.setAttribute("aria-hidden", "true"), O.innerHTML = a, z.body.appendChild(O), z.body.appendChild(I), O.addEventListener("click", function() {
            I.showModal(), z.body.classList.add("toc-dialog-open"), w();
            var q = I.querySelector("a.active");
            if (q)
              q.scrollIntoView({ block: "nearest" });
          }), F.addEventListener("click", R), I.addEventListener("click", function(q) {
            if (q.target === I)
              R();
          }), I.addEventListener("keydown", function(q) {
            if (q.key === "Escape")
              q.preventDefault(), R();
          }), I.addEventListener("close", function() {
            if (z.body.classList.remove("toc-dialog-open"), w(), O.classList.contains("is-visible"))
              setTimeout(function() {
                O.focus();
              }, 50);
          }), x.querySelectorAll("a").forEach(function(q) {
            q.addEventListener("click", R);
          });
        } else
          I = null;
      var T = C.slice();
      if (I)
        T = T.concat(Array.from(I.querySelectorAll('a[href^="#"]')));
      var e = C.map(function(q) {
        return z.getElementById((q.getAttribute("href") || "").slice(1));
      }), b = !1;
      z.addEventListener("scroll", J, { passive: !0 }), window.addEventListener("resize", j), j();
    }
    let p = z.querySelector(".copy-source");
    if (p && y)
      p.addEventListener("click", function() {
        var j = y.querySelector("pre");
        g(j ? j.textContent : "", p);
      });
    $.querySelectorAll("pre").forEach(function(j) {
      if (j.classList.contains("mermaid"))
        return;
      var J = z.createElement("div");
      J.className = "codewrap", j.parentNode?.insertBefore(J, j), J.appendChild(j);
      var q = z.createElement("button");
      q.className = "codecopy", q.type = "button", q.setAttribute("aria-label", "Copy code"), q.title = "Copy code", q.innerHTML = n + i + o, q.addEventListener("click", function() {
        var K = j.querySelector("code");
        g((K || j).textContent, q);
      }), J.appendChild(q);
    });
  })();
})();
