import {
  createLatestRequestGuard,
  type AirplanThemeChangeDetail,
  type MermaidThemeCatalog,
  type ThemeCatalog,
} from "./theme-state";

import { iconDiagramClose, iconDiagramOpen } from "../../airplan/assets/generated/icons.ts";

interface MermaidRenderResult {
  bindFunctions?: (element: Element) => void;
  svg: string;
}

interface MermaidAPI {
  initialize(options: Record<string, unknown>): void;
  render(id: string, source: string | null): Promise<MermaidRenderResult>;
}

interface MermaidViewer {
  enhance(block: HTMLPreElement): void;
}

declare global {
  interface Window {
    __AIRPLAN_MERMAID_THEMES__?: MermaidThemeCatalog;
  }
}

const diagrams = Array.from(document.querySelectorAll<HTMLPreElement>("pre.mermaid"));
const mermaidModuleURL = "__AIRPLAN_MERMAID_MODULE_URL__";
const sources = diagrams.map((diagram) => diagram.textContent);
const variants = diagrams.map(() => new Map<string, MermaidRenderResult>());
const failedVariants = diagrams.map(() => new Set<string>());
const catalog = window.__AIRPLAN_THEME_CATALOG__ as ThemeCatalog | undefined;
const mermaidCatalog = window.__AIRPLAN_MERMAID_THEMES__ as MermaidThemeCatalog | undefined;
const printMedia = matchMedia("print");
const printThemeKey = "__airplan-print-github-light";
let mermaidCloneID = 0;
let viewer: MermaidViewer | null = null;
let visibleTheme = window.__airplanThemeState?.theme ?? catalog?.defaultLight ?? "github-light";
const lastScreenThemes = diagrams.map(() => visibleTheme);
let printActive = printMedia.matches;
let prepareTheme: ((themeID: string, show?: boolean) => Promise<void>) | undefined;
const pendingThemes = new Set<string>();
const requestGuard = createLatestRequestGuard();

function showVariant(index: number, theme: string, trackScreen = true): boolean {
  const rendered = variants[index].get(theme);
  if (!rendered) return false;
  diagrams[index].innerHTML = rendered.svg;
  if (rendered.bindFunctions) rendered.bindFunctions(diagrams[index]);
  if (viewer) viewer.enhance(diagrams[index]);
  if (trackScreen && theme !== printThemeKey) lastScreenThemes[index] = theme;
  return true;
}

function showTheme(theme: string): boolean {
  let complete = true;
  variants.forEach((_themes, index) => {
    if (!showVariant(index, theme)) complete = false;
  });
  return complete;
}

function restoreScreenTheme(): void {
  variants.forEach((_themes, index) => {
    if (!showVariant(index, visibleTheme)) {
      showVariant(index, lastScreenThemes[index]);
    }
  });
}

function requestPreparation(themeID: string, show = true): void {
  if (prepareTheme) {
    void prepareTheme(themeID, show);
  } else {
    pendingThemes.add(themeID);
  }
}

window.addEventListener("airplan:themechange", (event) => {
  const detail = (event as CustomEvent<AirplanThemeChangeDetail>).detail;
  visibleTheme = detail.theme;
  if (!printActive) requestPreparation(detail.theme);
});
window.addEventListener("airplan:themeprepare", (event) => {
  const detail = (event as CustomEvent<{ theme: string }>).detail;
  requestPreparation(detail.theme, false);
});
printMedia.addEventListener("change", (event) => {
  printActive = event.matches;
  if (printActive) showTheme(printThemeKey);
  else {
    requestPreparation(visibleTheme);
    restoreScreenTheme();
  }
});
window.addEventListener("beforeprint", () => {
  printActive = true;
  showTheme(printThemeKey);
});
window.addEventListener("afterprint", () => {
  printActive = false;
  requestPreparation(visibleTheme);
  restoreScreenTheme();
});

function createButton(label: string, action: string, content: string) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "mermaid-button";
  button.setAttribute(`data-airplan-mermaid-${action}`, "");
  button.setAttribute("aria-label", label);
  button.title = label;
  button.innerHTML = content;
  return button;
}

interface SVGGeometry {
  x: number;
  y: number;
  width: number;
  height: number;
}

