(() => {
  function P(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function T(e, a) {
    try {
      return e?.getItem(a) ?? null;
    } catch {
      return null;
    }
  }
  function E(e, a, c) {
    try {
      if (c === null)
        e?.removeItem(a);
      else
        e?.setItem(a, c);
    } catch {}
  }
  function x(e, a, c) {
    let o = T(c, "airplan-color-mode");
    if (o === null) {
      let p = T(c, "airplan-theme");
      if (o = p === "light" || p === "dark" ? p : "system", o !== "system")
        E(c, "airplan-color-mode", o);
    }
    let m = P(o) ? o : "system", d = new Set(e.themes.map((p) => p.id)), i = T(c, "airplan-light-theme"), r = T(c, "airplan-dark-theme"), n = i !== null && d.has(i) ? i : e.defaultLight, h = r !== null && d.has(r) ? r : e.defaultDark;
    return C(e, m, n, h, a);
  }
  function C(e, a, c, o, m) {
    let d = new Map(e.themes.map((v) => [v.id, v])), i = d.has(c) ? c : e.defaultLight, r = d.has(o) ? o : e.defaultDark, n = a === "system" ? m ? "dark" : "light" : a, h = n === "light" ? i : r, p = d.get(h)?.variant ?? n;
    return { mode: a, resolvedMode: n, lightTheme: i, darkTheme: r, theme: h, variant: p };
  }
  function A(e, a) {
    if (a === "system")
      E(e, "airplan-color-mode", null), E(e, "airplan-theme", null);
    else
      E(e, "airplan-color-mode", a), E(e, "airplan-theme", a);
  }
  function y(e, a, c) {
    E(e, a === "light" ? "airplan-light-theme" : "airplan-dark-theme", c);
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
    let e = document, a = e.documentElement;
    e.querySelectorAll(".js-only").forEach((t) => {
      t.hidden = !1;
    });
    let c = window.__AIRPLAN_THEME_CATALOG__;
    if (!c)
      return;
    let o = c, m = window.matchMedia("(prefers-color-scheme: dark)"), d;
    try {
      d = window.localStorage;
    } catch {}
    let i = window.__airplanThemeState ?? x(o, m.matches, d), r = e.querySelector("[data-airplan-appearance-trigger]"), n = e.querySelector("[data-airplan-appearance-panel]"), h = e.querySelector('select[data-airplan-theme-slot="light"]'), p = e.querySelector('select[data-airplan-theme-slot="dark"]'), v = Array.from(e.querySelectorAll("[data-airplan-color-mode]"));
    if (n)
      e.body.appendChild(n);
    function M(t) {
      if (!t || t.options.length > 0)
        return;
      for (let [l, s] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let u = e.createElement("optgroup");
        u.label = s;
        for (let f of o.themes) {
          if (f.variant !== l)
            continue;
          let b = e.createElement("option");
          b.value = f.id, b.textContent = f.name, u.append(b);
        }
        if (u.children.length > 0)
          t.append(u);
      }
    }
    M(h), M(p);
    function _(t, l = !0) {
      if (i = t, window.__airplanThemeState = i, a.dataset.airplanMode = i.mode, a.dataset.airplanResolvedMode = i.resolvedMode, a.dataset.airplanTheme = i.theme, a.dataset.airplanThemeVariant = i.variant, v.forEach((s) => {
        let u = s.dataset.airplanColorMode === i.mode;
        s.classList.toggle("active", u), s.setAttribute("aria-pressed", String(u));
      }), h)
        h.value = i.lightTheme;
      if (p)
        p.value = i.darkTheme;
      if (l)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: B(i) }));
    }
    function w(t = {}) {
      _(C(o, t.mode ?? i.mode, t.lightTheme ?? i.lightTheme, t.darkTheme ?? i.darkTheme, m.matches));
    }
    function L(t, l = !1) {
      if (!n || !r)
        return;
      if (t)
        S();
      if (n.hidden = !t, r.setAttribute("aria-expanded", String(t)), t)
        n.querySelector("button,select")?.focus();
      else if (l)
        r.focus();
    }
    function S() {
      if (!n || !r)
        return;
      let t = r.getBoundingClientRect(), l = r.closest(".toolbar")?.getBoundingClientRect(), s = e.documentElement.clientWidth, u = Math.min(304, s - 32), f = Math.max(16, s - t.right);
      n.style.setProperty("--airplan-appearance-top", `${(l?.bottom ?? t.bottom) + 8}px`), n.style.setProperty("--airplan-appearance-right", `${Math.min(f, Math.max(16, s - u - 16))}px`);
    }
    r?.addEventListener("click", () => L(Boolean(n?.hidden ?? !0))), v.forEach((t) => t.addEventListener("click", () => {
      let l = t.dataset.airplanColorMode;
      if (!l)
        return;
      A(d, l), w({ mode: l });
    }));
    function H(t, l) {
      y(d, t, l.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: l.value } })), w(t === "light" ? { lightTheme: l.value } : { darkTheme: l.value });
    }
    h?.addEventListener("change", () => H("light", h)), p?.addEventListener("change", () => H("dark", p)), m.addEventListener("change", () => {
      if (i.mode === "system")
        w();
    }), e.addEventListener("keydown", (t) => {
      if (t.key === "Escape" && n && !n.hidden)
        t.preventDefault(), L(!1, !0);
    }), e.addEventListener("pointerdown", (t) => {
      if (!n || n.hidden || !r)
        return;
      let l = t.target;
      if (!(l instanceof Node) || n.contains(l) || r.contains(l))
        return;
      let u = (l instanceof Element ? l : l.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), f = n.contains(e.activeElement) && !u;
      if (L(!1), f)
        setTimeout(() => {
          if (e.activeElement === e.body || n.contains(e.activeElement))
            r.focus();
        });
    }), window.addEventListener("resize", () => {
      if (n && !n.hidden)
        S();
    }), window.addEventListener("scroll", () => {
      if (n && !n.hidden)
        S();
    }), _(i, !1);
  })();

  (function() {
    var e = document, a = new WeakMap;
    e.addEventListener("click", function(c) {
      let o = c.target instanceof Element ? c.target.closest("[data-copy],[data-copy-overview]") : null;
      if (!o)
        return;
      var m = o.hasAttribute("data-copy-overview") ? location.href : new URL(o.dataset.copy || "", e.baseURI).href;
      if (!navigator.clipboard) {
        prompt("Copy link", m);
        return;
      }
      navigator.clipboard.writeText(m).then(function() {
        let d = o.querySelector(".action-label");
        if (!d)
          return;
        var i = a.get(o);
        if (i)
          window.clearTimeout(i.timer);
        var r = i ? i.label : d.textContent || "Copy link";
        d.textContent = "Copied", o.classList.add("is-copied");
        var n = window.setTimeout(function() {
          d.textContent = r, o.classList.remove("is-copied"), a.delete(o);
        }, 1200);
        a.set(o, { label: r, timer: n });
      }, function() {
        prompt("Copy link", m);
      });
    });
  })();
})();
