# Revision-aware page navigation and diff UX plan

Status: proposed

Scope: built-in document-bundle pages, adjacent revision diffs, revision
selection, mobile navigation, upgrades, custom-template data, and browser
verification

Baseline: the document-bundle implementation on PR #97, marker v6, renderer
generation 4, versions metadata v1, and spec 0.41.0

Related plan: [Document bundles, assets, and multi-page navigation](document-bundles-and-assets.md)

## 1. Outcome

Make revision history behave like page-aware documentation rather than a set of
unrelated entry URLs:

- A revision selection keeps the reader on the same logical page when that page
  exists in the target revision.
- If the page does not exist in the target revision, selection opens that
  revision's entry page.
- `Changes` always describes the page currently being viewed.
- Unchanged pages do not show a `Changes` mode.
- Markdown pages use `Rendered`, `Source`, and, when applicable, `Changes`
  modes.
- Source-code and other managed text pages use `Code` and, when applicable,
  `Changes` modes.
- Every revision after revision 1 offers `All changes`, which opens the complete
  adjacent bundle diff on that revision's entry page.
- Narrow layouts keep Contents behind a permanent bottom-right button rather
  than placing the table of contents above the document.
- Mobile layouts use a compact sticky top bar for document navigation and file
  actions, while content modes remain with the document.

All destinations remain real standalone HTML documents. Revision selection and
page links perform normal browser navigations, so browser-native cross-document
View Transitions remain the only transition mechanism.

## 2. Current behavior and gaps

The current bundle implementation has the right storage building blocks, but
the rendered revision UI treats the bundle-wide diff as if it belonged to every
page:

- A v6 marker already records every managed page's logical `path`, rendered
  `page`, source object, format, title, and language.
- `.airplan-versions.json` stores one entry URL and one adjacent diff URL per
  live revision. It does not store page mappings.
- Revision creation writes one deterministic `.airplan-changes.diff` containing
  logical-path sections for the whole bundle.
- The renderer currently passes that same combined diff to every rendered page.
- The built-in template exposes the `Changes` control only inside the Markdown
  `Rendered` and `Source` toggle.
- Managed text pages render highlighted code as their primary content, but do
  not expose a `Code` and `Changes` toggle.
- The browser runtime requires the current browser URL to equal the revision's
  entry URL. That disables the revision picker on child pages.
- Selecting a revision always opens its entry URL, even when the current
  logical page also exists there.

The follow-up should fix those behaviors without expanding the replicated
versions index or adding a client-side router.

## 3. Settled decisions

1. Logical source path is page identity across revisions. Matching is exact and
   case-sensitive after the existing portable-path validation.
2. A rename remains a removal plus an addition. Airplan does not guess page
   identity from content, title, or digest.
3. `.airplan-versions.json` remains schema version 1 and continues to store only
   entry URLs. It does not gain a page list.
4. The selected revision's v6 `.airplan.json` marker supplies its page map. The
   browser fetches that marker only after the reader chooses a revision.
5. If marker retrieval or validation fails, revision selection still succeeds
   by opening the selected revision's validated entry URL.
6. `Changes` means the current page's adjacent change only. This rule also
   applies to the entry page.
7. `All changes` means the complete adjacent revision report. It includes page
   additions, removals, source and descriptor changes, ordering changes, and
   asset summaries.
8. `All changes` is a revision-level action next to the revision selector, not
   another item in the page's content-mode toggle.
9. The complete report appears only on the revision entry page. Child pages link
   to the entry page's reserved all-changes fragment.
10. Revision 1 has no adjacent diff and therefore no `Changes` or `All changes`
    UI.
11. The existing `.airplan-changes.diff` remains the only stored diff object.
    Airplan does not upload one diff per page.
12. The existing 32 MiB canonical diff limit remains. The 512 KiB inline limit
    applies independently to the complete report and each page-local section.
13. Revision and page navigation remain ordinary links or
    `window.location.assign` navigations. Airplan does not fetch HTML, replace
    DOM content, or call `history.pushState`.
14. Custom templates receive additive data but no injected controls or scripts.
15. When the right ToC rail collapses, the inline ToC leaves the normal page
    flow and its floating bottom-right trigger stays visible for the whole
    rendered view.
16. On mobile, the top document bar is sticky and contains Pages, applicable
    file actions, and Appearance. The revision selector, `All changes`, and
    content-mode toggle remain in the document.
17. The collapsed Pages control uses a top-anchored document menu. It does not
    reuse the Contents bottom-sheet dialog.

## 4. Reader experience

### 4.1 Page modes

The built-in UI follows this matrix for revisions greater than 1:

