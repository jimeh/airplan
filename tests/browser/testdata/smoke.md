+++
title = "Browser smoke plan"
purpose = "Print coverage"
+++

# Browser smoke plan

This fixture verifies airplan's rendered-page browser behavior.

<details id="print-disclosure">
<summary>Collapsed details</summary>
<p>Print must include this disclosure content.</p>
</details>

## Overview

The generated page should work without external assets or developer config.

## Details

- Rendered and source views remain accessible.
- Copy controls preserve exact source and code bytes.

## Code sample

```js
const answer = 42;
console.log(answer);
```

## Final checks

The compact table of contents remains available after the inline navigation
scrolls out of view on a narrow screen.
