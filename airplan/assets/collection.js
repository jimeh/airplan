(function () {
  'use strict';
  var d = document;

  d.addEventListener('click', function (event) {
    var button = event.target.closest('[data-copy],[data-copy-overview]');
    if (!button) return;
    var url = button.hasAttribute('data-copy-overview')
      ? location.href
      : new URL(button.dataset.copy, d.baseURI).href;
    if (!navigator.clipboard) {
      prompt('Copy link', url);
      return;
    }
    navigator.clipboard.writeText(url).then(function () {
      var old = button.textContent;
      button.textContent = 'Copied';
      setTimeout(function () { button.textContent = old; }, 1200);
    }, function () {
      prompt('Copy link', url);
    });
  });
}());