| Current page           | Adjacent page change | Content modes             | Revision action |
| ---------------------- | -------------------- | ------------------------- | --------------- |
| Entry Markdown         | yes                  | Rendered, Source, Changes | All changes     |
| Entry Markdown         | no                   | Rendered, Source          | All changes     |
| Child Markdown         | yes                  | Rendered, Source, Changes | All changes     |
| Child Markdown         | no                   | Rendered, Source          | All changes     |
| Managed text or source | yes                  | Source, Changes           | All changes     |
| Managed text or source | no                   | Source                    | All changes     |

`Changes` appears when the current page was added or its source, format, title,
or language changed relative to the adjacent predecessor. Reordering a page
does not add `Changes` to that page because ordering belongs to the bundle
navigation, not the page content. It appears in `All changes` instead.

A removed page has no page-local view in the new revision. Its removal appears
in `All changes`. When the reader views the older revision where the page still
exists, that revision continues to show only its own adjacent diff.

An unchanged page does not get a disabled or empty `Changes` button. This keeps
the mode control honest and avoids presenting the same bundle-wide report on
every page.

### 4.2 Page-local changes

The page-local view uses only the canonical diff section for the current
logical path. Its heading names both revisions and the page:

```text
Changes to server.go from revision 2
```

For a source change, the view contains the server-highlighted unified diff. For
a descriptor-only change, it contains a compact textual summary. For an added
page, it contains the page's addition diff and descriptor summary when needed.

If a page section exceeds 512 KiB, the mode still exists but shows the current
large-diff message and links to the canonical raw bundle diff. Airplan does not
create a second raw URL for that page.

Printing continues to print the primary rendered or code view, not a hidden
page diff or the complete report.

### 4.3 All changes

Every revision after revision 1 displays an `All changes` link beside its
revision selector. This includes revisions containing only asset, ordering, or
metadata changes, and revisions whose current page did not change.

The link targets the revision entry page with the reserved fragment:

```text
#airplan-all-changes
```

On the entry page, that fragment activates a separate full-width complete-diff
view. The view contains the combined highlighted report when it is no larger
than 512 KiB and always links to `.airplan-changes.diff`. Larger reports show
the existing inline-size message and raw link.

While active, the complete view replaces the page content modes and hides the
Pages and On this page rails. The toolbar and revision controls remain visible,
but the collapsed Pages trigger is hidden because the complete report is not a
page-navigation context. The view places `Back to document` and `Open raw diff`
before its heading and report. `Back to document` is an ordinary link to the
entry URL without the reserved fragment.

The complete report is not part of the entry page's `Rendered`, `Source`, and
`Changes` toggle. It is revision-level state with its own URL. On the entry
page, following the fragment link creates an ordinary same-document history
entry. On a child page, it performs a full navigation to the entry. Browser
Back reverses either action without Airplan-managed history state.

Child pages do not embed the complete highlighted report. Their `All changes`
link performs a normal navigation to the entry page. This avoids duplicating a
large report into every HTML object in the revision.

The entry template owns the reserved fragment. Authored content remains
trusted, but the Airplan runtime activates the all-changes panel through a
dedicated data attribute rather than selecting an arbitrary authored element
with the same ID.

### 4.4 Narrow and mobile navigation

Use two breakpoints for two different layout changes:

- At the existing rail-collapse breakpoint, currently `78rem`, remove the
  inline table of contents from the enhanced page flow. Show its floating
  bottom-right trigger whenever the rendered Markdown view has headings. Make
  the top document bar sticky here as well, and pin Pages to its left edge.
- At the existing mobile action breakpoint, currently `48rem`, compact the
  applicable file actions and Appearance to icon-only 44px targets.

The mobile page should read like this:

```text
┌─────────────────────────────────────────┐
│ Pages       Copy  Download  Raw  Theme │  sticky document bar
└─────────────────────────────────────────┘

┌──────────┬────────┬─────────┐
│ Rendered │ Source │ Changes │            content-local modes
└──────────┴────────┴─────────┘

Revision 3 (Latest)             All changes

Document content
                                      ┌─────┐
                                      │ ToC │  fixed floating button
                                      └─────┘
```

The labels in this wireframe describe actions, not their literal compact
rendering. Keep `Pages` as icon plus text because it establishes document
context. Render Copy, Download, Raw, and Appearance as icon-only buttons with
accessible names and tooltips. Omit actions that do not apply to the current
page. File actions keep their current behavior and feedback.

The sticky bar uses the existing document colors and border tokens. Give it an
opaque background, a quiet bottom border, safe-area top padding, and enough
height for 44px touch targets. Avoid a translucent floating-card treatment.
This is document chrome, not an overlay. Set document `scroll-padding-top` and
heading `scroll-margin-top` from the measured bar height so fragment navigation
does not hide headings behind it.

