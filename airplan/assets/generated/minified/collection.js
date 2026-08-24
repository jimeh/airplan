(() => {
  function A(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function T(e, o) {
    try {
      return e?.getItem(o) ?? null;
    } catch {
      return null;
    }
  }
  function f(e, o, d) {
    try {
      if (d === null)
        e?.removeItem(o);
      else
        e?.setItem(o, d);
    } catch {}
  }
  function x(e, o, d) {
    let r = T(d, "airplan-color-mode");
    if (r === null) {
      let c = T(d, "airplan-theme");
      if (r = c === "light" || c === "dark" ? c : "system", r !== "system")
        f(d, "airplan-color-mode", r);
    }
    let m = A(r) ? r : "system", s = new Set(e.themes.map((c) => c.id)), a = T(d, "airplan-light-theme"), l = T(d, "airplan-dark-theme"), n = a !== null && s.has(a) ? a : e.defaultLight, u = l !== null && s.has(l) ? l : e.defaultDark;
    return L(e, m, n, u, o);
  }
  function L(e, o, d, r, m) {
    let s = new Map(e.themes.map((v) => [v.id, v])), a = s.has(d) ? d : e.defaultLight, l = s.has(r) ? r : e.defaultDark, n = o === "system" ? m ? "dark" : "light" : o, u = n === "light" ? a : l, c = s.get(u)?.variant ?? n;
    return { mode: o, resolvedMode: n, lightTheme: a, darkTheme: l, theme: u, variant: c };
  }
  function b(e, o) {
    if (o === "system")
      f(e, "airplan-color-mode", null), f(e, "airplan-theme", null);
    else
      f(e, "airplan-color-mode", o), f(e, "airplan-theme", o);
  }
  function R(e, o, d) {
    f(e, o === "light" ? "airplan-light-theme" : "airplan-dark-theme", d);
  }
  function _(e) {
    return {
      mode: e.mode,
      resolvedMode: e.resolvedMode,
      theme: e.theme,
      variant: e.variant
    };
  }

  (function() {
    let e = document, o = e.documentElement;
    e.querySelectorAll(".js-only").forEach((t) => {
      t.hidden = !1;
    });
    let d = window.__AIRPLAN_THEME_CATALOG__;
    if (!d)
      return;
    let r = d, m = window.matchMedia("(prefers-color-scheme: dark)"), s;
    try {
      s = window.localStorage;
    } catch {}
    let a = window.__airplanThemeState ?? x(r, m.matches, s), l = e.querySelector("[data-airplan-appearance-trigger]"), n = e.querySelector("[data-airplan-appearance-panel]"), u = e.querySelector('select[data-airplan-theme-slot="light"]'), c = e.querySelector('select[data-airplan-theme-slot="dark"]'), v = Array.from(e.querySelectorAll("[data-airplan-color-mode]"));
    if (n)
      e.body.appendChild(n);
    function C(t) {
      if (!t || t.options.length > 0)
        return;
      for (let [i, p] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let h = e.createElement("optgroup");
        h.label = p;
        for (let g of r.themes) {
          if (g.variant !== i)
            continue;
          let k = e.createElement("option");
          k.value = g.id, k.textContent = g.name, h.append(k);
        }
        if (h.children.length > 0)
          t.append(h);
      }
    }
    C(u), C(c);
    function S(t, i = !0) {
      if (a = t, window.__airplanThemeState = a, o.dataset.airplanMode = a.mode, o.dataset.airplanResolvedMode = a.resolvedMode, o.dataset.airplanTheme = a.theme, o.dataset.airplanThemeVariant = a.variant, v.forEach((p) => {
        let h = p.dataset.airplanColorMode === a.mode;
        p.classList.toggle("active", h), p.setAttribute("aria-pressed", String(h));
      }), u)
        u.value = a.lightTheme;
      if (c)
        c.value = a.darkTheme;
      if (i)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: _(a) }));
    }
    function y(t = {}) {
      S(L(r, t.mode ?? a.mode, t.lightTheme ?? a.lightTheme, t.darkTheme ?? a.darkTheme, m.matches));
    }
    function M(t, i = !1) {
      if (!n || !l)
        return;
      if (t)
        E();
      if (n.hidden = !t, l.setAttribute("aria-expanded", String(t)), t)
        n.querySelector("button,select")?.focus();
      else if (i)
        l.focus();
    }
    function E() {
      if (!n || !l)
        return;
      let t = l.getBoundingClientRect(), i = l.closest(".toolbar")?.getBoundingClientRect(), p = e.documentElement.clientWidth, h = Math.min(304, p - 32), g = Math.max(16, p - t.right);
      n.style.setProperty("--airplan-appearance-top", `${(i?.bottom ?? t.bottom) + 8}px`), n.style.setProperty("--airplan-appearance-right", `${Math.min(g, Math.max(16, p - h - 16))}px`);
    }
    l?.addEventListener("click", () => M(Boolean(n?.hidden ?? !0))), v.forEach((t) => t.addEventListener("click", () => {
      let i = t.dataset.airplanColorMode;
      if (!i)
        return;
      b(s, i), y({ mode: i });
    }));
    function w(t, i) {
      R(s, t, i.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: i.value } })), y(t === "light" ? { lightTheme: i.value } : { darkTheme: i.value });
    }
    u?.addEventListener("change", () => w("light", u)), c?.addEventListener("change", () => w("dark", c)), m.addEventListener("change", () => {
      if (a.mode === "system")
        y();
    }), e.addEventListener("keydown", (t) => {
      if (t.key === "Escape" && n && !n.hidden)
        t.preventDefault(), M(!1, !0);
    }), e.addEventListener("pointerdown", (t) => {
      if (!n || n.hidden || !l)
        return;
      let i = t.target;
      if (!(i instanceof Node) || n.contains(i) || l.contains(i))
        return;
      let h = (i instanceof Element ? i : i.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), g = n.contains(e.activeElement) && !h;
      if (M(!1), g)
        setTimeout(() => {
          if (e.activeElement === e.body || n.contains(e.activeElement))
            l.focus();
        });
    }), window.addEventListener("resize", () => {
      if (n && !n.hidden)
        E();
    }), window.addEventListener("scroll", () => {
      if (n && !n.hidden)
        E();
    }), S(a, !1);
  })();

  (function() {
    var e = document, o = new WeakMap;
    e.addEventListener("click", function(d) {
      let r = d.target instanceof Element ? d.target.closest("[data-copy],[data-copy-overview]") : null;
      if (!r)
        return;
      var m = r.hasAttribute("data-copy-overview") ? location.href : new URL(r.dataset.copy || "", e.baseURI).href;
      if (!navigator.clipboard) {
        prompt("Copy link", m);
        return;
      }
      navigator.clipboard.writeText(m).then(function() {
        let s = r.querySelector(".action-label");
        if (!s)
          return;
        var a = o.get(r);
        if (a)
          window.clearTimeout(a.timer);
        var l = a ? a.label : s.textContent || "Copy link";
        s.textContent = "Copied", r.classList.add("is-copied");
        var n = window.setTimeout(function() {
          s.textContent = l, r.classList.remove("is-copied"), o.delete(r);
        }, 1200);
        o.set(r, { label: l, timer: n });
      }, function() {
        prompt("Copy link", m);
      });
    });
  })();
})();
