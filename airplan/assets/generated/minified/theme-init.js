(() => {
  try {
    let t = localStorage.getItem("airplan-theme");
    if (t === "light" || t === "dark")
      document.documentElement.dataset.theme = t;
  } catch {}
})();