On wide layouts, keep the Pages and On this page headings outside their rail
scroll containers. Only the item lists scroll, so readers retain the rail's
identity while moving through a long page or bundle.

Keep one semantic content-mode control. On wide layouts, align it at the left
of the top control row opposite the file actions and Appearance. When the page
rails collapse, move it below the fixed-height sticky toolbar and keep it above
the revision controls. Do not duplicate the toggle for different breakpoints.
Retain the established eye and code icons beside their visible labels. Reset
those buttons independently of toolbar styles: inactive modes are transparent,
while the active mode alone uses the theme's control background.

The revision selector and `All changes` remain non-sticky. They describe the
revision being read, so they belong with the page content rather than the
always-available document actions.

### 4.5 Distinct Pages and Contents menus

Pages and Contents answer different questions and should not look or move the
same way:

- Pages answers "Which document am I reading?" It opens downward from the
  Pages button in the top bar.
- Contents answers "Where am I in this document?" It opens upward from the
  floating bottom-right button as the existing bottom sheet.

Replace the collapsed Pages bottom-sheet dialog with a top-anchored document
menu. Prefer the native Popover API after capability detection. Position the
panel immediately below the top bar, align its leading edge with the Pages
button, and cap its width at about `30rem` and its height to the available
viewport. On a phone it may use the viewport width minus the page gutter.

The panel has a flat top edge attached visually to the bar, rounded lower
corners, a downward shadow, and a short top-origin reveal when reduced motion is
not requested. Reuse the technical-notebook page rows with title, logical path,
and the existing strong current-page indicator. Do not give it the Contents
dialog's bottom-sheet header, floating circular trigger, or upward motion.

The Pages button toggles the menu and reflects `aria-expanded`. Light dismiss,
Escape, choosing a page, widening beyond the rail-collapse breakpoint, or
opening Contents closes it. Opening Pages likewise closes Contents. Restore
focus to the Pages trigger when dismissal does not navigate.

Hide the inline Pages and ToC lists only after their enhanced controllers are
available. Without JavaScript or the required native menu/dialog support, leave
the corresponding inline list in the document as the functional fallback.

The floating ToC button no longer waits for the inline ToC to scroll above the
viewport. Keep it visible from the top of the page to the bottom whenever all
of these are true:

- the rail-collapse media query matches;
- the current page has a table of contents;
- the rendered Markdown view is active; and
- neither Pages nor Contents is open.

Hide it in Source, Changes, Code, and All changes views because those views do
not use the rendered document headings. Keep the existing safe-area-aware
bottom and right offsets.

## 5. Revision selection behavior

### 5.1 Metadata embedded in each managed page

Every built-in managed page in a linked document includes enough immutable
identity to make a selection:

```html
<meta name="airplan-revision" content="3" />
<meta name="airplan-revision-chain" content="abcdefghijklmnopqrstuvwxyz" />
<meta name="airplan-versions" content="../.airplan-versions.json" />
<meta name="airplan-page-path" content="docs/guide.md" />
<meta name="airplan-entrypoint" content="../index.html" />
```

The renderer calculates control and entry paths relative to each rendered page.
It emits revision discovery metadata for managed text/source pages as well as
Markdown pages. Ordinary standalone text and HTML documents remain unchanged.

The logical page path is immutable page identity. The entrypoint is the
relative rendered entry URL for the containing revision. The chain ID lets the
browser bind a fetched marker to the already validated versions index.

### 5.2 Selection algorithm

After validating `.airplan-versions.json`, the runtime builds the native
revision selector as it does today. Selecting from an all-changes view opens
the selected entry directly, preserving the reserved fragment for revisions
greater than 1. Selecting from the entry page also opens the selected entry
directly because every live revision has one.

Selecting from a child page follows these steps:

1. Validate the selected entry URL using the existing same-service capability
   URL rules.
2. Resolve the selected upload directory from that entry URL.
3. Fetch `<selected-directory>/.airplan.json` with `cache: "no-store"`,
   same-origin credentials, and a fresh nonce.
4. Strictly validate that the response is a v6 document marker whose revision
   number and chain ID match the selected versions entry and current chain.
5. Validate its entrypoint and ordered pages as same-directory, portable,
   unique mappings. Confirm that the marker entrypoint resolves to the selected
   entry URL.
6. Look for an exact `pages[].path` match to the current page's embedded logical
   path.
7. If found, navigate to that page's validated rendered object URL.
8. If absent, or if any fetch or marker validation step fails, navigate to the
   selected entry URL.

The fallback is deliberate. Versions metadata already proves the entry URL is
a valid chain destination, so an unavailable page map must not make revision
navigation unusable.

