# Built-in theme sources

Airplan's built-in themes map each upstream palette into the semantic document
tokens defined in `SPEC.md` §7. Palette values are pinned here so updates stay
explicit and reviewable. The rendered page does not copy an editor theme's UI;
it assigns official palette colors to background, foreground, muted, accent,
surface, border, and status roles, then derives quieter surfaces by deterministic
mixing. Syntax highlighting uses the corresponding Chroma 2.27.0 style.

| Airplan themes                     | Official source                                                                     | Pinned revision                                                                                                                               | License                         |
| ---------------------------------- | ----------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------- |
| GitHub Light, GitHub Dark          | [github/primer-primitives](https://github.com/primer/primitives)                    | [`30cb00c65d789d6ad4850f8a4fd172276e143226`](https://github.com/primer/primitives/tree/30cb00c65d789d6ad4850f8a4fd172276e143226)              | MIT, Copyright GitHub Inc.      |
| Catppuccin Latte, Catppuccin Mocha | [catppuccin/palette](https://github.com/catppuccin/palette)                         | [`07d02aa110ef9eb7e7427afca5c73ba9cf7f8ebd`](https://github.com/catppuccin/palette/tree/07d02aa110ef9eb7e7427afca5c73ba9cf7f8ebd)             | MIT, Copyright Catppuccin       |
| Rose Pine Dawn, Rose Pine          | [rose-pine/rose-pine-theme](https://github.com/rose-pine/rose-pine-theme)           | [`781bb844aae0bcec2763b23a4c7d3cc6aede780c`](https://github.com/rose-pine/rose-pine-theme/tree/781bb844aae0bcec2763b23a4c7d3cc6aede780c)      | MIT, Copyright Rose Pine        |
| Solarized Light, Solarized Dark    | [altercation/solarized](https://github.com/altercation/solarized)                   | [`62f656a02f93c5190a8753159e34b385588d5ff3`](https://github.com/altercation/solarized/tree/62f656a02f93c5190a8753159e34b385588d5ff3)          | MIT, Copyright Ethan Schoonover |
| Tokyo Night Day, Tokyo Night       | [enkia/tokyo-night-vscode-theme](https://github.com/enkia/tokyo-night-vscode-theme) | [`7c0f11eaef322f293621ca7befe462214b7ea468`](https://github.com/enkia/tokyo-night-vscode-theme/tree/7c0f11eaef322f293621ca7befe462214b7ea468) | MIT, Copyright Enkia            |
| One Dark                           | [Binaryify/OneDark-Pro](https://github.com/Binaryify/OneDark-Pro)                   | [`e6ccf638d5b69aa38cd1005edb0ee7ba7ef6fedc`](https://github.com/Binaryify/OneDark-Pro/tree/e6ccf638d5b69aa38cd1005edb0ee7ba7ef6fedc)          | MIT, Copyright Binaryify        |

The full license texts remain available at the pinned upstream revisions. The
palette mappings and deterministic derived values live in `airplan/theme.go`.
