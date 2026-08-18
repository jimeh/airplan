import {
  createLatestRequestGuard,
  type AirplanThemeChangeDetail,
  type ThemeCatalog,
} from "./theme-state";

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

const diagrams = Array.from(document.querySelectorAll<HTMLPreElement>("pre.mermaid"));
const mermaidModuleURL = "__AIRPLAN_MERMAID_MODULE_URL__";
const sources = diagrams.map((diagram) => diagram.textContent);
const variants = diagrams.map(() => new Map<string, MermaidRenderResult>());
const failedThemes = new Set<string>();
const catalog = window.__AIRPLAN_THEME_CATALOG__ as ThemeCatalog | undefined;
const printMedia = matchMedia("print");
const printThemeKey = "__airplan-print-github-light";
let mermaidCloneID = 0;
let viewer: MermaidViewer | null = null;
let visibleTheme = window.__airplanThemeState?.theme ?? catalog?.defaultLight ?? "github-light";
let lastScreenTheme = visibleTheme;
const requestGuard = createLatestRequestGuard();

function showTheme(theme: string): boolean {
  if (!variants.every((themes) => themes.has(theme))) return false;
  variants.forEach((themes, index) => {
    const rendered = themes.get(theme);
    if (!rendered) return;
    diagrams[index].innerHTML = rendered.svg;
    if (rendered.bindFunctions) rendered.bindFunctions(diagrams[index]);
    if (viewer) viewer.enhance(diagrams[index]);
  });
  if (theme !== printThemeKey) lastScreenTheme = theme;
  return true;
}

function restoreScreenTheme(): void {
  if (!showTheme(visibleTheme)) showTheme(lastScreenTheme);
}

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

