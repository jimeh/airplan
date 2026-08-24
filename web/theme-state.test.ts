import { describe, expect, test } from "bun:test";

import {
  createLatestRequestGuard,
  darkThemeKey,
  eventDetail,
  fixedNavbarKey,
  legacyModeKey,
  lightThemeKey,
  loadThemeState,
  loadFixedNavbar,
  modeKey,
  persistMode,
  persistFixedNavbar,
  persistTheme,
  resolveThemeState,
  type StorageLike,
  type ThemeCatalog,
} from "./src/theme-state.ts";

const catalog: ThemeCatalog = {
  defaultLight: "light-a",
  defaultDark: "dark-a",
  themes: [
    { id: "light-a", name: "Light A", variant: "light" },
    { id: "dark-a", name: "Dark A", variant: "dark" },
  ],
};

class MemoryStorage implements StorageLike {
  values = new Map<string, string>();
  getItem(key: string) {
    return this.values.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    this.values.set(key, value);
  }
  removeItem(key: string) {
    this.values.delete(key);
  }
}

describe("reader theme state", () => {
  test("rejects stale asynchronous render requests", () => {
    const guard = createLatestRequestGuard();
    const first = guard.next();
    const second = guard.next();
    expect(guard.isCurrent(first)).toBe(false);
    expect(guard.isCurrent(second)).toBe(true);
  });

  test("resolves system and explicit modes independently from variants", () => {
    expect(resolveThemeState(catalog, "system", "dark-a", "light-a", false)).toMatchObject({
      resolvedMode: "light",
      theme: "dark-a",
      variant: "dark",
    });
    expect(resolveThemeState(catalog, "dark", "dark-a", "light-a", false)).toMatchObject({
      resolvedMode: "dark",
      theme: "light-a",
      variant: "light",
    });
  });

  test("uses page defaults without deleting unavailable stored IDs", () => {
    const storage = new MemoryStorage();
    storage.setItem(lightThemeKey, "custom-on-another-page");
    const state = loadThemeState(catalog, false, storage);
    expect(state.lightTheme).toBe("light-a");
    expect(storage.getItem(lightThemeKey)).toBe("custom-on-another-page");
  });

  test("seeds the new mode from the legacy key only when absent", () => {
    const storage = new MemoryStorage();
    storage.setItem(legacyModeKey, "dark");
    expect(loadThemeState(catalog, false, storage).mode).toBe("dark");
    expect(storage.getItem(modeKey)).toBe("dark");
    storage.setItem(modeKey, "light");
    storage.setItem(legacyModeKey, "dark");
    expect(loadThemeState(catalog, false, storage).mode).toBe("light");
    storage.setItem(modeKey, "future-mode");
    expect(loadThemeState(catalog, false, storage).mode).toBe("system");
    expect(storage.getItem(modeKey)).toBe("future-mode");
  });

  test("mirrors explicit legacy modes and removes both keys for system", () => {
    const storage = new MemoryStorage();
    persistMode(storage, "dark");
    expect(storage.getItem(modeKey)).toBe("dark");
    expect(storage.getItem(legacyModeKey)).toBe("dark");
    persistMode(storage, "system");
    expect(storage.getItem(modeKey)).toBeNull();
    expect(storage.getItem(legacyModeKey)).toBeNull();
  });

  test("persists each slot independently", () => {
    const storage = new MemoryStorage();
    persistTheme(storage, "light", "dark-a");
    persistTheme(storage, "dark", "light-a");
    expect(storage.getItem(lightThemeKey)).toBe("dark-a");
    expect(storage.getItem(darkThemeKey)).toBe("light-a");
    expect(loadThemeState(catalog, false, storage)).toMatchObject({
      lightTheme: "dark-a",
      darkTheme: "light-a",
      theme: "dark-a",
      variant: "dark",
    });
  });

  test("keeps the navbar fixed by default and stores only the opt-out", () => {
    const storage = new MemoryStorage();
    expect(loadFixedNavbar(storage)).toBe(true);
    persistFixedNavbar(storage, false);
    expect(storage.getItem(fixedNavbarKey)).toBe("false");
    expect(loadFixedNavbar(storage)).toBe(false);
    persistFixedNavbar(storage, true);
    expect(storage.getItem(fixedNavbarKey)).toBeNull();
    expect(loadFixedNavbar(storage)).toBe(true);
  });

  test("survives storage exceptions", () => {
    const broken: StorageLike = {
      getItem() {
        throw new Error("blocked");
      },
      setItem() {
        throw new Error("blocked");
      },
      removeItem() {
        throw new Error("blocked");
      },
    };
    expect(loadThemeState(catalog, true, broken)).toMatchObject({
      mode: "system",
      resolvedMode: "dark",
      theme: "dark-a",
    });
    expect(() => persistMode(broken, "light")).not.toThrow();
    expect(loadFixedNavbar(broken)).toBe(true);
    expect(() => persistFixedNavbar(broken, false)).not.toThrow();
  });

  test("creates the typed integration event detail", () => {
    const state = resolveThemeState(catalog, "light", "dark-a", "light-a", true);
    expect(eventDetail(state)).toEqual({
      mode: "light",
      resolvedMode: "light",
      theme: "dark-a",
      variant: "dark",
    });
  });
});
