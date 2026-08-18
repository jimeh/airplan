(() => {
  function q(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function s(e, n) {
    try {
      return e?.getItem(n) ?? null;
    } catch {
      return null;
    }
  }
  function f(e, n, l) {
    try {
      if (l === null)
        e?.removeItem(n);
      else
        e?.setItem(n, l);
    } catch {}
  }
  function A(e, n, l) {
    let a = s(l, "airplan-color-mode");
    if (a === null) {
      let p = s(l, "airplan-theme");
      if (a = p === "light" || p === "dark" ? p : "system", a !== "system")
        f(l, "airplan-color-mode", a);
    }
    let u = q(a) ? a : "system", r = new Set(e.themes.map((p) => p.id)), i = s(l, "airplan-light-theme"), d = s(l, "airplan-dark-theme"), c = i !== null && r.has(i) ? i : e.defaultLight, m = d !== null && r.has(d) ? d : e.defaultDark;
    return L(e, u, c, m, n);
  }
  function L(e, n, l, a, u) {
    let r = new Map(e.themes.map((v) => [v.id, v])), i = r.has(l) ? l : e.defaultLight, d = r.has(a) ? a : e.defaultDark, c = n === "system" ? u ? "dark" : "light" : n, m = c === "light" ? i : d, p = r.get(m)?.variant ?? c;
    return { mode: n, resolvedMode: c, lightTheme: i, darkTheme: d, theme: m, variant: p };
  }
  function H(e, n) {
    if (n === "system")
      f(e, "airplan-color-mode", null), f(e, "airplan-theme", null);
    else
      f(e, "airplan-color-mode", n), f(e, "airplan-theme", n);
  }
  function x(e, n, l) {
    f(e, n === "light" ? "airplan-light-theme" : "airplan-dark-theme", l);
  }
  function B(e) {
    return {
      mode: e.mode,
      resolvedMode: e.resolvedMode,
      theme: e.theme,
      variant: e.variant
    };
  }

  (function() {
    let e = document, n = e.documentElement, l = window.__AIRPLAN_THEME_CATALOG__;
    if (!l)
      return;
    let a = l, u = window.matchMedia("(prefers-color-scheme: dark)"), r;
    try {
      r = window.localStorage;
    } catch {}
    let i = window.__airplanThemeState ?? A(a, u.matches, r);
    e.querySelectorAll(".js-only").forEach((t) => {
      t.hidden = !1;
    });
    let d = e.querySelector("[data-airplan-appearance-trigger]"), c = e.querySelector("[data-airplan-appearance-panel]"), m = e.querySelector('select[data-airplan-theme-slot="light"]'), p = e.querySelector('select[data-airplan-theme-slot="dark"]'), v = Array.from(e.querySelectorAll("[data-airplan-color-mode]"));
    function b(t) {
      if (!t || t.options.length > 0)
        return;
      for (let [o, E] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let h = e.createElement("optgroup");
        h.label = E;
        for (let T of a.themes) {
          if (T.variant !== o)
            continue;
          let C = e.createElement("option");
          C.value = T.id, C.textContent = T.name, h.append(C);
        }
        t.append(h);
      }
    }
    b(m), b(p);
    function _(t, o = !0) {
      if (i = t, window.__airplanThemeState = i, n.dataset.airplanMode = i.mode, n.dataset.airplanResolvedMode = i.resolvedMode, n.dataset.airplanTheme = i.theme, n.dataset.airplanThemeVariant = i.variant, v.forEach((E) => {
        let h = E.dataset.airplanColorMode === i.mode;
        E.classList.toggle("active", h), E.setAttribute("aria-pressed", String(h));
      }), m)
        m.value = i.lightTheme;
      if (p)
        p.value = i.darkTheme;
      if (o)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: B(i) }));
    }
    function S(t = {}) {
      _(L(a, t.mode ?? i.mode, t.lightTheme ?? i.lightTheme, t.darkTheme ?? i.darkTheme, u.matches));
    }
    function w(t, o = !1) {
      if (!c || !d)
        return;
      if (c.hidden = !t, d.setAttribute("aria-expanded", String(t)), t)
        c.querySelector("button,select")?.focus();
      else if (o)
        d.focus();
    }
    d?.addEventListener("click", () => w(Boolean(c?.hidden ?? !0))), v.forEach((t) => t.addEventListener("click", () => {
      let o = t.dataset.airplanColorMode;
      if (!o)
        return;
      H(r, o), S({ mode: o });
    }));
    function M(t, o) {
      x(r, t, o.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: o.value } })), S(t === "light" ? { lightTheme: o.value } : { darkTheme: o.value });
    }
    m?.addEventListener("change", () => M("light", m)), p?.addEventListener("change", () => M("dark", p)), u.addEventListener("change", () => {
      if (i.mode === "system")
        S();
    }), e.addEventListener("keydown", (t) => {
      if (t.key === "Escape" && c && !c.hidden)
        t.preventDefault(), w(!1, !0);
    }), e.addEventListener("pointerdown", (t) => {
      if (!c || c.hidden || !d)
        return;
      let o = t.target;
      if (!(o instanceof Node) || c.contains(o) || d.contains(o))
        return;
      let h = (o instanceof Element ? o : o.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), T = c.contains(e.activeElement) && !h;
      if (w(!1), T)
        setTimeout(() => {
          if (e.activeElement === e.body || c.contains(e.activeElement))
            d.focus();
        });
    }), _(i, !1);
  })();

  (function() {
    var e = document, n = new WeakMap;
    e.addEventListener("click", function(l) {
      let a = l.target instanceof Element ? l.target.closest("[data-copy],[data-copy-overview]") : null;
      if (!a)
        return;
      var u = a.hasAttribute("data-copy-overview") ? location.href : new URL(a.dataset.copy || "", e.baseURI).href;
      if (!navigator.clipboard) {
        prompt("Copy link", u);
        return;
      }
      navigator.clipboard.writeText(u).then(function() {
        var r = n.get(a);
        if (r)
          window.clearTimeout(r.timer);
        var i = r ? r.label : a.textContent;
        a.textContent = "Copied";
        var d = window.setTimeout(function() {
          a.textContent = i, n.delete(a);
        }, 1200);
        n.set(a, { label: i, timer: d });
      }, function() {
        prompt("Copy link", u);
      });
    });
  })();
})();
