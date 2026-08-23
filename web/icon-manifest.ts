export interface IconDefinition {
  octicon: string;
  classes?: readonly string[];
  attributes?: Readonly<Record<string, string>>;
}

// Semantic names keep template and browser code independent of upstream names.
// Updating an Octicon mapping remains an explicit, reviewable source change.
export const iconManifest = {
  back: { octicon: "arrow-left" },
  pages: { octicon: "list-unordered" },
  directory: { octicon: "file-directory" },
  copy: { octicon: "copy", classes: ["icon-copy"] },
  copied: { octicon: "check", classes: ["icon-check"] },
  failed: { octicon: "x", classes: ["icon-x"] },
  download: { octicon: "download" },
  source: { octicon: "code" },
  rendered: { octicon: "eye" },
  changes: { octicon: "diff" },
  toc: { octicon: "list-unordered" },
  close: { octicon: "x" },
  themeResolvedLight: {
    octicon: "sun",
    classes: ["appearance-resolved-icon"],
    attributes: { "data-airplan-resolved-icon": "light" },
  },
  themeResolvedDark: {
    octicon: "moon",
    classes: ["appearance-resolved-icon"],
    attributes: { "data-airplan-resolved-icon": "dark" },
  },
  themeSystem: { octicon: "device-desktop" },
  themeLight: { octicon: "sun" },
  themeDark: { octicon: "moon" },
  select: {
    octicon: "chevron-down",
    classes: ["appearance-select-icon"],
    attributes: { "data-airplan-select-icon": "" },
  },
  diagramOpen: { octicon: "screen-full" },
  diagramClose: { octicon: "x" },
} as const satisfies Record<string, IconDefinition>;

export type IconName = keyof typeof iconManifest;
