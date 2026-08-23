import { describe, expect, test } from "bun:test";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

import {
  assertTemplateSafe,
  generateIconSources,
  prepareGeneratedSource,
  separateCSSStructuralDelimiters,
} from "./build.ts";
import { iconManifest } from "./icon-manifest.ts";

describe("generated icons", () => {
  test("renders every semantic icon for templates and browser code", () => {
    const generated = generateIconSources();

    for (const name of Object.keys(iconManifest)) {
      const templateName = name.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
      const typescriptName = `icon${name[0].toUpperCase()}${name.slice(1)}`;
      expect(generated.template).toContain(`define "airplan-icon-${templateName}"`);
      expect(generated.typescript).toContain(`export const ${typescriptName} = "<svg `);
    }
    expect(generated.template).toContain('class="icon icon-copy"');
    expect(generated.template).toContain('data-airplan-resolved-icon="light"');
    expect(generated.typescript).not.toContain("data-component");
    expect(generated.typescript).not.toContain("octicon-");
  });

  test("rejects a missing upstream icon", () => {
    expect(() => generateIconSources({ missing: { octicon: "not-an-octicon" } })).toThrow(
      "missing: Octicon not-an-octicon has no 16px source",
    );
  });

  test("keeps hand-authored browser and template sources free of SVG markup", async () => {
    const root = resolve(import.meta.dir, "..");
    const paths = [
      "airplan/assets/collection.html.tmpl",
      "airplan/assets/page.html.tmpl",
      "airplan/assets/theme-toggle.html.tmpl",
      "web/src/page.ts",
      "web/src/mermaid.ts",
    ];

    for (const path of paths) {
      expect(await readFile(resolve(root, path), "utf8")).not.toContain("<svg");
    }
  });

  test("bundles only the icons each browser entry imports", async () => {
    const root = resolve(import.meta.dir, "..");
    const [page, mermaid] = await Promise.all([
      readFile(resolve(root, "airplan/assets/generated/readable/page.js"), "utf8"),
      readFile(resolve(root, "airplan/assets/generated/readable/mermaid.js.tmpl"), "utf8"),
    ]);

    expect(page).toContain("iconCopy");
    expect(page).not.toContain("iconDiagramOpen");
    expect(mermaid).toContain("iconDiagramOpen");
    expect(mermaid).not.toContain("iconCopy");
  });
});

describe("Go template delimiter safety", () => {
  test("separates only adjacent structural CSS braces", () => {
    const source = ".outer{{color:red}}}";
    const separated = prepareGeneratedSource(source, "page.css", "minified");

    expect(separated).toBe(".outer{ {color:red} } }");
    expect(() => assertTemplateSafe(separated, "page.css")).not.toThrow();
  });

  test("preserves and rejects quoted CSS delimiters", () => {
    const source = String.raw`.label{content:"{{quoted}} \\{{escaped}}"}`;

    expect(separateCSSStructuralDelimiters(source)).toBe(source);
    expect(() => prepareGeneratedSource(source, "page.css", "minified")).toThrow(
      "page.css: generated asset contains template delimiter {{",
    );
  });

  test("preserves and rejects CSS comment delimiters", () => {
    const source = ".label{/* {{comment}} */color:red}";

    expect(separateCSSStructuralDelimiters(source)).toBe(source);
    expect(() => prepareGeneratedSource(source, "page.css", "minified")).toThrow(
      "page.css: generated asset contains template delimiter {{",
    );
  });

  test("rejects JavaScript delimiters without rewriting source", () => {
    const source = 'const template = "{{value}}";';

    expect(() => prepareGeneratedSource(source, "page.js", "minified")).toThrow(
      "page.js: generated asset contains template delimiter {{",
    );
  });

  test("strips Bun source labels with either path separator", () => {
    const bundledSource = 'const value = "preserved";\n';
    const forwardSlash = `// web/src/page.ts\n${bundledSource}`;
    const backslash = `// web\\src\\page.ts\r\n${bundledSource}`;

    expect(prepareGeneratedSource(forwardSlash, "page.js", "minified")).toBe(bundledSource);
    expect(prepareGeneratedSource(backslash, "page.js", "minified")).toBe(bundledSource);
    expect(
      prepareGeneratedSource(
        `// airplan-generated-icons:icons.ts\n${bundledSource}`,
        "page.js",
        "minified",
      ),
    ).toBe(bundledSource);
  });

  test("rewrites exactly one quoted Mermaid URL sentinel", () => {
    const source = 'const mermaidURL = "__AIRPLAN_MERMAID_MODULE_URL__";';

    expect(prepareGeneratedSource(source, "mermaid.js.tmpl", "minified")).toBe(
      "const mermaidURL = {{.MermaidURL}};",
    );
  });

  test("rejects missing and duplicate Mermaid URL sentinels", () => {
    expect(() =>
      prepareGeneratedSource(
        'const mermaidURL = "https://example.test/mermaid.js";',
        "mermaid.js.tmpl",
        "minified",
      ),
    ).toThrow("mermaid.js.tmpl: expected exactly one quoted Mermaid URL sentinel");
    expect(() =>
      prepareGeneratedSource(
        'const first = "__AIRPLAN_MERMAID_MODULE_URL__"; const second = "__AIRPLAN_MERMAID_MODULE_URL__";',
        "mermaid.js.tmpl",
        "minified",
      ),
    ).toThrow("mermaid.js.tmpl: expected exactly one Mermaid URL sentinel");
  });
});
