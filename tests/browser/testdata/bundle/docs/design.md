---
title: Design notes
---

# Design notes

This page loads as its own document.

<script>
window.__airplanBundleLifetime = crypto.randomUUID();
sessionStorage.setItem(
  "airplan-bundle-authored-runs",
  String(Number(sessionStorage.getItem("airplan-bundle-authored-runs") || "0") + 1),
);
</script>

## Deep dive

```mermaid
flowchart LR
  Entry --> Design
```

<details id="bundle-print-detail">
<summary>Print detail</summary>

Printed bundle content

</details>

## Decisions

Use complete documents and ordinary links.