The selector shows a temporary busy state while it reads a child-page target
marker and ignores additional changes until the decision completes. A failed
marker read does not show an error toast because navigation still completes
through the entry fallback. Network and validation failures retain the existing
console warning for diagnosis.

### 5.3 Fragment rules

- When the same logical page exists in the target revision, preserve an
  ordinary content fragment. If the target page no longer has that anchor, the
  browser simply loads the top of the page.
- When the current page is absent and selection falls back to the entry, drop
  its content fragment because it belonged to a different page.
- Preserve `#airplan-all-changes` when selecting another revision greater than
  1. This always targets the selected revision's entry page.
- When selecting revision 1 from `#airplan-all-changes`, open the revision 1
  entry without the fragment because revision 1 has no adjacent report.
- Do not preserve transient `Rendered`, `Source`, `Code`, or `Changes` mode.
  Those modes remain page-local state and reset on the fresh document load.

### 5.4 URL and directory validation correction

The current runtime derives the service prefix by dropping two URL path
segments and requires the browser URL to equal the revision entry URL. Both
assumptions fail for nested child pages.

Instead, derive the current upload directory from the resolved
`airplan-versions` metadata URL. Validate that the current page and each fetched
control object remain beneath that exact directory. Accept the current revision
when its validated entry URL identifies the same upload directory, not only
when it equals `window.location`.

## 6. Diff representation and extraction

### 6.1 One canonical report, structured while rendering

Keep `.airplan-changes.diff` as the immutable adjacent report and storage
contract. Refactor generation to produce both:

- the exact canonical byte stream written to storage; and
- an ordered internal model containing the revision range, page sections,
  bundle-level sections, and asset sections.

Serializing that model produces the stored bytes. Rendering consumes the model
rather than handing one opaque combined string to every page.

Revision creation already has the prior marker, prior sources, new rendered
bundle, and prepared assets in one operation. It can classify each change once,
generate one canonical report, then pass only the matching page section to each
page renderer. The entry renderer also receives the complete report.

### 6.2 Deterministic sections

Retain the existing leading range header and follow it with an explicit report
format:

```text
# airplan revisions: 2 -> 3
# airplan diff format: 2
```

Each changed logical page gets one explicitly typed section whose exact path is
JSON encoded. Unified headers must carry the same logical path:

```text
# airplan page: "docs/guide.md"
--- revision-2/docs/guide.md
+++ revision-3/docs/guide.md
```

Page descriptor changes use one compact JSON object after a fixed line prefix
inside the same page section:

```text
page added: {"format":"md","title":"Guide","lang":"markdown"}
page removed: {"format":"txt","title":"Server","lang":"go"}
page metadata changed: {"title":{"before":"Guide","after":"Guide and API"}}
```

Emit only applicable fields, order them as `format`, `title`, then `lang`, and
use Go's JSON string encoding so values cannot imitate section boundaries.
Source changes follow the metadata line in the same section.

Page and asset order changes use reserved `# airplan page order` and
`# airplan asset order` sections. Each contains `before:` and `after:` JSON
arrays of logical paths. Asset changes carry their content-type, size, and
SHA-256 summary under an explicit
`# airplan asset: <json-string>` heading. The generator writes bundle-level
sections first, sorts path-keyed sections, and uses a fixed field order so
equivalent inputs produce identical bytes.

An adjacent revision that has no page-source changes can still produce a useful
complete report. `No textual changes.` remains only when the revision contains
no reportable page, asset, ordering, or metadata detail beyond the fact that a
revision was created.

### 6.3 Upgrade parsing

An upgrade does not have the original in-memory change model. It reads the
stored canonical diff, validates its adjacent revision range, and parses the
deterministic section envelope into page-local sections.

The parser is strict about Airplan-owned range and typed section headers. When
a page section contains unified headers, their logical paths and revision
numbers must match the section identity and envelope. Source lines beginning
with `#` remain safe because unified diff content prefixes them with a context
or change marker. Existing generation-4 untyped structured reports and
single-page revision diffs without the bundle range envelope remain supported;
the latter map wholly to the entry page.

If a revision marker declares a diff whose structure cannot be parsed safely,
upgrade fails closed before replacing any rendered page. Ordinary serving and
raw diff access remain unaffected.

## 7. Rendering and Go library changes

### 7.1 Render model

Replace the current single opaque bundle `DiffText` path with explicit revision
render data:

- canonical raw diff URL;
- current page changed state;
- current page inline diff text and highlighted HTML;
- complete inline diff text and highlighted HTML, entry page only;
- current logical page path and rendered entrypoint;
- all-changes target URL; and
- revision and chain identity.

