+++
title = "Browser smoke plan"
purpose = "Print coverage"
+++

# Browser smoke plan

This fixture verifies airplan's rendered-page browser behavior.

<details id="print-disclosure">
<summary>Collapsed details</summary>
<p>Print must include this disclosure content.</p>
<div hidden data-print-hidden>Hidden disclosure content.</div>
<script type="application/json" data-print-script>{"hidden":true}</script>
<style data-print-style>.print-hidden-fixture { color: red; }</style>
</details>

<details id="print-open-disclosure" open>
<summary>Expanded details</summary>
<p>Print must preserve this disclosure's expanded state.</p>
</details>

## Overview

The generated page should work independently of developer config.

## Details

- Read and Source views remain accessible.
- Copy controls preserve exact source and code bytes.

## Code sample

Inline `code`, [theme contrast guidance](https://example.com), and
representative prose keep theme contrast visible across dense reading content.

| Surface  | Expected behavior          |
| -------- | -------------------------- |
| Controls | Remain legible             |
| Syntax   | Follows the selected theme |

```js
const answer = 42;
console.log(answer);
```

<!-- markdownlint-disable MD028 -->

> [!NOTE]
> Informational context remains distinct.

> [!TIP]
> Successful guidance remains distinct.

> [!IMPORTANT]
> Important guidance remains distinct.

> [!WARNING]
> Caution remains distinct.

> [!CAUTION]
> Dangerous actions remain distinct.

<!-- markdownlint-enable MD028 -->

## Diagram

```mermaid
flowchart LR
  Plan --> Review --> Print
```

```mermaid
flowchart TD
  Source --> Render --> Share
```

```mermaid
sequenceDiagram
  Author->>Airplan: Upload themed document
  Airplan-->>Reader: Standalone page
```

```mermaid
stateDiagram-v2
  [*] --> System
  System --> Light
  System --> Dark
```

## Final checks

The compact table of contents remains available after the inline navigation
scrolls out of view on a narrow screen.
