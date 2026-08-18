(() => {
  (function() {
    var a = document;
    a.querySelectorAll(".js-only").forEach(function(r) {
      r.hidden = !1;
    });
    var n = a.documentElement, i = window.matchMedia("(prefers-color-scheme: dark)");
    function o() {
      return n.dataset.theme || "system";
    }
    function f() {
      var r = o();
      if (r !== "system")
        return r;
      return i.matches ? "dark" : "light";
    }
    function t() {
      window.dispatchEvent(new CustomEvent("airplan:themechange", {
        detail: { theme: f() }
      }));
    }
    function c() {
      var r = o();
      a.querySelectorAll(".themetoggle button").forEach(function(p) {
        var v = p.dataset.theme === r;
        p.classList.toggle("active", v), p.setAttribute("aria-pressed", v ? "true" : "false");
      });
    }
    a.querySelectorAll(".themetoggle button").forEach(function(r) {
      r.addEventListener("click", function() {
        var p = r.dataset.theme;
        if (!p)
          return;
        if (p === "system")
          delete n.dataset.theme;
        else
          n.dataset.theme = p;
        try {
          if (p === "system")
            localStorage.removeItem("airplan-theme");
          else
            localStorage.setItem("airplan-theme", p);
        } catch {}
        c(), t();
      });
    }), i.addEventListener("change", function() {
      if (o() === "system")
        t();
    }), c();
  })();

  (function() {
    var a = document;
    a.addEventListener("click", function(n) {
      let i = n.target instanceof Element ? n.target.closest("[data-copy],[data-copy-overview]") : null;
      if (!i)
        return;
      var o = i.hasAttribute("data-copy-overview") ? location.href : new URL(i.dataset.copy || "", a.baseURI).href;
      if (!navigator.clipboard) {
        prompt("Copy link", o);
        return;
      }
      navigator.clipboard.writeText(o).then(function() {
        var f = i.textContent;
        i.textContent = "Copied", setTimeout(function() {
          i.textContent = f;
        }, 1200);
      }, function() {
        prompt("Copy link", o);
      });
    });
  })();
})();
