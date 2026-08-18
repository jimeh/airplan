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
  function E(e, n, o) {
    try {
      if (o === null)
        e?.removeItem(n);
      else
        e?.setItem(n, o);
    } catch {}
  }
  function A(e, n, o) {
    let a = s(o, "airplan-color-mode");
    if (a === null) {
      let p = s(o, "airplan-theme");
      if (a = p === "light" || p === "dark" ? p : "system", a !== "system")
        E(o, "airplan-color-mode", a);
    }
    let u = q(a) ? a : "system", r = new Set(e.themes.map((p) => p.id)), i = s(o, "airplan-light-theme"), d = s(o, "airplan-dark-theme"), c = i !== null && r.has(i) ? i : e.defaultLight, m = d !== null && r.has(d) ? d : e.defaultDark;
    return L(e, u, c, m, n);
  }
  function L(e, n, o, a, u) {
    let r = new Map(e.themes.map((v) => [v.id, v])), i = r.has(o) ? o : e.defaultLight, d = r.has(a) ? a : e.defaultDark, c = n === "system" ? u ? "dark" : "light" : n, m = c === "light" ? i : d, p = r.get(m)?.variant ?? c;
    return { mode: n, resolvedMode: c, lightTheme: i, darkTheme: d, theme: m, variant: p };
  }
  function H(e, n) {
    if (n === "system")
      E(e, "airplan-color-mode", null), E(e, "airplan-theme", null);
    else
      E(e, "airplan-color-mode", n), E(e, "airplan-theme", n);
  }
  function x(e, n, o) {
    E(e, n === "light" ? "airplan-light-theme" : "airplan-dark-theme", o);
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
    let e = document, n = e.documentElement;
    e.querySelectorAll(".js-only").forEach((t) => {
      t.hidden = !1;
    });
    let o = window.__AIRPLAN_THEME_CATALOG__;
    if (!o)
      return;
    let a = o, u = window.matchMedia("(prefers-color-scheme: dark)"), r;
    try {
      r = window.localStorage;
    } catch {}
    let i = window.__airplanThemeState ?? A(a, u.matches, r), d = e.querySelector("[data-airplan-appearance-trigger]"), c = e.querySelector("[data-airplan-appearance-panel]"), m = e.querySelector('select[data-airplan-theme-slot="light"]'), p = e.querySelector('select[data-airplan-theme-slot="dark"]'), v = Array.from(e.querySelectorAll("[data-airplan-color-mode]"));
    function b(t) {
      if (!t || t.options.length > 0)
        return;
      for (let [l, f] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let h = e.createElement("optgroup");
        h.label = f;
        for (let T of a.themes) {
          if (T.variant !== l)
            continue;
          let C = e.createElement("option");
          C.value = T.id, C.textContent = T.name, h.append(C);
        }
        if (h.children.length > 0)
          t.append(h);
      }
    }
    b(m), b(p);
    function _(t, l = !0) {
      if (i = t, window.__airplanThemeState = i, n.dataset.airplanMode = i.mode, n.dataset.airplanResolvedMode = i.resolvedMode, n.dataset.airplanTheme = i.theme, n.dataset.airplanThemeVariant = i.variant, v.forEach((f) => {
        let h = f.dataset.airplanColorMode === i.mode;
        f.classList.toggle("active", h), f.setAttribute("aria-pressed", String(h));
      }), m)
        m.value = i.lightTheme;
      if (p)
        p.value = i.darkTheme;
      if (l)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: B(i) }));
    }
    function S(t = {}) {
      _(L(a, t.mode ?? i.mode, t.lightTheme ?? i.lightTheme, t.darkTheme ?? i.darkTheme, u.matches));
    }
    function w(t, l = !1) {
      if (!c || !d)
        return;
      if (c.hidden = !t, d.setAttribute("aria-expanded", String(t)), t)
        c.querySelector("button,select")?.focus();
      else if (l)
        d.focus();
    }
    d?.addEventListener("click", () => w(Boolean(c?.hidden ?? !0))), v.forEach((t) => t.addEventListener("click", () => {
      let l = t.dataset.airplanColorMode;
      if (!l)
        return;
      H(r, l), S({ mode: l });
    }));
    function M(t, l) {
      x(r, t, l.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: l.value } })), S(t === "light" ? { lightTheme: l.value } : { darkTheme: l.value });
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
      let l = t.target;
      if (!(l instanceof Node) || c.contains(l) || d.contains(l))
        return;
      let h = (l instanceof Element ? l : l.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), T = c.contains(e.activeElement) && !h;
      if (w(!1), T)
        setTimeout(() => {
          if (e.activeElement === e.body || c.contains(e.activeElement))
            d.focus();
        });
    }), _(i, !1);
  })();

  (function() {
    var e = document, n = new WeakMap;
    e.addEventListener("click", function(o) {
      let a = o.target instanceof Element ? o.target.closest("[data-copy],[data-copy-overview]") : null;
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
