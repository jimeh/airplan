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
    var label = btn.querySelector('.action-label');
    var previousLabel = label ? label.textContent : '';
    if (label) label.textContent = ok ? 'Copied' : 'Failed';
    btn.classList.add(ok ? 'is-copied' : 'is-failed');
    btn.disabled = true;
    setTimeout(function () {
      btn.classList.remove('is-copied', 'is-failed');
      btn.disabled = false;
      if (label) label.textContent = previousLabel;
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
  var iconToc = '<svg class="icon" aria-hidden="true"' +
    ' viewBox="0 0 16 16" fill="none" stroke="currentColor"' +
    ' stroke-width="1.5" stroke-linecap="round">' +
    '<path d="M5 4h9M5 8h9M5 12h9"/>' +
    '<circle cx="2" cy="4" r=".75" fill="currentColor" stroke="none"/>' +
    '<circle cx="2" cy="8" r=".75" fill="currentColor" stroke="none"/>' +
    '<circle cx="2" cy="12" r=".75" fill="currentColor"' +
    ' stroke="none"/></svg>';
  var iconClose = '<svg class="icon" aria-hidden="true"' +
    ' viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72' +
    'a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1' +
    ' 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749' +
    '.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3' +
    '.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1' +
    '.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>';

  // Rendered/source toggle.
  var rendered = d.getElementById('rendered');
  var source = d.getElementById('source');
  var toc = d.getElementById('toc');
  var tocTrigger = null;
  var tocDialog = null;
  var tocMedia = window.matchMedia('(max-width: 78rem)');

  function closeTocDialog() {
    if (tocDialog && tocDialog.open) tocDialog.close();
  }

  function syncTocTrigger() {
    if (!toc || !tocTrigger || !tocDialog) return;
    var show = tocMedia.matches && !rendered.hidden &&
      toc.getBoundingClientRect().bottom < 0 && !tocDialog.open;
    tocTrigger.classList.toggle('is-visible', show);
    tocTrigger.tabIndex = show ? 0 : -1;
    tocTrigger.setAttribute('aria-hidden', show ? 'false' : 'true');
    if (tocDialog.open && (!tocMedia.matches || rendered.hidden)) {
      closeTocDialog();
    }
  }

  d.querySelectorAll('.viewtoggle button').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var showSource = btn.dataset.view === 'source';
      source.hidden = !showSource;
      rendered.hidden = showSource;
      if (toc) toc.hidden = showSource;
      d.querySelectorAll('.viewtoggle button').forEach(function (b) {
        b.classList.toggle('active', b === btn);
        b.setAttribute('aria-pressed', b === btn ? 'true' : 'false');
      });
      syncTocTrigger();
    });
  });

  // Highlight the ToC entry nearest the top of the viewport. Links and
  // hierarchy are rendered server-side, so navigation still works when
  // JavaScript is disabled.
  if (toc) {
    var tocLinks = Array.from(toc.querySelectorAll('a[href^="#"]'));
    var tocList = toc.querySelector('.toc-list');

    if (tocList) {
      tocDialog = d.createElement('dialog');
      if (typeof tocDialog.showModal === 'function') {
        tocDialog.className = 'toc-dialog';
        tocDialog.id = 'toc-dialog';
        tocDialog.setAttribute('aria-labelledby', 'toc-dialog-title');

        var tocPanel = d.createElement('div');
        tocPanel.className = 'toc-dialog-panel';
        var tocHeader = d.createElement('div');
        tocHeader.className = 'toc-dialog-header';
        var tocTitle = d.createElement('h2');
        tocTitle.className = 'toc-dialog-title';
        tocTitle.id = 'toc-dialog-title';
        tocTitle.textContent = 'Contents';
        var tocClose = d.createElement('button');
        tocClose.className = 'toc-dialog-close';
        tocClose.type = 'button';
        tocClose.setAttribute('aria-label', 'Close table of contents');
        tocClose.innerHTML = iconClose;
        tocHeader.appendChild(tocTitle);
        tocHeader.appendChild(tocClose);

        var tocNav = d.createElement('nav');
        tocNav.className = 'toc-dialog-nav';
        tocNav.setAttribute('aria-label', 'Table of contents');
        tocNav.appendChild(tocList.cloneNode(true));
        tocPanel.appendChild(tocHeader);
        tocPanel.appendChild(tocNav);
        tocDialog.appendChild(tocPanel);

        tocTrigger = d.createElement('button');
        tocTrigger.className = 'toc-trigger';
        tocTrigger.type = 'button';
        tocTrigger.tabIndex = -1;
        tocTrigger.setAttribute('aria-label', 'Open table of contents');
        tocTrigger.setAttribute('aria-controls', 'toc-dialog');
        tocTrigger.setAttribute('aria-haspopup', 'dialog');
        tocTrigger.setAttribute('aria-hidden', 'true');
        tocTrigger.innerHTML = iconToc;

        d.body.appendChild(tocTrigger);
        d.body.appendChild(tocDialog);

        tocTrigger.addEventListener('click', function () {
          tocDialog.showModal();
          d.body.classList.add('toc-dialog-open');
          syncTocTrigger();
          var active = tocDialog.querySelector('a.active');
          if (active) active.scrollIntoView({ block: 'nearest' });
        });
        tocClose.addEventListener('click', closeTocDialog);
        tocDialog.addEventListener('click', function (event) {
          if (event.target === tocDialog) closeTocDialog();
        });
        tocDialog.addEventListener('keydown', function (event) {
          if (event.key === 'Escape') {
            event.preventDefault();
            closeTocDialog();
          }
        });
        tocDialog.addEventListener('close', function () {
          d.body.classList.remove('toc-dialog-open');
          syncTocTrigger();
          if (tocTrigger.classList.contains('is-visible')) {
            // Let the dialog leave the top layer before restoring focus.
            setTimeout(function () {
              tocTrigger.focus();
            }, 50);
          }
        });
        tocNav.querySelectorAll('a').forEach(function (link) {
          link.addEventListener('click', closeTocDialog);
        });
      } else {
        tocDialog = null;
      }
    }

    var allTocLinks = tocLinks.slice();
    if (tocDialog) {
      allTocLinks = allTocLinks.concat(
        Array.from(tocDialog.querySelectorAll('a[href^="#"]'))
      );
    }
    var tocHeadings = tocLinks.map(function (link) {
      return d.getElementById(link.getAttribute('href').slice(1));
    });
    function updateToc() {
      var current = 0;
      tocHeadings.forEach(function (heading, index) {
        if (heading && heading.getBoundingClientRect().top <= 128) {
          current = index;
        }
      });
      if (window.innerHeight + window.scrollY >=
          d.documentElement.scrollHeight - 2) {
        current = tocLinks.length - 1;
      }
      var activeHref = tocLinks[current].getAttribute('href');
      allTocLinks.forEach(function (link) {
        var active = link.getAttribute('href') === activeHref;
        link.classList.toggle('active', active);
        if (active) {
          link.setAttribute('aria-current', 'location');
        } else {
          link.removeAttribute('aria-current');
        }
      });
      syncTocTrigger();
    }
    var tocFramePending = false;
    function scheduleTocUpdate() {
      if (tocFramePending) return;
      tocFramePending = true;
      window.requestAnimationFrame(function () {
        tocFramePending = false;
        updateToc();
      });
    }
    d.addEventListener('scroll', scheduleTocUpdate, { passive: true });
    window.addEventListener('resize', updateToc);
    updateToc();
  }

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
    if (pre.classList.contains('mermaid')) return;
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
