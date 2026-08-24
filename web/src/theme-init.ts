import { loadFixedNavbar, loadThemeState, type ThemeCatalog, type ThemeState } from "./theme-state";

declare global {
  interface Window {
    __AIRPLAN_THEME_CATALOG__?: ThemeCatalog;
    __airplanThemeState?: ThemeState;
  }
}

try {
  const catalog = window.__AIRPLAN_THEME_CATALOG__;
  if (catalog) {
    let storage: Storage | undefined;
    try {
      storage = window.localStorage;
    } catch {}
    const state = loadThemeState(
      catalog,
      matchMedia("(prefers-color-scheme: dark)").matches,
      storage,
    );
    const root = document.documentElement;
    root.dataset.airplanMode = state.mode;
    root.dataset.airplanResolvedMode = state.resolvedMode;
    root.dataset.airplanTheme = state.theme;
    root.dataset.airplanThemeVariant = state.variant;
    root.dataset.airplanFixedNavbar = String(loadFixedNavbar(storage));
    window.__airplanThemeState = state;
  }
} catch {}