Keep the internal model structured until the renderer selects the current page.
Do not split and re-highlight the full report once per page.

For Markdown, the primary content remains rendered HTML and the secondary
source view remains Markdown. For managed text/source, the existing highlighted
code becomes the primary `Code` view. Both formats use the same optional
page-local changes component.

### 7.2 Public render options and custom templates

Extend `RenderOptions`, `DocumentRenderOptions`, and `TemplateData` with
additive page-local and complete-diff fields. Name the fields by meaning rather
than overloading `DiffText` for both scopes. Retain `DiffText` for existing
single-page render callers. When no structured revision data is supplied, map
it to both the page-local and complete diff for that one entry page. Built-in
bundle rendering uses only the explicit structured fields.

Custom templates receive:

- current logical page identity;
- whether the current page changed;
- page-local diff text and highlighted HTML when within the inline limit;
- canonical combined diff path;
- complete diff text and highlighted HTML on the entry page when within the
  inline limit; and
- the all-changes URL.

Airplan does not inject the built-in mode controls, marker-fetching runtime, or
all-changes panel into custom output. Document the new fields in SPEC.md's
template contract and IMPLEMENTATION.md.

### 7.3 Renderer generation

Increment `RendererGeneration` because existing linked bundles need new meta
elements, source-page revision controls, page-local diff selection, and the
entry all-changes panel.

The upgrade planner should identify every source-backed page generated by an
older renderer. A successful upgrade reproduces the complete bundle with the
same stored sources, assets, marker identity, and canonical diff, then updates
page digests in the marker according to the existing resumable entry-last
process.

## 8. Storage and metadata contracts

No new stored object is required:

```text
<revision-directory>/
  .airplan.json
  .airplan-versions.json
  .airplan-changes.diff
  index.html
  docs/guide.html
  server.go.html
  ...sources and assets
```

Marker v6 already contains the selected revision's complete page map. The
256 KiB marker limit and 100-item bundle limit remain unchanged.

Versions metadata remains capped at 64 KiB and replicated exactly as it is
today. Avoiding a page map in every revision entry preserves chain capacity and
avoids multiplying page inventory across every replica on every append or
delete.

The browser's marker fetch is read-only and uses an existing public capability
URL. Anyone who can read a revision already has access to its marker, source,
asset, and page URLs. This does not widen the trust boundary.

## 9. Lifecycle behavior

### 9.1 New revision creation

During complete-bundle comparison, classify page, asset, ordering, and metadata
changes before rendering the candidate. Generate the canonical report and its
structured sections once. Render and upload:

1. every managed source and asset under the existing checksum rules;
2. non-entry pages with only their page-local inline section;
3. the canonical raw diff;
4. the entry page with its page-local section and complete inline report; and
5. revisions metadata under the existing append and reconciliation protocol.

Keep the existing marker-first ownership boundary and entry-last visibility
ordering. A page-local UI decision must not change revision atomicity,
reconciliation, no-op detection, or complete-replacement semantics.

### 9.2 First-link promotion

When a standalone v6 Markdown bundle becomes revision 1, re-render every
managed page with revision identity and selector metadata, including text/source
children. Revision 1 receives no page-local or complete diff UI.

The candidate revision 2 uses the ordinary page-aware diff rendering path. The
promotion must not stamp the entry's identity or controls onto every child.

### 9.3 Upgrade and retry

Upgrade reads and validates the existing canonical diff once, derives each
page's render data, and spools desired page outputs. It retains the current
checksum-based retry behavior:

- skip pages whose stored digest already matches;
- repair mismatched non-entry pages first;
- publish the entry page last; and
- replace the marker only after its desired object inventory and digests are
  complete.

A failed upgrade may still leave a mixture of renderer generations at stable
page URLs. The next attempt deterministically derives the same page-local and
complete views from the immutable diff and converges without per-page marker
writes.

### 9.4 Deletion and tombstones

Deleted revisions remain absent from the selector. A surviving revision's
marker page map is fetched only if the reader selects it. Tombstoning does not
rewrite canonical diffs or page HTML, so an already open older page may still
show its baked `All changes` link while live versions metadata removes deleted
destinations.

## 10. CLI, REST API, HTTP transport, and MCP

The upload and revision request models do not change. Callers already submit a
complete ordered page inventory, and results already return page URLs. No new
CLI flags, REST fields, OpenAPI schemas, capability names, MCP inputs, MCP
outputs, or versions-metadata fields are needed.

All entry points benefit from the core renderer change:

- `airplan new-revision` and its `update` alias publish page-aware revision UI;
- direct Go and HTTP-backed `CreateDocumentRevision` produce identical pages;
- `new_document_revision`, `update_document`, and their local-file paths retain
  schema parity; and