function readSVGGeometry(svg: SVGSVGElement): SVGGeometry {
  const values = svg
    .getAttribute("viewBox")
    ?.trim()
    .split(/[\s,]+/)
    .map(Number);
  if (
    values?.length === 4 &&
    Number.isFinite(values[0]) &&
    Number.isFinite(values[1]) &&
    Number.isFinite(values[2]) &&
    values[2] > 0 &&
    Number.isFinite(values[3]) &&
    values[3] > 0
  ) {
    return { x: values[0], y: values[1], width: values[2], height: values[3] };
  }
  const width = Number.parseFloat(svg.getAttribute("width") || "");
  const height = Number.parseFloat(svg.getAttribute("height") || "");
  return {
    x: 0,
    y: 0,
    width: Number.isFinite(width) && width > 0 ? width : 800,
    height: Number.isFinite(height) && height > 0 ? height : 600,
  };
}

function cloneMermaidSVG(svg: SVGSVGElement): SVGSVGElement {
  const clone = svg.cloneNode(true) as SVGSVGElement;
  const prefix = `airplan-mermaid-viewer-${mermaidCloneID++}-`;
  const ids = new Map<string, string>();
  const elements: Element[] = [clone, ...clone.querySelectorAll("*")];

  elements.forEach((element) => {
    const original = element.getAttribute("id");
    if (!original) return;
    const replacement = `${prefix}${original}`;
    ids.set(original, replacement);
    element.setAttribute("id", replacement);
  });

  elements.forEach((element) => {
    Array.from(element.attributes).forEach((attribute) => {
      let value = attribute.value;
      ids.forEach((replacement, original) => {
        value = value.replaceAll(`url(#${original})`, `url(#${replacement})`);
        if (value === `#${original}`) value = `#${replacement}`;
      });
      if (attribute.name === "aria-labelledby" || attribute.name === "aria-describedby") {
        value = value
          .split(/\s+/)
          .map((id) => ids.get(id) || id)
          .join(" ");
      }
      if (value !== attribute.value) {
        element.setAttribute(attribute.name, value);
      }
    });
  });

  const idsBySpecificity = Array.from(ids).sort(([left], [right]) => right.length - left.length);
  clone.querySelectorAll<HTMLStyleElement>("style").forEach((style) => {
    let css = style.textContent || "";
    idsBySpecificity.forEach(([original, replacement]) => {
      const escaped = original.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const token = new RegExp(`#${escaped}(?![\\w-])`, "g");
      css = css.replace(token, () => `#${replacement}`);
    });
    style.textContent = css;
  });
  return clone;
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(Math.max(value, minimum), maximum);
}

