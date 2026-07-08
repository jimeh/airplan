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

  // Buttons are icon-only: feedback is an icon swap (check on
  // success, x on failure) plus the live-region announcement.
  function flash(btn, text, ok) {
    live.textContent = text;
    btn.classList.add(ok ? 'is-copied' : 'is-failed');
    btn.disabled = true;
    setTimeout(function () {
      btn.classList.remove('is-copied', 'is-failed');
      btn.disabled = false;
    }, 1200);
  }

  function copyText(text, btn) {
    if (!navigator.clipboard) {
      flash(btn, 'Copy failed', false);
      return;
    }
    navigator.clipboard.writeText(text).then(
      function () { flash(btn, 'Copied!', true); },
      function () { flash(btn, 'Copy failed', false); }
    );
  }

  // Octicons mirroring the toolbar's inline SVGs, for the
  // JS-created per-code-block buttons.
  var iconCopy = '<svg class="icon icon-copy" aria-hidden="true"' +
    ' viewBox="0 0 16 16" fill="currentColor"><path d="M0 6.75C0' +
    ' 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0' +
    '-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5' +
    'a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.7' +
    '5 0 0 1 0 14.25Z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5' +
    'C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1' +
    '.75 1.75 0 0 1 5 9.25Zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .13' +
    '8.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25' +
    '-.25Z"/></svg>';
  var iconCheck = '<svg class="icon icon-check" aria-hidden="true"' +
    ' viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22' +
    'a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28' +
    'a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6' +
    '.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>';
  var iconX = '<svg class="icon icon-x" aria-hidden="true"' +
    ' viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72' +
    'a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.32' +
    '6.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326' +
    ' 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0' +
    ' 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.7' +
    '5 0 0 1 0-1.06Z"/></svg>';

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
    btn.setAttribute('aria-label', 'Copy code');
    btn.title = 'Copy code';
    btn.innerHTML = iconCopy + iconCheck + iconX;
    btn.addEventListener('click', function () {
      var code = pre.querySelector('code');
      copyText((code || pre).textContent, btn);
    });
    wrap.appendChild(btn);
  });
})();