- `airplan upgrade` rebuilds older revision pages with the new renderer
  generation.

Transport parity tests should assert the resulting marker and rendered-page
behavior rather than adding redundant protocol fields. The server capability
document continues to advertise marker v6 and existing bundle limits.

`airplan preview` remains a preview of an unlinked document and therefore does
not invent revision history or diff controls. Browser tests can use the existing
fixture-generation path to materialize linked revision pages and control
objects locally.

## 11. Browser implementation details

Keep the existing one-shot page initializer. Add small helpers for:

- resolving the current upload directory from the versions metadata URL;
- validating a fetched v6 marker and producing a logical-path page map;
- selecting a same-page or entry fallback destination;
- preserving fragments according to section 5.3;
- showing and clearing the selector busy state; and
- activating the entry all-changes panel from the reserved fragment.

Use DOM text APIs for all metadata-derived labels. Treat marker strings as
untrusted until validated. Never place fetched HTML into the document.

The content-mode controller should discover the modes present in the template
instead of assuming Markdown's rendered/source pair. It must handle:

- `Rendered`, `Source`, and optional `Changes`;
- `Code` and optional `Changes`;
- print-time restoration to the primary view; and
- a direct all-changes fragment on initial load, reload, Back, and Forward.

The `All changes` and `Back to document` controls are ordinary anchors. The
runtime reacts to initial load and `hashchange` to activate the complete view,
but does not call `pushState` or `replaceState`. Cross-page and cross-revision
movement always remains a real navigation.

Refactor the narrow navigation controllers at the same time:

- treat the server-rendered Pages and ToC lists as the source for their
  enhanced copies;
- replace the Pages modal controller with a top-anchored popover controller;
- make ToC trigger visibility depend on the current content mode, media query,
  and open-menu state rather than scroll position;
- coordinate Pages and Contents so only one can be open;
- measure the sticky mobile bar and publish its height through a CSS custom
  property for fragment offsets; and
- close transient menus before a content-mode or all-changes transition.

Do not listen to every scroll event merely to keep the ToC button visible. Once
the inline list no longer controls visibility, media-query, content-mode,
popover, and dialog events are sufficient. Retain heading observation or the
current bounded scroll work only for active-section highlighting inside the ToC
list.

## 12. Accessibility and responsive layout

- Keep the native revision `<select>` with the existing accessible name.
- Mark the selector's wrapper `aria-busy="true"` during marker resolution and
  disable the select to prevent duplicate decisions.
- Give `All changes` a normal link target so it works with modifier keys,
  copying, and browser status previews.
- Keep `aria-pressed` accurate for page content modes.
- Use `aria-current` only for the active page and selected content mode, not for
  the revision-level all-changes link.
- Move focus to the all-changes heading only when activation follows an
  in-document user action. Initial deep links retain normal browser focus.
- At narrow widths, allow the revision selector and `All changes` link to wrap
  without colliding with the Pages, file, and theme controls.
- Give every icon-only sticky-bar action a unique accessible name, visible
  tooltip, 44px touch target, and clear focus treatment.
- Keep the Pages panel a navigation landmark, not an application-style
  `role="menu"`; its children remain ordinary links.
- Expose the Pages trigger's expanded state and associate it with the popover.
  Native light dismiss and Escape behavior must return focus predictably.
- Preserve logical focus order: sticky document actions, content modes,
  revision controls, then document content. Visual placement must not reorder
  them with positive `tabindex` values.
- Keep the permanent ToC trigger out of the tab order whenever its related
  rendered view is inactive or either transient menu is open.
- Reduced-motion readers receive the same navigation and fragment behavior
  without cross-document animation.

## 13. Compatibility and specification updates

Update SPEC.md in the implementation change to define:

- exact logical-path preservation and entry fallback during revision selection;
- the selected-marker validation and failure fallback;
- page-local `Changes` eligibility;
- `Code` and `Changes` behavior for managed text pages;
- the entry-only `All changes` view and reserved fragment;
- the permanent narrow-layout ToC trigger and its content-mode visibility;
- the sticky mobile document bar and its retained actions;
- the top-anchored Pages menu, enhancement fallback, and interaction with the
  Contents dialog;
- complete-report coverage for page descriptors, ordering, and assets;
- inline limits for each page section and the complete report;
- custom-template fields; and
- renderer-generation upgrade behavior.

Update IMPLEMENTATION.md with the structured diff model, marker lookup flow,
render projection, and upgrade parsing path. Update the original bundle plan's
revision-navigation paragraph to point to this follow-up decision.

