(() => {
  function p(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function s(e, a) {
    try {
      return e?.getItem(a) ?? null;
    } catch {
      return null;
    }
  }
  function f(e, a, t) {
    try {
      if (t === null)
        e?.removeItem(a);
      else
        e?.setItem(a, t);
    } catch {}
  }
  function g(e, a, t) {
    let r = s(t, "airplan-color-mode");
    if (r === null) {
      let o = s(t, "airplan-theme");
      if (r = o === "light" || o === "dark" ? o : "system", r !== "system")
        f(t, "airplan-color-mode", r);
    }
    let h = p(r) ? r : "system", n = new Set(e.themes.map((o) => o.id)), i = s(t, "airplan-light-theme"), d = s(t, "airplan-dark-theme"), l = i !== null && n.has(i) ? i : e.defaultLight, m = d !== null && n.has(d) ? d : e.defaultDark;
    return y(e, h, l, m, a);
  }
  function y(e, a, t, r, h) {
    let n = new Map(e.themes.map((c) => [c.id, c])), i = n.has(t) ? t : e.defaultLight, d = n.has(r) ? r : e.defaultDark, l = a === "system" ? h ? "dark" : "light" : a, m = l === "light" ? i : d, o = n.get(m)?.variant ?? l;
    return { mode: a, resolvedMode: l, lightTheme: i, darkTheme: d, theme: m, variant: o };
  }
  function u(e) {
    return s(e, "airplan-fixed-navbar") !== "false";
  }

  try {
    let e = window.__AIRPLAN_THEME_CATALOG__;
    if (e) {
      let a;
      try {
        a = window.localStorage;
      } catch {}
      let t = g(e, matchMedia("(prefers-color-scheme: dark)").matches, a), r = document.documentElement;
      r.dataset.airplanMode = t.mode, r.dataset.airplanResolvedMode = t.resolvedMode, r.dataset.airplanTheme = t.theme, r.dataset.airplanThemeVariant = t.variant, r.dataset.airplanFixedNavbar = String(u(a)), window.__airplanThemeState = t;
    }
  } catch {}
})();
