(function () {
  'use strict';
  var d = document;

  // Markdown pages are revision-ready even while standalone. The control
  // object intentionally does not exist until revision 2 is uploaded, so a
  // 404 is the ordinary dormant state. A nonce and no-store prevent a cached
  // negative response from hiding linkage added after this page was opened.
  var versionsMeta = d.querySelector('meta[name="airplan-versions"]');
  var currentPathParts = window.location.pathname.split('/').filter(Boolean);
  var servicePathParts = currentPathParts.slice(0, -2);
  function safeRevisionURL(raw, diff) {
    if (typeof raw !== 'string') return null;
    try {
      var parsed = new URL(raw);
      if (parsed.origin !== window.location.origin || parsed.username ||
          parsed.password || parsed.search || parsed.hash) return null;
      var parts = parsed.pathname.split('/').filter(Boolean);
      if (parts.length !== servicePathParts.length + 2 ||
          !servicePathParts.every(function (part, index) {
            return parts[index] === part;
          }) || !/^[a-z2-7]{26}$/.test(parts[parts.length - 2])) {
        return null;
      }
      var name = parts[parts.length - 1];
      if (diff ? name !== '.airplan-changes.diff' : !name.endsWith('.html')) {
        return null;
      }
      return parsed.href;
    } catch (_) {
      return null;
    }
  }
  function renderVersions(metadata) {
    var context = d.querySelector('.revision-context');
    if (!context) throw new Error('revision context is unavailable');
    var currentMeta = d.querySelector('meta[name="airplan-revision"]');
    var embedded = currentMeta ? Number(currentMeta.content) :
      Number(metadata.current_revision);
    if (!Number.isInteger(embedded) || embedded <= 0 ||
        metadata.current_revision !== embedded ||
        !Number.isInteger(metadata.latest_revision) ||
        !Number.isInteger(metadata.last_assigned_revision) ||
        !Array.isArray(metadata.revisions) || metadata.revisions.length === 0 ||
        metadata.last_assigned_revision !== metadata.revisions.length ||
        !/^[a-z2-7]{26}$/.test(metadata.chain_id)) {
      throw new Error('revision identity is invalid');
    }
    var invalidEntry = false;
    var previousNumber = 0;
    var live = metadata.revisions.filter(function (revision) {
      if (!revision || !Number.isInteger(revision.number) ||
          revision.number !== previousNumber + 1) {
        invalidEntry = true;
        return false;
      }
      previousNumber = revision.number;
      if (revision.deleted) return false;
      revision.safeURL = safeRevisionURL(revision.url, false);
      if (!revision.safeURL) {
        invalidEntry = true;
        return false;
      }
      if (revision.number > 1) {
        var safeDiff = safeRevisionURL(revision.diff_url, true);
        if (!safeDiff || new URL(safeDiff).pathname.replace(/[^/]+$/, '') !==
            new URL(revision.safeURL).pathname.replace(/[^/]+$/, '')) {
          invalidEntry = true;
          return false;
        }
      }
      return true;
    });
    if (invalidEntry || metadata.revisions[0].number !== 1 ||
        !live.some(function (revision) {
      return revision.number === embedded;
    })) throw new Error('revision entries are invalid');
    var current = live.find(function (revision) {
      return revision.number === embedded;
    });
    var currentPage = window.location.origin + window.location.pathname;
    if (!current || current.safeURL !== currentPage) {
      throw new Error('current revision URL is invalid');
    }
    var latest = Math.max.apply(null, live.map(function (revision) {
      return revision.number;
    }));
    if (latest !== metadata.latest_revision) throw new Error('latest is invalid');

    if (live.length < 2) return;

    context.replaceChildren();
    context.hidden = false;
    context.classList.remove('js-only');
    var previous = live.filter(function (revision) {
      return revision.number < embedded;
    }).pop();
    var next = live.find(function (revision) {
      return revision.number > embedded;
    });
    function navigation(label, revision, className) {
      var link = d.createElement('a');
      link.className = className;
      link.textContent = label;
      link.href = revision.safeURL;
      return link;
    }
    if (previous) context.appendChild(navigation('Previous', previous, 'revision-previous'));
    var label = d.createElement('label');
    label.textContent = 'Revision ';
    var select = d.createElement('select');
    select.setAttribute('aria-label', 'Document revision');
    live.forEach(function (revision) {
      var option = d.createElement('option');
      option.value = revision.safeURL;
      option.textContent = revision.number + ' of ' + latest;
      option.selected = revision.number === embedded;
      select.appendChild(option);
    });
    select.addEventListener('change', function () {
      var selected = select.selectedIndex;
      if (selected < 0 || selected >= live.length) return;
      window.location.assign(live[selected].safeURL);
    });
    label.appendChild(select);
    context.appendChild(label);
    if (next) context.appendChild(navigation('Next', next, 'revision-next'));
    var heading = d.querySelector('[data-revision-heading]');
    if (!heading) {
      heading = d.createElement('p');
      heading.className = 'revision-heading';
      heading.setAttribute('data-revision-heading', '');
      var renderedView = d.getElementById('rendered');
      if (renderedView) renderedView.prepend(heading);
    }
    if (heading) heading.textContent = 'Revision ' + embedded + ' of ' + latest;
    if (embedded < latest) {
      d.body.classList.add('airplan-stale-revision');
      context.appendChild(navigation('Latest: revision ' + latest,
        live.find(function (revision) { return revision.number === latest; }),
        'revision-latest'));
      if (heading) {
        heading.classList.add('is-stale');
        heading.textContent += ' — an older revision';
      }
    }
  }
  if (versionsMeta) {
    var versionsURL = new URL(versionsMeta.content, window.location.href);
    versionsURL.searchParams.set('_airplan',
      Date.now().toString(36) + Math.random().toString(36).slice(2));
    fetch(versionsURL, { cache: 'no-store', credentials: 'same-origin' })
      .then(function (response) {
        if (response.status === 404) return null;
        if (!response.ok) throw new Error('metadata request failed');
        return response.json();
      })
      .then(function (metadata) {
        if (metadata === null) return;
        if (!metadata || metadata.schema !== 'airplan-versions' ||
            metadata.version !== 1 ||
            !Array.isArray(metadata.revisions) ||
            metadata.revisions.length < 2) {
          throw new Error('metadata is invalid');
        }
        renderVersions(metadata);
        window.dispatchEvent(new CustomEvent('airplan:versions', {
          detail: metadata,
        }));
      })
      .catch(function () {
        console.warn('airplan: revision metadata is unavailable or invalid');
      });
  }

  // Visually-hidden live region so screen readers announce copy
  // feedback even when focus doesn't stay on the button.
  var live = d.createElement('div');
  live.className = 'sr-status';
  live.setAttribute('aria-live', 'polite');
  d.body.appendChild(live);

  // Printing should never omit collapsed disclosure content. Restore the
  // reader's open/closed state after the print dialog finishes.
  var printClosedDetails = null;
  function expandDetailsForPrint() {
    if (printClosedDetails !== null) return;
    printClosedDetails = Array.from(d.querySelectorAll('details:not([open])'));
    printClosedDetails.forEach(function (details) {
      details.open = true;
    });
  }
  function restoreDetailsAfterPrint() {
    if (printClosedDetails === null) return;
    printClosedDetails.forEach(function (details) {
      details.open = false;
    });
    printClosedDetails = null;
  }
  window.addEventListener('beforeprint', expandDetailsForPrint);
  window.addEventListener('afterprint', restoreDetailsAfterPrint);

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
  var changes = d.getElementById('changes');
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
      var view = btn.dataset.view;
      rendered.hidden = view !== 'rendered';
      if (source) source.hidden = view !== 'source';
      if (changes) changes.hidden = view !== 'changes';
      if (toc) toc.hidden = view !== 'rendered';
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
