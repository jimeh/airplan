(() => {
  function P(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function T(e, i) {
    try {
      return e?.getItem(i) ?? null;
    } catch {
      return null;
    }
  }
  function E(e, i, l) {
    try {
      if (l === null)
        e?.removeItem(i);
      else
        e?.setItem(i, l);
    } catch {}
  }
  function x(e, i, l) {
    let o = T(l, "airplan-color-mode");
    if (o === null) {
      let p = T(l, "airplan-theme");
      if (o = p === "light" || p === "dark" ? p : "system", o !== "system")
        E(l, "airplan-color-mode", o);
    }
    let u = P(o) ? o : "system", d = new Set(e.themes.map((p) => p.id)), a = T(l, "airplan-light-theme"), c = T(l, "airplan-dark-theme"), n = a !== null && d.has(a) ? a : e.defaultLight, h = c !== null && d.has(c) ? c : e.defaultDark;
    return b(e, u, n, h, i);
  }
  function b(e, i, l, o, u) {
    let d = new Map(e.themes.map((v) => [v.id, v])), a = d.has(l) ? l : e.defaultLight, c = d.has(o) ? o : e.defaultDark, n = i === "system" ? u ? "dark" : "light" : i, h = n === "light" ? a : c, p = d.get(h)?.variant ?? n;
    return { mode: i, resolvedMode: n, lightTheme: a, darkTheme: c, theme: h, variant: p };
  }
  function A(e, i) {
    if (i === "system")
      E(e, "airplan-color-mode", null), E(e, "airplan-theme", null);
    else
      E(e, "airplan-color-mode", i), E(e, "airplan-theme", i);
  }
  function B(e, i, l) {
    E(e, i === "light" ? "airplan-light-theme" : "airplan-dark-theme", l);
  }
  function y(e) {
    return {
      mode: e.mode,
      resolvedMode: e.resolvedMode,
      theme: e.theme,
      variant: e.variant
    };
  }

  (function() {
    let e = document, i = e.documentElement;
    e.querySelectorAll(".js-only").forEach((t) => {
      t.hidden = !1;
    });
    let l = window.__AIRPLAN_THEME_CATALOG__;
    if (!l)
      return;
    let o = l, u = window.matchMedia("(prefers-color-scheme: dark)"), d;
    try {
      d = window.localStorage;
    } catch {}
    let a = window.__airplanThemeState ?? x(o, u.matches, d), c = e.querySelector("[data-airplan-appearance-trigger]"), n = e.querySelector("[data-airplan-appearance-panel]"), h = e.querySelector('select[data-airplan-theme-slot="light"]'), p = e.querySelector('select[data-airplan-theme-slot="dark"]'), v = Array.from(e.querySelectorAll("[data-airplan-color-mode]"));
    if (n)
      e.body.appendChild(n);
    function M(t) {
      if (!t || t.options.length > 0)
        return;
      for (let [r, f] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let m = e.createElement("optgroup");
        m.label = f;
        for (let s of o.themes) {
          if (s.variant !== r)
            continue;
          let L = e.createElement("option");
          L.value = s.id, L.textContent = s.name, m.append(L);
        }
        if (m.children.length > 0)
          t.append(m);
      }
    }
    M(h), M(p);
    function _(t, r = !0) {
      if (a = t, window.__airplanThemeState = a, i.dataset.airplanMode = a.mode, i.dataset.airplanResolvedMode = a.resolvedMode, i.dataset.airplanTheme = a.theme, i.dataset.airplanThemeVariant = a.variant, v.forEach((f) => {
        let m = f.dataset.airplanColorMode === a.mode;
        f.classList.toggle("active", m), f.setAttribute("aria-pressed", String(m));
      }), h)
        h.value = a.lightTheme;
      if (p)
        p.value = a.darkTheme;
      if (r)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: y(a) }));
    }
    function w(t = {}) {
      _(b(o, t.mode ?? a.mode, t.lightTheme ?? a.lightTheme, t.darkTheme ?? a.darkTheme, u.matches));
    }
    function S(t, r = !1) {
      if (!n || !c)
        return;
      if (t)
        C();
      if (n.hidden = !t, c.setAttribute("aria-expanded", String(t)), t)
        n.querySelector("button,select")?.focus();
      else if (r)
        c.focus();
    }
    function C() {
      if (!n || !c)
        return;
      let t = c.getBoundingClientRect(), r = c.closest(".toolbar")?.getBoundingClientRect(), f = e.documentElement.clientWidth, m = Math.min(304, f - 32), s = Math.max(16, f - t.right);
      n.style.setProperty("--airplan-appearance-top", `${(r?.bottom ?? t.bottom) + 8}px`), n.style.setProperty("--airplan-appearance-right", `${Math.min(s, Math.max(16, f - m - 16))}px`);
    }
    c?.addEventListener("click", () => S(Boolean(n?.hidden ?? !0))), v.forEach((t) => t.addEventListener("click", () => {
      let r = t.dataset.airplanColorMode;
      if (!r)
        return;
      A(d, r), w({ mode: r });
    }));
    function H(t, r) {
      B(d, t, r.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: r.value } })), w(t === "light" ? { lightTheme: r.value } : { darkTheme: r.value });
    }
    h?.addEventListener("change", () => H("light", h)), p?.addEventListener("change", () => H("dark", p)), u.addEventListener("change", () => {
      if (a.mode === "system")
        w();
    }), e.addEventListener("keydown", (t) => {
      if (t.key === "Escape" && n && !n.hidden)
        t.preventDefault(), S(!1, !0);
    }), e.addEventListener("pointerdown", (t) => {
      if (!n || n.hidden || !c)
        return;
      let r = t.target;
      if (!(r instanceof Node) || n.contains(r) || c.contains(r))
        return;
      let m = (r instanceof Element ? r : r.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), s = n.contains(e.activeElement) && !m;
      if (S(!1), s)
        setTimeout(() => {
          if (e.activeElement === e.body || n.contains(e.activeElement))
            c.focus();
        });
    }), window.addEventListener("resize", () => {
      if (n && !n.hidden)
        C();
    }), window.addEventListener("scroll", () => {
      if (n && !n.hidden)
        C();
    }), _(a, !1);
  })();

  (function() {
    var e = document, i = new WeakMap;
    e.addEventListener("click", function(l) {
      let o = l.target instanceof Element ? l.target.closest("[data-copy],[data-copy-overview]") : null;
      if (!o)
        return;
      var u = o.hasAttribute("data-copy-overview") ? location.href : new URL(o.dataset.copy || "", e.baseURI).href;
      if (!navigator.clipboard) {
        prompt("Copy link", u);
        return;
      }
      navigator.clipboard.writeText(u).then(function() {
        var d = i.get(o);
        if (d)
          window.clearTimeout(d.timer);
        var a = d ? d.label : o.textContent;
        o.textContent = "Copied";
        var c = window.setTimeout(function() {
          o.textContent = a, i.delete(o);
        }, 1200);
        i.set(o, { label: a, timer: c });
      }, function() {
        prompt("Copy link", u);
      });
    });
  })();
})();
