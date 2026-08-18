import { describe, expect, test } from "bun:test";

import {
  assertTemplateSafe,
  prepareGeneratedSource,
  separateCSSStructuralDelimiters,
} from "./build.ts";

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
