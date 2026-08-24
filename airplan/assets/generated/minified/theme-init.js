(() => {
  function u(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function m(e, o) {
    try {
      return e?.getItem(o) ?? null;
    } catch {
      return null;
    }
  }
  function p(e, o, t) {
    try {
      if (t === null)
        e?.removeItem(o);
      else
        e?.setItem(o, t);
    } catch {}
  }
  function g(e, o, t) {
    let r = m(t, "airplan-color-mode");
    if (r === null) {
      let n = m(t, "airplan-theme");
      if (r = n === "light" || n === "dark" ? n : "system", r !== "system")
        p(t, "airplan-color-mode", r);
    }
    let h = u(r) ? r : "system", a = new Set(e.themes.map((n) => n.id)), i = m(t, "airplan-light-theme"), d = m(t, "airplan-dark-theme"), l = i !== null && a.has(i) ? i : e.defaultLight, s = d !== null && a.has(d) ? d : e.defaultDark;
    return T(e, h, l, s, o);
  }
  function T(e, o, t, r, h) {
    let a = new Map(e.themes.map((c) => [c.id, c])), i = a.has(t) ? t : e.defaultLight, d = a.has(r) ? r : e.defaultDark, l = o === "system" ? h ? "dark" : "light" : o, s = l === "light" ? i : d, n = a.get(s)?.variant ?? l;
    return { mode: o, resolvedMode: l, lightTheme: i, darkTheme: d, theme: s, variant: n };
  }

  try {
    let e = window.__AIRPLAN_THEME_CATALOG__;
    if (e) {
      let o;
      try {
        o = window.localStorage;
      } catch {}
      let t = g(e, matchMedia("(prefers-color-scheme: dark)").matches, o), r = document.documentElement;
      r.dataset.airplanMode = t.mode, r.dataset.airplanResolvedMode = t.resolvedMode, r.dataset.airplanTheme = t.theme, r.dataset.airplanThemeVariant = t.variant, window.__airplanThemeState = t;
    }
  } catch {}
})();
