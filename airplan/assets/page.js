(function () {
  'use strict';
  var d = document;

  // Reveal JS-dependent controls (hidden for no-JS readers).
  d.querySelectorAll('.js-only').forEach(function (el) {
    el.hidden = false;
  });

  // Visually-hidden live region so screen readers announce copy
  // feedback even when focus doesn't stay on the button.
  var live = d.createElement('div');
  live.className = 'sr-status';
  live.setAttribute('aria-live', 'polite');
  d.body.appendChild(live);

  function flash(btn, text) {
    var old = btn.textContent;
    btn.textContent = text;
    live.textContent = text;
    btn.disabled = true;
    setTimeout(function () {
      btn.textContent = old;
      btn.disabled = false;
    }, 1200);
  }

  function copyText(text, btn) {
    if (!navigator.clipboard) {
      flash(btn, 'Copy failed');
      return;
    }
    navigator.clipboard.writeText(text).then(
      function () { flash(btn, 'Copied!'); },
      function () { flash(btn, 'Copy failed'); }
    );
  }

  // Rendered/source toggle.
  var rendered = d.getElementById('rendered');
  var source = d.getElementById('source');
  d.querySelectorAll('.viewtoggle button').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var showSource = btn.dataset.view === 'source';
      source.hidden = !showSource;
      rendered.hidden = showSource;
      d.querySelectorAll('.viewtoggle button').forEach(function (b) {
        b.classList.toggle('active', b === btn);
        b.setAttribute('aria-pressed', b === btn ? 'true' : 'false');
      });
    });
  });

  // Copy the full original source. The highlighted block's text
  // content preserves the raw source exactly.
  var copySource = d.querySelector('.copy-source');
  if (copySource && source) {
    copySource.addEventListener('click', function () {
      var pre = source.querySelector('pre');
      copyText(pre ? pre.textContent : '', copySource);
    });
  }

  // Per-code-block copy buttons in the rendered view.
  rendered.querySelectorAll('pre').forEach(function (pre) {
    var wrap = d.createElement('div');
    wrap.className = 'codewrap';
    pre.parentNode.insertBefore(wrap, pre);
    wrap.appendChild(pre);

    var btn = d.createElement('button');
    btn.className = 'codecopy';
    btn.type = 'button';
    btn.textContent = 'Copy';
    btn.addEventListener('click', function () {
      var code = pre.querySelector('code');
      copyText((code || pre).textContent, btn);
    });
    wrap.appendChild(btn);
  });
})();