function createMermaidViewer(): MermaidViewer | null {
  const dialog = document.createElement("dialog");
  if (typeof dialog.showModal !== "function") return null;

  dialog.id = "airplan-mermaid-dialog";
  dialog.className = "mermaid-dialog";
  dialog.setAttribute("data-airplan-mermaid-dialog", "");
  dialog.setAttribute("aria-labelledby", "airplan-mermaid-dialog-title");

  const shell = document.createElement("div");
  shell.className = "mermaid-dialog-shell";
  const header = document.createElement("header");
  header.className = "mermaid-dialog-header";
  const title = document.createElement("strong");
  title.id = "airplan-mermaid-dialog-title";
  title.textContent = "Mermaid diagram";
  header.appendChild(title);

  const toolbar = document.createElement("div");
  toolbar.className = "mermaid-toolbar";
  toolbar.setAttribute("role", "toolbar");
  toolbar.setAttribute("aria-label", "Diagram zoom controls");
  const zoomOut = createButton("Zoom out", "zoom-out", "&minus;");
  const zoomValue = document.createElement("output");
  zoomValue.className = "mermaid-zoom-value";
  zoomValue.setAttribute("data-airplan-mermaid-zoom-value", "");
  zoomValue.setAttribute("aria-live", "polite");
  zoomValue.textContent = "100%";
  const zoomIn = createButton("Zoom in", "zoom-in", "+");
  const fit = createButton("Fit diagram", "fit", "Fit");
  const closeButton = createButton("Close diagram viewer", "close", iconDiagramClose);
  toolbar.append(zoomOut, zoomValue, zoomIn, fit, closeButton);
  header.appendChild(toolbar);

  const canvas = document.createElement("div");
  canvas.className = "mermaid-canvas";
  canvas.setAttribute("data-airplan-mermaid-canvas", "");
  canvas.tabIndex = 0;
  canvas.setAttribute("role", "group");
  canvas.setAttribute(
    "aria-label",
    "Zoomable Mermaid diagram. Scroll, pinch, or use plus and minus to zoom. " +
      "Drag or use arrow keys to pan.",
  );
  const surface = document.createElement("div");
  surface.className = "mermaid-surface";
  surface.setAttribute("data-airplan-mermaid-surface", "");
  canvas.appendChild(surface);

  const help = document.createElement("p");
  help.className = "mermaid-help";
  help.textContent = "Scroll or pinch to zoom · Drag to pan · 0 to fit";
  shell.append(header, canvas, help);
  dialog.appendChild(shell);
  document.body.appendChild(dialog);

  let sourceBlock: HTMLPreElement | null = null;
  let returnFocus: HTMLButtonElement | null = null;
  let viewerSVG: SVGSVGElement | null = null;
  let geometry: SVGGeometry = { x: 0, y: 0, width: 1, height: 1 };
  let scale = 1;
  let panX = 0;
  let panY = 0;
  const activePointers = new Map<number, { x: number; y: number }>();
  let pointerStartX = 0;
  let pointerStartY = 0;
  let pointerPanX = 0;
  let pointerPanY = 0;
  let pinchDistance = 0;
  let pinchMidpointX = 0;
  let pinchMidpointY = 0;

  function applyViewBox() {
    zoomValue.textContent = `${Math.round(scale * 100)}%`;
    if (!viewerSVG) return;
    const bounds = canvas.getBoundingClientRect();
    const viewWidth = Math.max(bounds.width, 1) / scale;
    const viewHeight = Math.max(bounds.height, 1) / scale;
    const centerX = geometry.x + geometry.width / 2 + panX;
    const centerY = geometry.y + geometry.height / 2 + panY;
    viewerSVG.setAttribute(
      "viewBox",
      [centerX - viewWidth / 2, centerY - viewHeight / 2, viewWidth, viewHeight].join(" "),
    );
  }

  function fitDiagram() {
    const bounds = canvas.getBoundingClientRect();
    const availableWidth = Math.max(bounds.width - 48, 1);
    const availableHeight = Math.max(bounds.height - 48, 1);
    scale = clamp(
      Math.min(availableWidth / geometry.width, availableHeight / geometry.height, 1),
      0.05,
      8,
    );
    panX = 0;
    panY = 0;
    applyViewBox();
  }

  function setZoomBetweenPoints(
    nextScale: number,
    fromClientX: number,
    fromClientY: number,
    toClientX: number,
    toClientY: number,
  ) {
    const clamped = clamp(nextScale, 0.05, 8);
    const bounds = canvas.getBoundingClientRect();
    const centerX = bounds.left + bounds.width / 2;
    const centerY = bounds.top + bounds.height / 2;
    panX += (fromClientX - centerX) / scale - (toClientX - centerX) / clamped;
    panY += (fromClientY - centerY) / scale - (toClientY - centerY) / clamped;
    scale = clamped;
    applyViewBox();
  }

  function setZoom(nextScale: number, clientX?: number, clientY?: number) {
    if (clientX !== undefined && clientY !== undefined) {
      setZoomBetweenPoints(nextScale, clientX, clientY, clientX, clientY);
      return;
    }
    const clamped = clamp(nextScale, 0.05, 8);
    if (clamped === scale) return;
    scale = clamped;
    applyViewBox();
  }

  function beginSinglePointerPan(pointer: { x: number; y: number }) {
    pointerStartX = pointer.x;
    pointerStartY = pointer.y;
    pointerPanX = panX;
    pointerPanY = panY;
  }

  function beginPinch() {
    const [first, second] = Array.from(activePointers.values());
    if (!first || !second) {
      pinchDistance = 0;
      return;
    }
    pinchDistance = Math.hypot(second.x - first.x, second.y - first.y);
    pinchMidpointX = (first.x + second.x) / 2;
    pinchMidpointY = (first.y + second.y) / 2;
  }

  function showSVG(svg: SVGSVGElement, reset: boolean) {
    const clone = cloneMermaidSVG(svg);
    viewerSVG = clone;
    geometry = readSVGGeometry(svg);
    clone.style.width = "100%";
    clone.style.height = "100%";
    clone.style.maxWidth = "none";
    clone.style.maxHeight = "none";
    surface.replaceChildren(clone);
    if (reset) fitDiagram();
    else applyViewBox();
  }

  if (typeof ResizeObserver === "function") {
    new ResizeObserver(() => applyViewBox()).observe(canvas);
  } else {
    window.addEventListener("resize", applyViewBox);
  }

  function closeViewer() {
    if (dialog.open) dialog.close();
  }

  function openViewer(
    block: HTMLPreElement,
    trigger: HTMLButtonElement,
    pointerInitiated: boolean,
  ) {
    const svg = block.querySelector<SVGSVGElement>("svg");
    if (!svg) return;
    sourceBlock = block;
    returnFocus = trigger;
    showSVG(svg, false);
    dialog.showModal();
    document.body.classList.add("mermaid-dialog-open");
    fitDiagram();
    canvas.classList.toggle("mermaid-canvas-pointer-focus", pointerInitiated);
    canvas.focus();
  }

  zoomOut.addEventListener("click", () => setZoom(scale / 1.25));
  zoomIn.addEventListener("click", () => setZoom(scale * 1.25));
  fit.addEventListener("click", fitDiagram);
  closeButton.addEventListener("click", closeViewer);
  dialog.addEventListener("cancel", (event) => {
    event.preventDefault();
    closeViewer();
  });
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) closeViewer();
  });
  dialog.addEventListener("close", () => {
    document.body.classList.remove("mermaid-dialog-open");
    activePointers.clear();
    canvas.classList.remove("mermaid-canvas-panning", "mermaid-canvas-pointer-focus");
    const target = returnFocus;
    sourceBlock = null;
    returnFocus = null;
    if (target?.isConnected) {
      setTimeout(() => target.focus(), 0);
    }
  });
  dialog.addEventListener("keydown", (event) => {
    canvas.classList.remove("mermaid-canvas-pointer-focus");
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.key === "+" || event.key === "=") {
      event.preventDefault();
      setZoom(scale * 1.25);
      return;
    }
    if (event.key === "-") {
      event.preventDefault();
      setZoom(scale / 1.25);
      return;
    }
    if (event.key === "0") {
      event.preventDefault();
      fitDiagram();
      return;
    }
    if (event.target !== canvas) return;
    const panStep = event.shiftKey ? 120 : 40;
    const direction = {
      ArrowLeft: [-panStep, 0],
      ArrowRight: [panStep, 0],
      ArrowUp: [0, -panStep],
      ArrowDown: [0, panStep],
    }[event.key];
    if (!direction) return;
    event.preventDefault();
    panX += direction[0] / scale;
    panY += direction[1] / scale;
    applyViewBox();
  });
  canvas.addEventListener(
    "wheel",
    (event) => {
      event.preventDefault();
      const deltaMultiplier =
        event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? canvas.clientHeight : 1;
      const delta = clamp(event.deltaY * deltaMultiplier, -500, 500);
      setZoom(scale * Math.exp(-delta * 0.001), event.clientX, event.clientY);
    },
    { passive: false },
  );
  canvas.addEventListener("pointerdown", (event) => {
    if (event.button !== 0 || activePointers.has(event.pointerId) || activePointers.size >= 2) {
      return;
    }
    activePointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
    canvas.classList.add("mermaid-canvas-panning");
    canvas.classList.add("mermaid-canvas-pointer-focus");
    try {
      canvas.setPointerCapture(event.pointerId);
    } catch {}
    if (activePointers.size === 1) {
      beginSinglePointerPan({ x: event.clientX, y: event.clientY });
    } else {
      beginPinch();
    }
  });
  canvas.addEventListener("pointermove", (event) => {
    const pointer = activePointers.get(event.pointerId);
    if (!pointer) return;
    pointer.x = event.clientX;
    pointer.y = event.clientY;
    if (activePointers.size === 1) {
      panX = pointerPanX - (event.clientX - pointerStartX) / scale;
      panY = pointerPanY - (event.clientY - pointerStartY) / scale;
      applyViewBox();
      return;
    }
    const [first, second] = Array.from(activePointers.values());
    if (!first || !second) return;
    const nextDistance = Math.hypot(second.x - first.x, second.y - first.y);
    const nextMidpointX = (first.x + second.x) / 2;
    const nextMidpointY = (first.y + second.y) / 2;
    if (pinchDistance > 0 && nextDistance > 0) {
      setZoomBetweenPoints(
        scale * (nextDistance / pinchDistance),
        pinchMidpointX,
        pinchMidpointY,
        nextMidpointX,
        nextMidpointY,
      );
    }
    pinchDistance = nextDistance;
    pinchMidpointX = nextMidpointX;
    pinchMidpointY = nextMidpointY;
  });
  function stopPanning(event: PointerEvent) {
    if (!activePointers.has(event.pointerId)) return;
    try {
      canvas.releasePointerCapture(event.pointerId);
    } catch {}
    activePointers.delete(event.pointerId);
    if (activePointers.size === 0) {
      pinchDistance = 0;
      canvas.classList.remove("mermaid-canvas-panning");
      return;
    }
    const remaining = activePointers.values().next().value;
    if (remaining) beginSinglePointerPan(remaining);
  }
  canvas.addEventListener("pointerup", stopPanning);
  canvas.addEventListener("pointercancel", stopPanning);

  return {
    enhance(block: HTMLPreElement) {
      const svg = block.querySelector<SVGSVGElement>("svg");
      if (!svg) return;
      let trigger = block.querySelector<HTMLButtonElement>(":scope > [data-airplan-mermaid-open]");
      if (!trigger) {
        trigger = createButton("Open diagram viewer", "open", iconDiagramOpen);
        trigger.classList.add("mermaid-open");
        trigger.setAttribute("aria-haspopup", "dialog");
        trigger.setAttribute("aria-controls", dialog.id);
        const activeTrigger = trigger;
        trigger.addEventListener("click", (event) =>
          openViewer(block, activeTrigger, event.detail > 0),
        );
        block.appendChild(trigger);
      }
      if (sourceBlock === block && dialog.open) {
        returnFocus = trigger;
        showSVG(svg, false);
      }
    },
  };
}

