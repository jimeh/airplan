import {
  eventDetail,
  loadThemeState,
  persistMode,
  persistTheme,
  resolveThemeState,
  type ColorMode,
  type ResolvedMode,
  type ThemeCatalog,
  type ThemeState,
} from "./theme-state";

declare global {
  interface Window {
    __AIRPLAN_THEME_CATALOG__?: ThemeCatalog;
    __airplanThemeState?: ThemeState;
  }
  interface WindowEventMap {
    "airplan:themechange": CustomEvent<ReturnType<typeof eventDetail>>;
    "airplan:themeprepare": CustomEvent<{ theme: string }>;
  }
}

(function () {
  "use strict";
  const d = document;
  const root = d.documentElement;
  const embeddedCatalog = window.__AIRPLAN_THEME_CATALOG__;
  if (!embeddedCatalog) return;
  const catalog: ThemeCatalog = embeddedCatalog;
  const themeMedia = window.matchMedia("(prefers-color-scheme: dark)");
  let storage: Storage | undefined;
  try {
    storage = window.localStorage;
  } catch {}
  let state = window.__airplanThemeState ?? loadThemeState(catalog, themeMedia.matches, storage);

  d.querySelectorAll<HTMLElement>(".js-only").forEach((element) => {
    element.hidden = false;
  });

  const trigger = d.querySelector<HTMLButtonElement>("[data-airplan-appearance-trigger]");
  const panel = d.querySelector<HTMLElement>("[data-airplan-appearance-panel]");
  const lightSelect = d.querySelector<HTMLSelectElement>('select[data-airplan-theme-slot="light"]');
  const darkSelect = d.querySelector<HTMLSelectElement>('select[data-airplan-theme-slot="dark"]');
  const modeButtons = Array.from(
    d.querySelectorAll<HTMLButtonElement>("[data-airplan-color-mode]"),
  );

  function populateSelect(select: HTMLSelectElement | null): void {
    if (!select || select.options.length > 0) return;
    for (const [variant, label] of [
      ["light", "Light themes"],
      ["dark", "Dark themes"],
    ] as const) {
      const group = d.createElement("optgroup");
      group.label = label;
      for (const theme of catalog.themes) {
        if (theme.variant !== variant) continue;
        const option = d.createElement("option");
        option.value = theme.id;
        option.textContent = theme.name;
        group.append(option);
      }
      select.append(group);
    }
  }
  populateSelect(lightSelect);
  populateSelect(darkSelect);

  function apply(next: ThemeState, announce = true): void {
    state = next;
    window.__airplanThemeState = state;
    root.dataset.airplanMode = state.mode;
    root.dataset.airplanResolvedMode = state.resolvedMode;
    root.dataset.airplanTheme = state.theme;
    root.dataset.airplanThemeVariant = state.variant;
    modeButtons.forEach((button) => {
      const active = button.dataset.airplanColorMode === state.mode;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    });
    if (lightSelect) lightSelect.value = state.lightTheme;
    if (darkSelect) darkSelect.value = state.darkTheme;
    if (announce)
      window.dispatchEvent(new CustomEvent("airplan:themechange", { detail: eventDetail(state) }));
  }

  function recompute(
    overrides: Partial<Pick<ThemeState, "mode" | "lightTheme" | "darkTheme">> = {},
  ): void {
    apply(
      resolveThemeState(
        catalog,
        overrides.mode ?? state.mode,
        overrides.lightTheme ?? state.lightTheme,
        overrides.darkTheme ?? state.darkTheme,
        themeMedia.matches,
      ),
    );
  }

  function setPanel(open: boolean, restoreFocus = false): void {
    if (!panel || !trigger) return;
    panel.hidden = !open;
    trigger.setAttribute("aria-expanded", String(open));
    if (open) panel.querySelector<HTMLElement>("button,select")?.focus();
    else if (restoreFocus) trigger.focus();
  }

  trigger?.addEventListener("click", () => setPanel(Boolean(panel?.hidden ?? true)));
  modeButtons.forEach((button) =>
    button.addEventListener("click", () => {
      const mode = button.dataset.airplanColorMode as ColorMode | undefined;
      if (!mode) return;
      persistMode(storage, mode);
      recompute({ mode });
    }),
  );

  function selectChanged(slot: ResolvedMode, select: HTMLSelectElement): void {
    persistTheme(storage, slot, select.value);
    window.dispatchEvent(
      new CustomEvent("airplan:themeprepare", { detail: { theme: select.value } }),
    );
    recompute(slot === "light" ? { lightTheme: select.value } : { darkTheme: select.value });
  }
  lightSelect?.addEventListener("change", () => selectChanged("light", lightSelect));
  darkSelect?.addEventListener("change", () => selectChanged("dark", darkSelect));
  themeMedia.addEventListener("change", () => {
    if (state.mode === "system") recompute();
  });
  d.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && panel && !panel.hidden) {
      event.preventDefault();
      setPanel(false, true);
    }
  });
  d.addEventListener("pointerdown", (event) => {
    if (!panel || panel.hidden || !trigger) return;
    const target = event.target;
    if (!(target instanceof Node) || panel.contains(target) || trigger.contains(target)) return;
    const targetElement = target instanceof Element ? target : target.parentElement;
    const outsideFocusable = targetElement?.closest(
      'a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])',
    );
    const restoreFocus = panel.contains(d.activeElement) && !outsideFocusable;
    setPanel(false);
    if (restoreFocus) {
      setTimeout(() => {
        if (d.activeElement === d.body || panel.contains(d.activeElement)) trigger.focus();
      });
    }
  });
  apply(state, false);
})();