If this work lands in PR #97 before spec 0.41.0 merges, keep the working spec
and implementation version at 0.41.0 because the published baseline is still
0.40.0. If it lands after 0.41.0 is published, apply the semver rules at the top
of SPEC.md and bump both documents together.

Existing generated revision pages continue to work. They keep entry-only
revision navigation and bundle-wide Markdown Changes views until upgraded.
Versions metadata v1, marker v6, raw diff URLs, CLI output, REST/OpenAPI, MCP,
and stored capability URLs remain compatible.

## 14. Implementation sequence

1. **Contract and fixtures**
   - Update SPEC.md, IMPLEMENTATION.md, and the original bundle plan.
   - Add a four-page revision fixture with two changed pages, one unchanged
     Markdown page, one changed source page, an asset change, and a removed
     page in a later revision.
2. **Structured diff generation**
   - Represent page, asset, ordering, and metadata changes explicitly.
   - Serialize the canonical report deterministically and correct bundle
     unified headers to include logical paths.
   - Add strict parsing for upgrades and legacy single-page fallback.
3. **Render projection**
   - Add explicit page-local and complete-diff render fields.
   - Project one page section per managed page and the complete report only to
     the entry.
   - Add revision identity metadata to managed text/source children.
4. **Built-in template and styles**
   - Implement the Markdown and code mode matrix.
   - Add the revision-controls row and entry all-changes panel.
   - Separate content modes from mobile sticky document actions.
   - Replace collapsed Pages with the top-anchored document menu and make the
     floating ToC trigger permanent in the rendered view.
   - Keep print, narrow layout, themes, safe areas, and custom templates
     correct.
5. **Revision selection runtime**
   - Validate same-directory current revision state on child pages.
   - Fetch and validate the selected v6 marker.
   - Preserve logical page and fragments, with entry fallback.
6. **Lifecycle integration**
   - Use the structured model in revision creation, first-link promotion, and
     upgrades.
   - Preserve spooling, checksums, upload order, reconciliation, and retries.
7. **Generated assets and documentation**
   - Regenerate browser assets and rendering goldens.
   - Update README revision examples if they currently describe entry-only
     switching or bundle-wide page Changes.
8. **End-to-end verification**
   - Run focused Go and browser tests while each behavior lands, then run the
     full project gates in section 15.

## 15. Verification strategy

### 15.1 Diff and renderer tests

- Confirm a four-page revision changing two pages gives `Changes` only to those
  two pages.
- Confirm the entry's page-local view excludes changes from child pages.
- Confirm a changed `server.go` page has `Code` and `Changes`, while an
  unchanged source page has only `Code`.
- Cover added, removed, descriptor-only, reordered, asset-only, and unchanged
  page cases.
- Assert the complete report contains every classified change in deterministic
  order.
- Assert logical paths appear in unified headers and remain parseable with
  spaces and other allowed path characters.
- Exercise exact 512 KiB page-section and complete-report boundaries
  independently, plus the existing 32 MiB canonical limit.
- Prove child HTML does not embed an unrelated page section or the complete
  report.
- Prove the entry embeds its own section and, when small enough, the complete
  report as separate views.
- Cover legacy single-page diff parsing during upgrade.

### 15.2 Revision lifecycle tests

- Create revision 2 from a standalone multi-page revision 1 and assert every
  promoted child has correct page identity and selector metadata.
- Upgrade an existing linked bundle and assert each page receives only its own
  adjacent section.
- Fail upgrade after selected page writes, retry, and prove digest-based resume
  converges with the entry written last.
- Confirm a metadata-only or asset-only revision still has `All changes` and no
  false page-local `Changes` modes.
- Confirm no-op detection remains unchanged and consumes no revision number.
- Compare direct and HTTP-backed revision results and marker inventories.

### 15.3 Browser tests

Use the existing Chromium suite across desktop, narrow, light, and dark
projects:

- Switch from `guide.md` to a revision where `guide.md` exists and assert the
  destination remains `guide.html`.
- Switch from a removed page and assert the destination is the target entry.
- Repeat with a nested source page such as `examples/server.go.html`.
- Preserve an ordinary fragment only for a same-page destination.
- Preserve `#airplan-all-changes` across revisions greater than 1 and drop it
  for revision 1.
- Return to the prior page with Back and move forward again through real
  document navigations.
- Simulate marker 404, malformed JSON, wrong version, wrong chain, mismatched
  entrypoint, traversal, and duplicate mapping. Each case must open the target
  entry and never an unvalidated URL.
- Verify the selector busy state prevents duplicate navigation.
- Verify changed and unchanged Markdown and code mode controls, including
  print behavior.
