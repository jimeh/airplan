(function () {
  'use strict';
  var d = document;

  // Reveal JS-dependent controls (hidden for no-JS readers).
  d.querySelectorAll('.js-only').forEach(function (el) {
    el.hidden = false;
  });

  // Theme preference is shared by airplan pages on the same origin.
  // System remains the default and follows operating-system changes.
  var root = d.documentElement;
  var themeMedia = window.matchMedia('(prefers-color-scheme: dark)');

  function selectedTheme() {
    return root.dataset.theme || 'system';
  }

  function resolvedTheme() {
    var selected = selectedTheme();
    if (selected !== 'system') return selected;
    return themeMedia.matches ? 'dark' : 'light';
  }

  function announceThemeChange() {
    window.dispatchEvent(new CustomEvent('airplan:themechange', {
      detail: { theme: resolvedTheme() }
    }));
  }

  function syncThemeButtons() {
    var selected = selectedTheme();
    d.querySelectorAll('.themetoggle button').forEach(function (button) {
      var active = button.dataset.theme === selected;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', active ? 'true' : 'false');
    });
  }

  d.querySelectorAll('.themetoggle button').forEach(function (button) {
    button.addEventListener('click', function () {
      var theme = button.dataset.theme;
      if (theme === 'system') {
        delete root.dataset.theme;
      } else {
        root.dataset.theme = theme;
      }
      try {
        if (theme === 'system') {
          localStorage.removeItem('airplan-theme');
        } else {
          localStorage.setItem('airplan-theme', theme);
        }
      } catch (_) {}
      syncThemeButtons();
      announceThemeChange();
    });
  });
  themeMedia.addEventListener('change', function () {
    if (selectedTheme() === 'system') announceThemeChange();
  });
  syncThemeButtons();
}());
