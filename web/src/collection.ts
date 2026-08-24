import "./theme.ts";

(function () {
  "use strict";
  var d = document;
  var pendingRestores = new WeakMap<HTMLButtonElement, { label: string; timer: number }>();

  d.addEventListener("click", function (event) {
    const button =
      event.target instanceof Element
        ? event.target.closest<HTMLButtonElement>("[data-copy],[data-copy-overview]")
        : null;
    if (!button) return;
    var url = button.hasAttribute("data-copy-overview")
      ? location.href
      : new URL(button.dataset.copy || "", d.baseURI).href;
    if (!navigator.clipboard) {
      prompt("Copy link", url);
      return;
    }
    navigator.clipboard.writeText(url).then(
      function () {
        const label = button.querySelector<HTMLElement>(".action-label");
        if (!label) return;
        var pending = pendingRestores.get(button);
        if (pending) window.clearTimeout(pending.timer);
        var old = pending ? pending.label : label.textContent || "Copy link";
        label.textContent = "Copied";
        button.classList.add("is-copied");
        var timer = window.setTimeout(function () {
          label.textContent = old;
          button.classList.remove("is-copied");
          pendingRestores.delete(button);
        }, 1200);
        pendingRestores.set(button, { label: old, timer: timer });
      },
      function () {
        prompt("Copy link", url);
      },
    );
  });
})();