function readSVGSize(svg: SVGSVGElement) {
  const values = svg
    .getAttribute("viewBox")
    ?.trim()
    .split(/[\s,]+/)
    .map(Number);
  if (
    values?.length === 4 &&
    Number.isFinite(values[2]) &&
    values[2] > 0 &&
    Number.isFinite(values[3]) &&
    values[3] > 0
  ) {
    return { width: values[2], height: values[3] };
  }
  const width = Number.parseFloat(svg.getAttribute("width") || "");
  const height = Number.parseFloat(svg.getAttribute("height") || "");
  return {
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
  const closeButton = createButton(
    "Close diagram viewer",
    "close",
    '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16"' +
      ' fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8' +
      " 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215" +
      ".734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0" +
      " 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018" +
      ".751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1" +
      ' 0-1.06Z"/></svg>',
  );
  toolbar.append(zoomOut, zoomValue, zoomIn, fit, closeButton);
  header.appendChild(toolbar);

  const canvas = document.createElement("div");
  canvas.className = "mermaid-canvas";
  canvas.setAttribute("data-airplan-mermaid-canvas", "");
  canvas.tabIndex = 0;
  canvas.setAttribute("role", "group");
  canvas.setAttribute(
    "aria-label",
    "Zoomable Mermaid diagram. Scroll or use plus and minus to zoom. " +
      "Drag or use arrow keys to pan.",
  );
  const surface = document.createElement("div");
  surface.className = "mermaid-surface";
  surface.setAttribute("data-airplan-mermaid-surface", "");
  canvas.appendChild(surface);

  const help = document.createElement("p");
  help.className = "mermaid-help";
  help.textContent = "Scroll to zoom · Drag to pan · 0 to fit";
  shell.append(header, canvas, help);
  dialog.appendChild(shell);
  document.body.appendChild(dialog);

  let sourceBlock: HTMLPreElement | null = null;
  let returnFocus: HTMLButtonElement | null = null;
  let viewerSVG: SVGSVGElement | null = null;
  let naturalWidth = 1;
  let naturalHeight = 1;
  let scale = 1;
  let panX = 0;
  let panY = 0;
  let activePointer: number | null = null;
  let pointerStartX = 0;
  let pointerStartY = 0;
  let pointerPanX = 0;
  let pointerPanY = 0;

  function applyTransform() {
    surface.style.transform = `translate(${panX}px, ${panY}px)`;
    if (viewerSVG) {
      viewerSVG.style.transform = `translate(-50%, -50%) scale(${scale})`;
    }
    zoomValue.textContent = `${Math.round(scale * 100)}%`;
  }

  function fitDiagram() {
    const bounds = canvas.getBoundingClientRect();
    const availableWidth = Math.max(bounds.width - 48, 1);
    const availableHeight = Math.max(bounds.height - 48, 1);
    scale = clamp(
      Math.min(availableWidth / naturalWidth, availableHeight / naturalHeight, 1),
      0.05,
      8,
    );
    panX = 0;
    panY = 0;
    applyTransform();
  }

  function setZoom(nextScale: number, clientX?: number, clientY?: number) {
    const clamped = clamp(nextScale, 0.05, 8);
    if (clamped === scale) return;
    if (clientX !== undefined && clientY !== undefined) {
      const bounds = canvas.getBoundingClientRect();
      const offsetX = clientX - (bounds.left + bounds.width / 2) - panX;
      const offsetY = clientY - (bounds.top + bounds.height / 2) - panY;
      const ratio = clamped / scale;
      panX += offsetX * (1 - ratio);
      panY += offsetY * (1 - ratio);
    }
    scale = clamped;
    applyTransform();
  }

  function showSVG(svg: SVGSVGElement, reset: boolean) {
    const clone = cloneMermaidSVG(svg);
    viewerSVG = clone;
    const size = readSVGSize(svg);
    naturalWidth = size.width;
    naturalHeight = size.height;
    clone.style.width = `${naturalWidth}px`;
    clone.style.height = `${naturalHeight}px`;
    clone.style.maxWidth = "none";
    clone.style.maxHeight = "none";
    surface.replaceChildren(clone);
    if (reset) fitDiagram();
    else applyTransform();
  }

  function closeViewer() {
    if (dialog.open) dialog.close();
  }

  function openViewer(block: HTMLPreElement, trigger: HTMLButtonElement) {
    const svg = block.querySelector<SVGSVGElement>("svg");
    if (!svg) return;
    sourceBlock = block;
    returnFocus = trigger;
    showSVG(svg, false);
    dialog.showModal();
    document.body.classList.add("mermaid-dialog-open");
    fitDiagram();
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
    activePointer = null;
    canvas.classList.remove("mermaid-canvas-panning");
    const target = returnFocus;
    sourceBlock = null;
    returnFocus = null;
    if (target?.isConnected) {
      setTimeout(() => target.focus(), 0);
    }
  });
  dialog.addEventListener("keydown", (event) => {
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
      ArrowLeft: [panStep, 0],
      ArrowRight: [-panStep, 0],
      ArrowUp: [0, panStep],
      ArrowDown: [0, -panStep],
    }[event.key];
    if (!direction) return;
    event.preventDefault();
    panX += direction[0];
    panY += direction[1];
    applyTransform();
  });
  canvas.addEventListener(
    "wheel",
    (event) => {
      event.preventDefault();
      setZoom(scale * (event.deltaY < 0 ? 1.2 : 1 / 1.2), event.clientX, event.clientY);
    },
    { passive: false },
  );
  canvas.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    activePointer = event.pointerId;
    pointerStartX = event.clientX;
    pointerStartY = event.clientY;
    pointerPanX = panX;
    pointerPanY = panY;
    canvas.classList.add("mermaid-canvas-panning");
    try {
      canvas.setPointerCapture(event.pointerId);
    } catch {}
  });
  canvas.addEventListener("pointermove", (event) => {
    if (activePointer !== event.pointerId) return;
    panX = pointerPanX + event.clientX - pointerStartX;
    panY = pointerPanY + event.clientY - pointerStartY;
    applyTransform();
  });
  function stopPanning(event: PointerEvent) {
    if (activePointer !== event.pointerId) return;
    try {
      canvas.releasePointerCapture(event.pointerId);
    } catch {}
    activePointer = null;
    canvas.classList.remove("mermaid-canvas-panning");
  }
  canvas.addEventListener("pointerup", stopPanning);
  canvas.addEventListener("pointercancel", stopPanning);

  return {
    enhance(block: HTMLPreElement) {
      const svg = block.querySelector<SVGSVGElement>("svg");
      if (!svg) return;
      let trigger = block.querySelector<HTMLButtonElement>(":scope > [data-airplan-mermaid-open]");
      if (!trigger) {
        trigger = createButton(
          "Open diagram viewer",
          "open",
          '<svg class="icon" aria-hidden="true" viewBox="0 0 16 16"' +
            ' fill="currentColor"><path d="M3.75 2A1.75 1.75 0 0 0 2 3.75v2.5' +
            "a.75.75 0 0 0 1.5 0v-2.5a.25.25 0 0 1 .25-.25h2.5a.75.75 0" +
            " 0 0 0-1.5Zm6 0a.75.75 0 0 0 0 1.5h2.5a.25.25 0 0 1 .25.25" +
            "v2.5a.75.75 0 0 0 1.5 0v-2.5A1.75 1.75 0 0 0 12.25 2ZM2.75" +
            " 9a.75.75 0 0 1 .75.75v2.5c0 .138.112.25.25.25h2.5a.75.75 0 0" +
            " 1 0 1.5h-2.5A1.75 1.75 0 0 1 2 12.25v-2.5A.75.75 0 0 1 2.75" +
            " 9m10.5 0a.75.75 0 0 1 .75.75v2.5A1.75 1.75 0 0 1 12.25 14h" +
            "-2.5a.75.75 0 0 1 0-1.5h2.5a.25.25 0 0 0 .25-.25v-2.5a.75.75" +
            ' 0 0 1 .75-.75"/></svg>',
        );
        trigger.classList.add("mermaid-open");
        trigger.setAttribute("aria-haspopup", "dialog");
        trigger.setAttribute("aria-controls", dialog.id);
        const activeTrigger = trigger;
        trigger.addEventListener("click", () => openViewer(block, activeTrigger));
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
  if (!catalog) throw new Error("theme catalog is unavailable");
  const { default: mermaid } = (await import(mermaidModuleURL)) as {
    default: MermaidAPI;
  };
  const themeByID = new Map(catalog.themes.map((theme) => [theme.id, theme]));
  let queue = Promise.resolve();

  function renderTheme(themeID: string, key = themeID): Promise<boolean> {
    if (variants.every((cache) => cache.has(key))) return Promise.resolve(true);
    if (failedThemes.has(key)) return Promise.resolve(false);
    const theme = themeByID.get(themeID);
    if (!theme) return Promise.resolve(false);
    let result = false;
    queue = queue.then(async () => {
      if (variants.every((cache) => cache.has(key))) {
        result = true;
        return;
      }
      try {
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: "strict",
          secure: ["theme", "themeVariables", "themeCSS", "darkMode"],
          theme: "base",
          themeVariables: theme.mermaid,
        });
        const rendered: MermaidRenderResult[] = [];
        for (const [index, source] of sources.entries()) {
          rendered.push(
            await mermaid.render(
              `airplan-mermaid-${key.replace(/[^a-z0-9-]/g, "print")}-${index}`,
              source,
            ),
          );
        }
        rendered.forEach((value, index) => variants[index].set(key, value));
        diagrams.forEach((diagram) => {
          diagram.classList.add("mermaid-rendered");
          diagram.classList.remove("mermaid-failed");
        });
        if (!viewer) viewer = createMermaidViewer();
        result = true;
      } catch (error) {
        failedThemes.add(key);
        if (!variants.some((cache) => cache.size > 0)) {
          diagrams.forEach((diagram, index) => {
            diagram.textContent = sources[index];
            diagram.classList.add("mermaid-failed");
          });
        }
        console.warn(`airplan: Mermaid ${themeID} diagram rendering failed`, error);
      }
    });
    return queue.then(() => result);
  }

  async function requestTheme(themeID: string, show = true): Promise<void> {
    const request = requestGuard.next();
    if (await renderTheme(themeID)) {
      if (
        show &&
        requestGuard.isCurrent(request) &&
        visibleTheme === themeID &&
        !printMedia.matches
      ) {
        showTheme(themeID);
      }
    }
  }

  const state = window.__airplanThemeState;
  const startup = new Set([
    state?.lightTheme ?? catalog.defaultLight,
    state?.darkTheme ?? catalog.defaultDark,
  ]);
  for (const themeID of startup) await renderTheme(themeID);
  await renderTheme("github-light", printThemeKey);
  showTheme(visibleTheme);

  window.addEventListener("airplan:themechange", (event) => {
    const detail = (event as CustomEvent<AirplanThemeChangeDetail>).detail;
    visibleTheme = detail.theme;
    void requestTheme(detail.theme);
  });
  window.addEventListener("airplan:themeprepare", (event) => {
    const detail = (event as CustomEvent<{ theme: string }>).detail;
    void requestTheme(detail.theme, false);
  });
  printMedia.addEventListener("change", (event) => {
    if (event.matches) showTheme(printThemeKey);
    else restoreScreenTheme();
  });
  // The print variant was prepared during startup; this path is synchronous so
  // the browser cannot overtake an asynchronous Mermaid render.
  window.addEventListener("beforeprint", () => showTheme(printThemeKey));
  window.addEventListener("afterprint", restoreScreenTheme);
} catch (error) {
  diagrams.forEach((diagram, index) => {
    diagram.textContent = sources[index];
    diagram.classList.add("mermaid-failed");
  });
  console.warn("airplan: Mermaid rendering failed", error);
}
