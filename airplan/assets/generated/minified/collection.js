(() => {
  function q(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function T(e, n) {
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
    let a = T(l, "airplan-color-mode");
    if (a === null) {
      let p = T(l, "airplan-theme");
      if (a = p === "light" || p === "dark" ? p : "system", a !== "system")
        f(l, "airplan-color-mode", a);
    }
    let u = q(a) ? a : "system", o = new Set(e.themes.map((p) => p.id)), i = T(l, "airplan-light-theme"), c = T(l, "airplan-dark-theme"), d = i !== null && o.has(i) ? i : e.defaultLight, m = c !== null && o.has(c) ? c : e.defaultDark;
    return _(e, u, d, m, n);
  }
  function _(e, n, l, a, u) {
    let o = new Map(e.themes.map((E) => [E.id, E])), i = o.has(l) ? l : e.defaultLight, c = o.has(a) ? a : e.defaultDark, d = n === "system" ? u ? "dark" : "light" : n, m = d === "light" ? i : c, p = o.get(m)?.variant ?? d;
    return { mode: n, resolvedMode: d, lightTheme: i, darkTheme: c, theme: m, variant: p };
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
    let a = l, u = window.matchMedia("(prefers-color-scheme: dark)"), o;
    try {
      o = window.localStorage;
    } catch {}
    let i = window.__airplanThemeState ?? A(a, u.matches, o);
    e.querySelectorAll(".js-only").forEach((t) => {
      t.hidden = !1;
    });
    let c = e.querySelector("[data-airplan-appearance-trigger]"), d = e.querySelector("[data-airplan-appearance-panel]"), m = e.querySelector('select[data-airplan-theme-slot="light"]'), p = e.querySelector('select[data-airplan-theme-slot="dark"]'), E = Array.from(e.querySelectorAll("[data-airplan-color-mode]"));
    function s(t) {
      if (!t || t.options.length > 0)
        return;
      for (let [r, v] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let h = e.createElement("optgroup");
        h.label = v;
        for (let C of a.themes) {
          if (C.variant !== r)
            continue;
          let L = e.createElement("option");
          L.value = C.id, L.textContent = C.name, h.append(L);
        }
        t.append(h);
      }
    }
    s(m), s(p);
    function M(t, r = !0) {
      if (i = t, window.__airplanThemeState = i, n.dataset.airplanMode = i.mode, n.dataset.airplanResolvedMode = i.resolvedMode, n.dataset.airplanTheme = i.theme, n.dataset.airplanThemeVariant = i.variant, E.forEach((v) => {
        let h = v.dataset.airplanColorMode === i.mode;
        v.classList.toggle("active", h), v.setAttribute("aria-pressed", String(h));
      }), m)
        m.value = i.lightTheme;
      if (p)
        p.value = i.darkTheme;
      if (r)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: B(i) }));
    }
    function S(t = {}) {
      M(_(a, t.mode ?? i.mode, t.lightTheme ?? i.lightTheme, t.darkTheme ?? i.darkTheme, u.matches));
    }
    function w(t, r = !1) {
      if (!d || !c)
        return;
      if (d.hidden = !t, c.setAttribute("aria-expanded", String(t)), t)
        d.querySelector("button,select")?.focus();
      else if (r)
        c.focus();
    }
    c?.addEventListener("click", () => w(Boolean(d?.hidden ?? !0))), E.forEach((t) => t.addEventListener("click", () => {
      let r = t.dataset.airplanColorMode;
      if (!r)
        return;
      H(o, r), S({ mode: r });
    }));
    function b(t, r) {
      x(o, t, r.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: r.value } })), S(t === "light" ? { lightTheme: r.value } : { darkTheme: r.value });
    }
    m?.addEventListener("change", () => b("light", m)), p?.addEventListener("change", () => b("dark", p)), u.addEventListener("change", () => {
      if (i.mode === "system")
        S();
    }), e.addEventListener("keydown", (t) => {
      if (t.key === "Escape" && d && !d.hidden)
        t.preventDefault(), w(!1, !0);
    }), e.addEventListener("pointerdown", (t) => {
      if (!d || d.hidden || !c)
        return;
      let r = t.target;
      if (!d.contains(r) && !c.contains(r))
        w(!1);
    }), M(i, !1);
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
        var o = n.get(a);
        if (o)
          window.clearTimeout(o.timer);
        var i = o ? o.label : a.textContent;
        a.textContent = "Copied";
        var c = window.setTimeout(function() {
          a.textContent = i, n.delete(a);
        }, 1200);
        n.set(a, { label: i, timer: c });
      }, function() {
        prompt("Copy link", u);
      });
    });
  })();
})();
