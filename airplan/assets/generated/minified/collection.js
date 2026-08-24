(() => {
  function q(e) {
    return e === "system" || e === "light" || e === "dark";
  }
  function y(e, n) {
    try {
      return e?.getItem(n) ?? null;
    } catch {
      return null;
    }
  }
  function v(e, n, d) {
    try {
      if (d === null)
        e?.removeItem(n);
      else
        e?.setItem(n, d);
    } catch {}
  }
  function w(e, n, d) {
    let r = y(d, "airplan-color-mode");
    if (r === null) {
      let c = y(d, "airplan-theme");
      if (r = c === "light" || c === "dark" ? c : "system", r !== "system")
        v(d, "airplan-color-mode", r);
    }
    let m = q(r) ? r : "system", s = new Set(e.themes.map((c) => c.id)), o = y(d, "airplan-light-theme"), l = y(d, "airplan-dark-theme"), a = o !== null && s.has(o) ? o : e.defaultLight, p = l !== null && s.has(l) ? l : e.defaultDark;
    return k(e, m, a, p, n);
  }
  function k(e, n, d, r, m) {
    let s = new Map(e.themes.map((g) => [g.id, g])), o = s.has(d) ? d : e.defaultLight, l = s.has(r) ? r : e.defaultDark, a = n === "system" ? m ? "dark" : "light" : n, p = a === "light" ? o : l, c = s.get(p)?.variant ?? a;
    return { mode: n, resolvedMode: a, lightTheme: o, darkTheme: l, theme: p, variant: c };
  }
  function R(e, n) {
    if (n === "system")
      v(e, "airplan-color-mode", null), v(e, "airplan-theme", null);
    else
      v(e, "airplan-color-mode", n), v(e, "airplan-theme", n);
  }
  function K(e, n, d) {
    v(e, n === "light" ? "airplan-light-theme" : "airplan-dark-theme", d);
  }
  function _(e) {
    return y(e, "airplan-fixed-navbar") !== "false";
  }
  function A(e, n) {
    v(e, "airplan-fixed-navbar", n ? null : "false");
  }
  function H(e) {
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
    let d = window.__AIRPLAN_THEME_CATALOG__;
    if (!d)
      return;
    let r = d, m = window.matchMedia("(prefers-color-scheme: dark)"), s;
    try {
      s = window.localStorage;
    } catch {}
    let o = window.__airplanThemeState ?? w(r, m.matches, s), l = e.querySelector("[data-airplan-appearance-trigger]"), a = e.querySelector("[data-airplan-appearance-panel]"), p = e.querySelector('select[data-airplan-theme-slot="light"]'), c = e.querySelector('select[data-airplan-theme-slot="dark"]'), g = e.querySelector("input[data-airplan-fixed-navbar]"), L = Array.from(e.querySelectorAll("[data-airplan-color-mode]"));
    if (a)
      e.body.appendChild(a);
    function b(t) {
      if (!t || t.options.length > 0)
        return;
      for (let [i, u] of [
        ["light", "Light themes"],
        ["dark", "Dark themes"]
      ]) {
        let h = e.createElement("optgroup");
        h.label = u;
        for (let f of r.themes) {
          if (f.variant !== i)
            continue;
          let T = e.createElement("option");
          T.value = f.id, T.textContent = f.name, h.append(T);
        }
        if (h.children.length > 0)
          t.append(h);
      }
    }
    b(p), b(c);
    function S(t, i = !0) {
      if (o = t, window.__airplanThemeState = o, n.dataset.airplanMode = o.mode, n.dataset.airplanResolvedMode = o.resolvedMode, n.dataset.airplanTheme = o.theme, n.dataset.airplanThemeVariant = o.variant, L.forEach((u) => {
        let h = u.dataset.airplanColorMode === o.mode;
        u.classList.toggle("active", h), u.setAttribute("aria-pressed", String(h));
      }), p)
        p.value = o.lightTheme;
      if (c)
        c.value = o.darkTheme;
      if (i)
        window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: H(o) }));
    }
    function E(t = {}) {
      S(k(r, t.mode ?? o.mode, t.lightTheme ?? o.lightTheme, t.darkTheme ?? o.darkTheme, m.matches));
    }
    function x(t, i = !1) {
      if (!a || !l)
        return;
      if (t)
        M();
      if (a.hidden = !t, l.setAttribute("aria-expanded", String(t)), t)
        a.querySelector("button,select,input")?.focus();
      else if (i)
        l.focus();
    }
    function M() {
      if (!a || !l)
        return;
      let t = l.getBoundingClientRect(), i = l.closest(".toolbar")?.getBoundingClientRect(), u = e.documentElement.clientWidth, h = Math.min(304, u - 32), f = Math.max(16, u - t.right), T = (i?.bottom ?? t.bottom) + 8;
      a.style.setProperty("--airplan-appearance-top", `${Math.max(16, T)}px`), a.style.setProperty("--airplan-appearance-right", `${Math.min(f, Math.max(16, u - h - 16))}px`);
    }
    l?.addEventListener("click", () => x(Boolean(a?.hidden ?? !0))), L.forEach((t) => t.addEventListener("click", () => {
      let i = t.dataset.airplanColorMode;
      if (!i)
        return;
      R(s, i), E({ mode: i });
    }));
    function C(t, i) {
      K(s, t, i.value), window.dispatchEvent(new CustomEvent("airplan:themeprepare", { detail: { theme: i.value } })), E(t === "light" ? { lightTheme: i.value } : { darkTheme: i.value });
    }
    if (p?.addEventListener("change", () => C("light", p)), c?.addEventListener("change", () => C("dark", c)), g?.addEventListener("change", () => {
      let t = g.checked;
      A(s, t), n.dataset.airplanFixedNavbar = String(t), window.dispatchEvent(new CustomEvent("airplan:navbarchange", { detail: { fixed: t } })), M();
    }), m.addEventListener("change", () => {
      if (o.mode === "system")
        E();
    }), e.addEventListener("keydown", (t) => {
      if (t.key === "Escape" && a && !a.hidden)
        t.preventDefault(), x(!1, !0);
    }), e.addEventListener("pointerdown", (t) => {
      if (!a || a.hidden || !l)
        return;
      let i = t.target;
      if (!(i instanceof Node) || a.contains(i) || l.contains(i))
        return;
      let h = (i instanceof Element ? i : i.parentElement)?.closest('a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])'), f = a.contains(e.activeElement) && !h;
      if (x(!1), f)
        setTimeout(() => {
          if (e.activeElement === e.body || a.contains(e.activeElement))
            l.focus();
        });
    }), window.addEventListener("resize", () => {
      if (a && !a.hidden)
        M();
    }), window.addEventListener("scroll", () => {
      if (a && !a.hidden)
        M();
    }), g)
      g.checked = _(s);
    S(o, !1);
  })();

  (function() {
    var e = document, n = new WeakMap;
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
        var o = n.get(r);
        if (o)
          window.clearTimeout(o.timer);
        var l = o ? o.label : s.textContent || "Copy link";
        s.textContent = "Copied", r.classList.add("is-copied");
        var a = window.setTimeout(function() {
          s.textContent = l, r.classList.remove("is-copied"), n.delete(r);
        }, 1200);
        n.set(r, { label: l, timer: a });
      }, function() {
        prompt("Copy link", m);
      });
    });
  })();
})();