- Deep-link directly to `#airplan-all-changes`, reload it, and navigate Back.
- Confirm narrow revision controls wrap without hiding Pages, file, or theme
  actions.
- Confirm the inline ToC is absent after narrow enhancement and the floating
  ToC trigger is visible before scrolling, midway through the page, and at the
  bottom.
- Confirm the ToC trigger hides for Source, Changes, Code, and All changes, then
  returns when the rendered view becomes active.
- Confirm the mobile bar remains at the viewport top while scrolling, respects
  safe-area inset, and does not cover fragment targets.
- Confirm Pages stays labeled while Copy, Download, Raw, and Appearance are
  icon-only with accessible names and 44px hit targets.
- Confirm Pages opens below its sticky trigger as a top-origin popover rather
  than the Contents bottom sheet. Test light dismiss, Escape, trigger toggling,
  current-page focus, navigation, and focus restoration.
- Confirm opening Pages closes Contents and temporarily hides its floating
  trigger. Confirm widening restores desktop rails and closes the popover.
- Disable Popover API or native dialog support in focused tests and assert the
  corresponding inline navigation remains usable.
- Prove document lifetime changes on cross-page and cross-revision navigation.

Manually inspect current Chrome and Safari for native cross-document transition
quality. Confirm current Firefox performs the same usable navigations without
the transition effect. Test reduced motion in at least one supporting browser.

### 15.4 Handoff gates

Run:

```console
GOENV=off mise run check
GOENV=off mise run test:browser
GOENV=off mise run test-integration
mise run generate:check
mise run check:spec-sync
git diff --check
```

Native Windows CI remains required evidence for generated output and path
behavior. Because this change affects public rendered HTML, exercise an actual
two-revision, multi-page upload before handoff and inspect page preservation,
entry fallback, page-local changes, all changes, and browser history.

For this plan-only change, Markdown formatting and `git diff --check` are
proportionate. It changes no executable behavior.

## 16. Risks and mitigations

| Risk                                                                       | Mitigation                                                                                                                              |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| A fetched marker redirects revision navigation outside the selected upload | Strictly validate origin, revision identity, entrypoint, page paths, and same-directory rendered URLs before using a mapping            |
| Marker latency makes selection feel stalled                                | Fetch only one bounded control object after selection, expose a busy state, and fall back to the already validated entry URL on failure |
| Page-local extraction disagrees with the stored raw report                 | Generate stored bytes and render sections from one structured model; round-trip the strict upgrade parser in tests                      |
| Combined diff bytes are copied into every page                             | Embed the complete report only in the entry and pass one bounded local section to each changed child                                    |
| An unchanged page exposes misleading revision UI                           | Derive `PageChanged` from the classified adjacent change model, not from the existence of a bundle diff                                 |
| Source pages remain unable to switch revisions                             | Emit revision identity on every managed built-in page and test nested text/source children                                              |
| Versions metadata reaches its 64 KiB limit sooner                          | Keep page maps in per-revision v6 markers and leave the replicated index unchanged                                                      |
| An old or damaged marker blocks revision selection                         | Treat the marker as an optional page-preservation aid and navigate to the validated entry on any failure                                |
| Authored content collides with the reserved fragment                       | Use a dedicated data attribute for panel activation and treat the fragment as an Airplan runtime command                                |
| Upgrade produces mixed old and new page UI after failure                   | Keep checksum-based resume and entry-last writes; the next upgrade deterministically rebuilds every desired page                        |
| A sticky bar covers headings reached through ToC or copied fragments       | Publish its measured height as a CSS property and apply matching scroll padding and heading margins                                     |
| Pages and Contents still feel interchangeable                              | Give Pages a top-anchored document-menu shape and motion while Contents keeps its bottom-right trigger and bottom sheet                 |
| Mobile controls crowd or wrap unpredictably                                | Keep Pages labeled, make applicable utility actions icon-only, omit unavailable actions, and test the smallest supported viewport       |
| Enhanced mobile navigation removes the no-JavaScript fallback              | Hide server-rendered lists only after each controller and required browser API initialize successfully                                  |

## 17. Explicit non-goals

- Storing a page map in `.airplan-versions.json`.
- Per-page diff objects or per-page diff URLs.
- Fuzzy page identity, rename tracking, or redirect pages for removed content.
- Client-side HTML fetching, content replacement, routing, or custom history
  entries for page navigation.
- Preserving transient content mode across full document navigations.
- Adding revision history to authored HTML documents.
- Making `preview` synthesize a revision chain.
- Changing CLI, REST, OpenAPI, or MCP request and result schemas.
- Rewriting already published page HTML outside an explicit `upgrade`.

## 18. Unresolved questions

None. The product and compatibility choices needed to implement this follow-up
are settled above.
