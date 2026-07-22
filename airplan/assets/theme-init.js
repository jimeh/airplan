try {
  const theme = localStorage.getItem('airplan-theme');
  if (theme === 'light' || theme === 'dark') {
    document.documentElement.dataset.theme = theme;
  }
} catch (_) {}
