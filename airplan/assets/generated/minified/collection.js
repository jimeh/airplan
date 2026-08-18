(() => {
  (function() {
    var t = document;
    t.querySelectorAll(".js-only").forEach(function(r) {
      r.hidden = !1;
    });
    var a = t.documentElement, o = window.matchMedia("(prefers-color-scheme: dark)");
    function i() {
      return a.dataset.theme || "system";
    }
    function l() {
      var r = i();
      if (r !== "system")
        return r;
      return o.matches ? "dark" : "light";
    }
    function p() {
      window.dispatchEvent(new CustomEvent("airplan:themechange", {
        detail: { theme: l() }
      }));
    }
    function f() {
      var r = i();
      t.querySelectorAll(".themetoggle button").forEach(function(n) {
        var v = n.dataset.theme === r;
        n.classList.toggle("active", v), n.setAttribute("aria-pressed", v ? "true" : "false");
      });
    }
    t.querySelectorAll(".themetoggle button").forEach(function(r) {
      r.addEventListener("click", function() {
        var n = r.dataset.theme;
        if (!n)
          return;
        if (n === "system")
          delete a.dataset.theme;
        else
          a.dataset.theme = n;
        try {
          if (n === "system")
            localStorage.removeItem("airplan-theme");
          else
            localStorage.setItem("airplan-theme", n);
        } catch {}
        f(), p();
      });
    }), o.addEventListener("change", function() {
      if (i() === "system")
        p();
    }), f();
  })();

  (function() {
    var t = document, a = new WeakMap;
    t.addEventListener("click", function(o) {
      let i = o.target instanceof Element ? o.target.closest("[data-copy],[data-copy-overview]") : null;
      if (!i)
        return;
      var l = i.hasAttribute("data-copy-overview") ? location.href : new URL(i.dataset.copy || "", t.baseURI).href;
      if (!navigator.clipboard) {
        prompt("Copy link", l);
        return;
      }
      navigator.clipboard.writeText(l).then(function() {
        var p = a.get(i);
        if (p)
          window.clearTimeout(p.timer);
        var f = p ? p.label : i.textContent;
        i.textContent = "Copied";
        var r = window.setTimeout(function() {
          i.textContent = f, a.delete(i);
        }, 1200);
        a.set(i, { label: f, timer: r });
      }, function() {
        prompt("Copy link", l);
      });
    });
  })();
})();
