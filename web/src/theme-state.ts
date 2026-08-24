export type ColorMode = "system" | "light" | "dark";
export type ResolvedMode = "light" | "dark";
export type ThemeVariant = "light" | "dark";

export interface BrowserTheme {
  id: string;
  name: string;
  variant: ThemeVariant;
}

export interface MermaidThemeCatalog {
  themes: Record<string, Record<string, string | boolean>>;
  print: Record<string, string | boolean>;
}

export interface ThemeCatalog {
  defaultLight: string;
  defaultDark: string;
  themes: BrowserTheme[];
}

export interface ThemeState {
  mode: ColorMode;
  resolvedMode: ResolvedMode;
  lightTheme: string;
  darkTheme: string;
  theme: string;
  variant: ThemeVariant;
}

export interface AirplanThemeChangeDetail {
  mode: ColorMode;
  resolvedMode: ResolvedMode;
  theme: string;
  variant: ThemeVariant;
}

export interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

export interface LatestRequestGuard {
  next(): number;
  isCurrent(request: number): boolean;
}

export const modeKey = "airplan-color-mode";
export const lightThemeKey = "airplan-light-theme";
export const darkThemeKey = "airplan-dark-theme";
export const legacyModeKey = "airplan-theme";
export const fixedNavbarKey = "airplan-fixed-navbar";

export function createLatestRequestGuard(): LatestRequestGuard {
  let latest = 0;
  return {
    next: () => ++latest,
    isCurrent: (request) => request === latest,
  };
}

export function isColorMode(value: string | null): value is ColorMode {
  return value === "system" || value === "light" || value === "dark";
}

function safelyRead(storage: StorageLike | undefined, key: string): string | null {
  try {
    return storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function safelyWrite(storage: StorageLike | undefined, key: string, value: string | null): void {
  try {
    if (value === null) storage?.removeItem(key);
    else storage?.setItem(key, value);
  } catch {}
}

export function loadThemeState(
  catalog: ThemeCatalog,
  systemDark: boolean,
  storage?: StorageLike,
): ThemeState {
  let storedMode = safelyRead(storage, modeKey);
  if (storedMode === null) {
    const legacy = safelyRead(storage, legacyModeKey);
    storedMode = legacy === "light" || legacy === "dark" ? legacy : "system";
    if (storedMode !== "system") safelyWrite(storage, modeKey, storedMode);
  }
  const mode: ColorMode = isColorMode(storedMode) ? storedMode : "system";
  const known = new Set(catalog.themes.map((theme) => theme.id));
  const storedLight = safelyRead(storage, lightThemeKey);
  const storedDark = safelyRead(storage, darkThemeKey);
  // Unknown IDs intentionally remain persisted. They may be valid on a
  // different immutable Airplan page sharing this origin.
  const lightTheme =
    storedLight !== null && known.has(storedLight) ? storedLight : catalog.defaultLight;
  const darkTheme = storedDark !== null && known.has(storedDark) ? storedDark : catalog.defaultDark;
  return resolveThemeState(catalog, mode, lightTheme, darkTheme, systemDark);
}

export function resolveThemeState(
  catalog: ThemeCatalog,
  mode: ColorMode,
  lightTheme: string,
  darkTheme: string,
  systemDark: boolean,
): ThemeState {
  const known = new Map(catalog.themes.map((theme) => [theme.id, theme]));
  const safeLight = known.has(lightTheme) ? lightTheme : catalog.defaultLight;
  const safeDark = known.has(darkTheme) ? darkTheme : catalog.defaultDark;
  const resolvedMode: ResolvedMode = mode === "system" ? (systemDark ? "dark" : "light") : mode;
  const theme = resolvedMode === "light" ? safeLight : safeDark;
  const variant = known.get(theme)?.variant ?? resolvedMode;
  return { mode, resolvedMode, lightTheme: safeLight, darkTheme: safeDark, theme, variant };
}

export function persistMode(storage: StorageLike | undefined, mode: ColorMode): void {
  if (mode === "system") {
    safelyWrite(storage, modeKey, null);
    safelyWrite(storage, legacyModeKey, null);
  } else {
    safelyWrite(storage, modeKey, mode);
    safelyWrite(storage, legacyModeKey, mode);
  }
}

export function persistTheme(
  storage: StorageLike | undefined,
  slot: ResolvedMode,
  id: string,
): void {
  safelyWrite(storage, slot === "light" ? lightThemeKey : darkThemeKey, id);
}

export function loadFixedNavbar(storage?: StorageLike): boolean {
  return safelyRead(storage, fixedNavbarKey) !== "false";
}

export function persistFixedNavbar(storage: StorageLike | undefined, fixed: boolean): void {
  safelyWrite(storage, fixedNavbarKey, fixed ? null : "false");
}

export function eventDetail(state: ThemeState): AirplanThemeChangeDetail {
  return {
    mode: state.mode,
    resolvedMode: state.resolvedMode,
    theme: state.theme,
    variant: state.variant,
  };
}
