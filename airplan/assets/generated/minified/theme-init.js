(() => {
  function T(_) {
    return _ === "system" || _ === "light" || _ === "dark";
  }
  function I(_, n) {
    try {
      return _?.getItem(n) ?? null;
    } catch {
      return null;
    }
  }
  function V(_, n, p) {
    try {
      if (p === null)
        _?.removeItem(n);
      else
        _?.setItem(n, p);
    } catch {}
  }
  function P(_, n, p) {
    let A = I(p, "airplan-color-mode");
    if (A === null) {
      let S = I(p, "airplan-theme");
      if (A = S === "light" || S === "dark" ? S : "system", A !== "system")
        V(p, "airplan-color-mode", A);
    }
    let N = T(A) ? A : "system", E = new Set(_.themes.map((S) => S.id)), C = I(p, "airplan-light-theme"), w = I(p, "airplan-dark-theme"), G = C !== null && E.has(C) ? C : _.defaultLight, H = w !== null && E.has(w) ? w : _.defaultDark;
    return b(_, N, G, H, n);
  }
  function b(_, n, p, A, N) {
    let E = new Map(_.themes.map((O) => [O.id, O])), C = E.has(p) ? p : _.defaultLight, w = E.has(A) ? A : _.defaultDark, G = n === "system" ? N ? "dark" : "light" : n, H = G === "light" ? C : w, S = E.get(H)?.variant ?? G;
    return { mode: n, resolvedMode: G, lightTheme: C, darkTheme: w, theme: H, variant: S };
  }

  try {
    let _ = window.__AIRPLAN_THEME_CATALOG__;
    if (_) {
      let n;
      try {
        n = window.localStorage;
      } catch {}
      let p = P(_, matchMedia("(prefers-color-scheme: dark)").matches, n), A = document.documentElement;
      A.dataset.airplanMode = p.mode, A.dataset.airplanResolvedMode = p.resolvedMode, A.dataset.airplanTheme = p.theme, A.dataset.airplanThemeVariant = p.variant, window.__airplanThemeState = p;
    }
  } catch {}
})();