try {
  if (!catalog || !mermaidCatalog) throw new Error("theme catalog is unavailable");
  const palettes = mermaidCatalog;
  const { default: mermaid } = (await import(mermaidModuleURL)) as {
    default: MermaidAPI;
  };
  let queue = Promise.resolve();
  const preparations = new Map<string, Promise<void>>();

  function themePrepared(key: string): boolean {
    return variants.every((cache, index) => cache.has(key) || failedVariants[index].has(key));
  }

  function renderTheme(themeID: string, key = themeID): Promise<void> {
    if (themePrepared(key)) return Promise.resolve();
    const existing = preparations.get(key);
    if (existing) return existing;
    const variables = key === printThemeKey ? palettes.print : palettes.themes[themeID];
    if (!variables) return Promise.resolve();
    const task = queue.then(async () => {
      if (themePrepared(key)) return;
      try {
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: "strict",
          secure: ["theme", "themeVariables", "themeCSS", "darkMode"],
          theme: "base",
          themeVariables: variables,
        });
      } catch (error) {
        failedVariants.forEach((failures, index) => {
          failures.add(key);
          if (variants[index].size === 0) {
            diagrams[index].textContent = sources[index];
            diagrams[index].classList.add("mermaid-failed");
          }
        });
        console.warn(`airplan: Mermaid ${themeID} initialization failed`, error);
        return;
      }
      for (const [index, source] of sources.entries()) {
        if (variants[index].has(key) || failedVariants[index].has(key)) continue;
        try {
          const rendered = await mermaid.render(
            `airplan-mermaid-${key.replace(/[^a-z0-9-]/g, "-")}-${index}`,
            source,
          );
          variants[index].set(key, rendered);
          const diagram = diagrams[index];
          diagram.classList.add("mermaid-rendered");
          diagram.classList.remove("mermaid-failed");
          if (!viewer) viewer = createMermaidViewer();
        } catch (error) {
          failedVariants[index].add(key);
          if (variants[index].size === 0) {
            diagrams[index].textContent = source;
            diagrams[index].classList.add("mermaid-failed");
          }
          console.warn(`airplan: Mermaid ${themeID} diagram ${index + 1} rendering failed`, error);
        }
      }
    });
    queue = task.catch(() => {});
    preparations.set(key, task);
    void task.then(
      () => preparations.delete(key),
      () => preparations.delete(key),
    );
    return task;
  }

  async function requestTheme(themeID: string, show = true): Promise<void> {
    const request = show ? requestGuard.next() : 0;
    await renderTheme(themeID);
    if (show && requestGuard.isCurrent(request) && visibleTheme === themeID && !printActive) {
      showTheme(themeID);
    }
  }
  prepareTheme = requestTheme;

  const state = window.__airplanThemeState;
  const startup = new Set([
    state?.lightTheme ?? catalog.defaultLight,
    state?.darkTheme ?? catalog.defaultDark,
  ]);
  for (const themeID of startup) await renderTheme(themeID);
  await preparations.get("github-light");
  if (palettes.themes["github-light"] && themePrepared("github-light")) {
    variants.forEach((cache, index) => {
      const rendered = cache.get("github-light");
      if (rendered) cache.set(printThemeKey, rendered);
      if (failedVariants[index].has("github-light")) failedVariants[index].add(printThemeKey);
    });
  } else {
    await renderTheme("github-light", printThemeKey);
  }
  for (const themeID of pendingThemes) await renderTheme(themeID);
  pendingThemes.clear();
  await renderTheme(visibleTheme);
  if (printActive) showTheme(printThemeKey);
  else restoreScreenTheme();
} catch (error) {
  diagrams.forEach((diagram, index) => {
    diagram.textContent = sources[index];
    diagram.classList.add("mermaid-failed");
  });
  console.warn("airplan: Mermaid rendering failed", error);
}
