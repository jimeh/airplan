import "./theme.ts";

import {
  iconClose,
  iconCopied,
  iconCopy,
  iconFailed,
  iconToc,
} from "../../airplan/assets/generated/icons.ts";

interface RevisionMetadata {
  deleted?: boolean;
  diff_url?: string;
  number: number;
  safeURL?: string | null;
  url: string;
}

interface VersionsMetadata {
  chain_id: string;
  current_revision: number;
  last_assigned_revision: number;
  latest_revision: number;
  revisions: RevisionMetadata[];
  schema: string;
  version: number;
}

interface MarkerPage {
  format: string;
  lang: string;
  page: string;
  path: string;
  source?: string;
  title?: string;
}

interface MarkerObject {
  bytes: number;
  content_type: string;
  name: string;
  role: string;
  sha256: string;
}

interface DocumentMarker {
  created_at: string;
  directory: string;
  entrypoint: string;
  format: string;
  kind: string;
  objects: MarkerObject[];
  pages: MarkerPage[];
  producer: { name: string; version: string };
  repo?: string;
  render: {
    generation: number;
    indexable: boolean;
    mermaid_url?: string;
    no_external_assets: boolean;
    template: { kind: string; sha256?: string };
    themes: { catalog_sha256: string; default_dark: string; default_light: string };
  };
  revision: { chain_id: string; number: number; previous_url?: string };
  schema: string;
  slug: string;
  title?: string;
  version: number;
}

(function () {
  "use strict";
  var d = document;
  var markerMaxBytes = 256 * 1024;
  const renderedElement = d.getElementById("rendered");
  if (!renderedElement) return;
  const rendered: HTMLElement = renderedElement;

  // Markdown pages are revision-ready even while standalone. The control
  // object intentionally does not exist until revision 2 is uploaded, so a
  // 404 is the ordinary dormant state. A nonce and no-store prevent a cached
  // negative response from hiding linkage added after this page was opened.
  var versionsMeta = d.querySelector<HTMLMetaElement>('meta[name="airplan-versions"]');
  var revisionChainMeta = d.querySelector<HTMLMetaElement>('meta[name="airplan-revision-chain"]');
  var pagePathMeta = d.querySelector<HTMLMetaElement>('meta[name="airplan-page-path"]');
  var entrypointMeta = d.querySelector<HTMLMetaElement>('meta[name="airplan-entrypoint"]');
  var versionsControlURL = versionsMeta
    ? new URL(versionsMeta.content, window.location.href)
    : null;
  var currentUploadURL = versionsControlURL ? new URL("./", versionsControlURL) : null;
  var currentUploadParts = currentUploadURL
    ? currentUploadURL.pathname.split("/").filter(Boolean)
    : [];
  var servicePathParts = currentUploadParts.slice(0, -1);
  function safeRevisionURL(raw: unknown, diff: boolean) {
    if (typeof raw !== "string") return null;
    try {
      var parsed = new URL(raw);
      if (
        parsed.origin !== window.location.origin ||
        parsed.username ||
        parsed.password ||
        parsed.search ||
        parsed.hash
      )
        return null;
      var parts = parsed.pathname.split("/").filter(Boolean);
      if (
        parts.length !== servicePathParts.length + 2 ||
        !servicePathParts.every(function (part, index) {
          return parts[index] === part;
        }) ||
        !/^[a-z2-7]{26}$/.test(parts[parts.length - 2])
      ) {
        return null;
      }
      var name = parts[parts.length - 1];
      if (diff ? name !== ".airplan-changes.diff" : !name.endsWith(".html")) {
        return null;
      }
      return parsed.href;
    } catch {
      return null;
    }
  }

  function portableMarkerPath(value: unknown) {
    if (typeof value !== "string" || value === "" || value.startsWith("/") || value.includes("\\"))
      return false;
    var parts = value.split("/");
    return parts.every(function (part) {
      var lowerPart = part.toLowerCase();
      var hasControl = Array.from(part).some(function (character) {
        var code = character.codePointAt(0) || 0;
        return code < 32 || code === 127;
      });
      if (
        !part ||
        part === "." ||
        part === ".." ||
        lowerPart.startsWith(".airplan-") ||
        lowerPart === ".airplan.json" ||
        hasControl ||
        /[. ]$/.test(part) ||
        /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(part)
      )
        return false;
      return true;
    });
  }

  function objectURL(directory: URL, value: unknown) {
    if (!portableMarkerPath(value)) return null;
    var relative = String(value)
      .split("/")
      .map(function (part) {
        return encodeURIComponent(part);
      })
      .join("/");
    var parsed = new URL(relative, directory);
    if (
      parsed.origin !== directory.origin ||
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash ||
      !parsed.pathname.startsWith(directory.pathname)
    )
      return null;
    return parsed.href;
  }

  async function readBoundedMarker(response: Response) {
    if (!response.ok) throw new Error("marker request failed");
    var declared = response.headers.get("content-length");
    if (declared && /^\d+$/.test(declared) && Number(declared) > markerMaxBytes) {
      if (response.body) await response.body.cancel("marker is too large");
      throw new Error("marker is too large");
    }
    if (!response.body || typeof response.body.getReader !== "function")
      throw new Error("bounded marker stream is unavailable");
    var reader = response.body.getReader();
    var chunks: Uint8Array[] = [];
    var total = 0;
    try {
      for (;;) {
        var result = await reader.read();
        if (result.done) break;
        total += result.value.byteLength;
        if (total > markerMaxBytes) {
          await reader.cancel("marker is too large");
          throw new Error("marker is too large");
        }
        chunks.push(result.value);
      }
    } finally {
      reader.releaseLock();
    }
    var body = new Uint8Array(total);
    var offset = 0;
    chunks.forEach(function (chunk) {
      body.set(chunk, offset);
      offset += chunk.byteLength;
    });
    var text = new TextDecoder("utf-8", { fatal: true }).decode(body);
    validateJSONMemberNames(text);
    return JSON.parse(text);
  }

  function validateJSONMemberNames(text: string) {
    var offset = 0;
    function skipWhitespace() {
      while (/\s/.test(text[offset] || "")) offset += 1;
    }
    function readString() {
      if (text[offset] !== '"') throw new Error("JSON string is invalid");
      var start = offset++;
      while (offset < text.length) {
        var character = text[offset++];
        if (character === '"') return JSON.parse(text.slice(start, offset)) as string;
        if (character === "\\") offset += 1;
      }
      throw new Error("JSON string is incomplete");
    }
    function readValue() {
      skipWhitespace();
      if (text[offset] === "{") {
        offset += 1;
        skipWhitespace();
        var names = new Set<string>();
        if (text[offset] === "}") {
          offset += 1;
          return;
        }
        for (;;) {
          skipWhitespace();
          var name = readString();
          if (names.has(name)) throw new Error("JSON object has a duplicate field");
          names.add(name);
          skipWhitespace();
          if (text[offset++] !== ":") throw new Error("JSON object is invalid");
          readValue();
          skipWhitespace();
          var delimiter = text[offset++];
          if (delimiter === "}") return;
          if (delimiter !== ",") throw new Error("JSON object is invalid");
        }
      }
      if (text[offset] === "[") {
        offset += 1;
        skipWhitespace();
        if (text[offset] === "]") {
          offset += 1;
          return;
        }
        for (;;) {
          readValue();
          skipWhitespace();
          var delimiter = text[offset++];
          if (delimiter === "]") return;
          if (delimiter !== ",") throw new Error("JSON array is invalid");
        }
      }
      if (text[offset] === '"') {
        readString();
        return;
      }
      var scalar = text
        .slice(offset)
        .match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/);
      if (!scalar) throw new Error("JSON value is invalid");
      offset += scalar[0].length;
    }
    readValue();
    skipWhitespace();
    if (offset !== text.length) throw new Error("JSON has trailing content");
  }

  function validateMarker(
    raw: unknown,
    directory: URL,
    selected: RevisionMetadata,
    chainID: string,
  ) {
    if (!isRecord(raw)) throw new Error("marker is invalid");
    var marker = raw as unknown as DocumentMarker;
    var directoryParts = directory.pathname.split("/").filter(Boolean);
    var directoryName = directoryParts[directoryParts.length - 1] || "";
    if (
      marker.schema !== "airplan-upload" ||
      marker.version !== 6 ||
      marker.kind !== "document" ||
      marker.directory !== directoryName ||
      !/^[a-z2-7]{26}$/.test(marker.directory) ||
      !validMarkerTimestamp(marker.created_at) ||
      marker.format !== "md" ||
      !validMarkerSlug(marker.slug) ||
      marker.entrypoint !== marker.slug + ".html" ||
      !isRecord(marker.producer) ||
      marker.producer.name !== "airplan" ||
      typeof marker.producer.version !== "string" ||
      marker.producer.version.trim() !== marker.producer.version ||
      marker.producer.version === "" ||
      !validMarkerRender(marker.render) ||
      !isRecord(marker.revision) ||
      marker.revision.number !== selected.number ||
      marker.revision.chain_id !== chainID ||
      (marker.revision.number === 1
        ? marker.revision.previous_url !== undefined
        : typeof marker.revision.previous_url !== "string" ||
          !safeAbsoluteHTMLURL(marker.revision.previous_url)) ||
      !Array.isArray(marker.objects) ||
      !Array.isArray(marker.pages) ||
      marker.pages.length === 0
    )
      throw new Error("marker identity is invalid");
    var entry = objectURL(directory, marker.entrypoint);
    if (entry !== selected.safeURL) throw new Error("marker entrypoint is invalid");
    if (
      (marker.title !== undefined && typeof marker.title !== "string") ||
      (marker.repo !== undefined && !validMarkerRepository(marker.repo)) ||
      marker.objects.length === 0 ||
      marker.pages.length > 100
    )
      throw new Error("marker shape is invalid");
    var objectRoles = validateMarkerObjects(marker);
    var paths = new Set<string>();
    var foldedPaths = new Set<string>();
    var renderedObjects = new Set<string>();
    var pages = new Map<string, string>();
    marker.pages.forEach(function (page, pageIndex) {
      if (
        !isRecord(page) ||
        !portableMarkerPath(page.path) ||
        paths.has(page.path) ||
        foldedPaths.has(page.path.toLowerCase()) ||
        (page.format !== "md" && page.format !== "txt") ||
        typeof page.lang !== "string" ||
        (page.title !== undefined && typeof page.title !== "string") ||
        !portableMarkerPath(page.page) ||
        !portableMarkerPath(page.source)
      )
        throw new Error("marker page descriptor is invalid");
      var expectedPage = managedPageName(page.path, page.format);
      var expectedSource = page.path;
      if (pageIndex === 0) {
        expectedPage = marker.entrypoint;
        expectedSource = marker.slug + ".md";
        if (page.format !== marker.format) throw new Error("marker entry format is invalid");
      }
      if (page.page !== expectedPage || page.source !== expectedSource)
        throw new Error("marker generated page mapping is invalid");
      var rendered = objectURL(directory, page.page);
      if (!rendered || renderedObjects.has(rendered))
        throw new Error("marker page object is invalid");
      if (!objectURL(directory, page.source)) throw new Error("marker source object is invalid");
      if (objectRoles.get(page.page) !== "page" || objectRoles.get(page.source!) !== "source")
        throw new Error("marker page object relationship is invalid");
      var sourceType = marker.objects.find(function (object) {
        return object.name === page.source;
      })!.content_type;
      if (
        (page.format === "md" && sourceType !== "text/markdown; charset=utf-8") ||
        (page.format === "txt" && sourceType !== "text/plain; charset=utf-8")
      )
        throw new Error("marker source content type is invalid");
      paths.add(page.path);
      foldedPaths.add(page.path.toLowerCase());
      renderedObjects.add(rendered);
      pages.set(page.path, rendered);
    });
    if (hasMarkerAncestorConflict(foldedPaths)) throw new Error("marker page paths conflict");
    if (!paths.has(marker.pages[0].path) || pages.get(marker.pages[0].path) !== entry)
      throw new Error("marker entry page is invalid");
    if (
      renderedObjects.size !== marker.pages.length ||
      Array.from(objectRoles.values()).filter(function (role) {
        return role === "source";
      }).length !== marker.pages.length
    )
      throw new Error("marker page inventory is invalid");
    return pages;
  }

  function managedPageName(logical: string, format: string) {
    if (format !== "md") return logical + ".html";
    var slash = logical.lastIndexOf("/");
    var dot = logical.lastIndexOf(".");
    return (dot > slash ? logical.slice(0, dot) : logical) + ".html";
  }

  function validMarkerRender(render: DocumentMarker["render"]) {
    if (
      !isRecord(render) ||
      !isRecord(render.template) ||
      !isRecord(render.themes) ||
      !Number.isInteger(render.generation) ||
      render.generation <= 0 ||
      typeof render.indexable !== "boolean" ||
      typeof render.no_external_assets !== "boolean" ||
      !render.template ||
      (render.template.kind !== "builtin" && render.template.kind !== "custom") ||
      (render.mermaid_url !== undefined && !safeMermaidURL(render.mermaid_url)) ||
      !render.themes
    )
      return false;
    if (
      (render.template.kind === "builtin" && render.template.sha256 !== undefined) ||
      (render.template.kind === "custom" && !validDigest(render.template.sha256))
    )
      return false;
    return (
      validMarkerThemeID(render.themes.default_light) &&
      validMarkerThemeID(render.themes.default_dark) &&
      validDigest(render.themes.catalog_sha256)
    );
  }

  function validMarkerThemeID(value: unknown) {
    return (
      typeof value === "string" &&
      new TextEncoder().encode(value).byteLength <= 48 &&
      /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(value)
    );
  }

  function hasMarkerAncestorConflict(paths: Set<string>) {
    for (var candidate of paths) {
      var separator = candidate.indexOf("/");
      while (separator >= 0) {
        if (paths.has(candidate.slice(0, separator))) return true;
        separator = candidate.indexOf("/", separator + 1);
      }
    }
    return false;
  }

  function validDigest(value: unknown) {
    return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
  }

  function isRecord(value: unknown): value is Record<string, unknown> {
    return !!value && typeof value === "object" && !Array.isArray(value);
  }

  function validMarkerSlug(value: unknown) {
    return typeof value === "string" && value.length <= 64 && /^[a-z0-9-]+$/.test(value);
  }

  function validMarkerTimestamp(value: unknown) {
    if (typeof value !== "string") return false;
    var match = value.match(
      /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:[.,]\d+)?(Z|[+-]00:00)$/,
    );
    if (!match) return false;
    var year = Number(match[1]);
    var month = Number(match[2]);
    var day = Number(match[3]);
    var hour = Number(match[4]);
    var minute = Number(match[5]);
    var second = Number(match[6]);
    var leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
    var monthDays = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
    return (
      month >= 1 &&
      month <= 12 &&
      day >= 1 &&
      day <= monthDays[month - 1] &&
      hour <= 23 &&
      minute <= 59 &&
      second <= 59
    );
  }

  function validMarkerRepository(value: unknown) {
    if (typeof value !== "string" || value === "" || value.trim() !== value) return false;
    try {
      var parsed = new URL(value);
      if (
        parsed.protocol !== "https:" ||
        parsed.username ||
        parsed.password ||
        parsed.port ||
        parsed.search ||
        parsed.hash
      )
        return false;
      var parts = parsed.pathname.replace(/^\/+|\/+$/g, "").split("/");
      if (parts.length !== 2) return false;
      var owner = parts[0];
      var repository = parts[1].replace(/\.git$/, "");
      if (
        !owner ||
        !repository ||
        owner === "." ||
        owner === ".." ||
        repository === "." ||
        repository === ".." ||
        /[?#@:\\]/.test(owner + repository)
      )
        return false;
      return value === "https://" + parsed.hostname.toLowerCase() + "/" + owner + "/" + repository;
    } catch {
      return false;
    }
  }

  function validNormalizedContentType(value: unknown) {
    return (
      typeof value === "string" &&
      /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+(?:; [a-z0-9!#$&^_.+-]+=(?:[a-z0-9!#$&^_.+-]+|"(?:[^"\\\r\n]|\\.)*"))*$/.test(
        value,
      )
    );
  }

  function safeAbsoluteHTMLURL(value: string) {
    try {
      var parsed = new URL(value);
      return (
        (parsed.protocol === "https:" || parsed.protocol === "http:") &&
        !parsed.username &&
        !parsed.password &&
        !parsed.search &&
        !parsed.hash &&
        parsed.pathname.endsWith(".html")
      );
    } catch {
      return false;
    }
  }

  function safeMermaidURL(value: unknown) {
    if (typeof value !== "string") return false;
    try {
      var parsed = new URL(value);
      return (
        parsed.protocol === "https:" &&
        !!parsed.host &&
        !parsed.username &&
        !parsed.password &&
        !parsed.hash
      );
    } catch {
      return false;
    }
  }

  function validateMarkerObjects(marker: DocumentMarker) {
    var roles = new Map<string, string>();
    var folded = new Set<string>();
    var diffCount = 0;
    var pages = 0;
    var sources = 0;
    var assets = 0;
    marker.objects.forEach(function (object) {
      if (
        !isRecord(object) ||
        (!portableMarkerPath(object.name) && object.name !== ".airplan-changes.diff") ||
        roles.has(object.name) ||
        folded.has(object.name.toLowerCase()) ||
        !Number.isSafeInteger(object.bytes) ||
        object.bytes < 0 ||
        !validDigest(object.sha256) ||
        !validNormalizedContentType(object.content_type)
      )
        throw new Error("marker object inventory is invalid");
      if (object.role === "page") {
        pages += 1;
        if (object.bytes <= 0 || object.content_type !== "text/html; charset=utf-8")
          throw new Error("marker page object is invalid");
      } else if (object.role === "source") {
        sources += 1;
        if (object.bytes <= 0) throw new Error("marker source object is invalid");
      } else if (object.role === "asset") {
        assets += 1;
      } else if (object.role === "diff") {
        diffCount += 1;
        if (
          object.name !== ".airplan-changes.diff" ||
          object.bytes <= 0 ||
          object.content_type !== "text/plain; charset=utf-8"
        )
          throw new Error("marker diff object is invalid");
      } else throw new Error("marker object role is invalid");
      roles.set(object.name, object.role);
      folded.add(object.name.toLowerCase());
    });
    if (hasMarkerAncestorConflict(folded)) throw new Error("marker object paths conflict");
    if (
      pages !== marker.pages.length ||
      sources !== marker.pages.length ||
      pages + assets > 100 ||
      (marker.revision.number === 1 ? diffCount !== 0 : diffCount !== 1)
    )
      throw new Error("marker object counts are invalid");
    return roles;
  }

  function revisionDestination(entryURL: string, targetPage: string | null) {
    var hash = window.location.hash;
    if (hash === "#airplan-all-changes") return entryURL + hash;
    if (!targetPage) return entryURL;
    return targetPage + (hash && hash !== "#airplan-all-changes" ? hash : "");
  }
  function renderVersions(metadata: VersionsMetadata) {
    var currentMeta = d.querySelector<HTMLMetaElement>('meta[name="airplan-revision"]');
    var embedded = currentMeta ? Number(currentMeta.content) : Number(metadata.current_revision);
    if (
      !Number.isInteger(embedded) ||
      embedded <= 0 ||
      metadata.current_revision !== embedded ||
      !Number.isInteger(metadata.latest_revision) ||
      !Number.isInteger(metadata.last_assigned_revision) ||
      !Array.isArray(metadata.revisions) ||
      metadata.revisions.length === 0 ||
      metadata.last_assigned_revision !== metadata.revisions.length ||
      !/^[a-z2-7]{26}$/.test(metadata.chain_id) ||
      (revisionChainMeta && revisionChainMeta.content !== metadata.chain_id)
    ) {
      throw new Error("revision identity is invalid");
    }
    var invalidEntry = false;
    var previousNumber = 0;
    var live = metadata.revisions.filter(function (revision) {
      if (
        !revision ||
        !Number.isInteger(revision.number) ||
        revision.number !== previousNumber + 1
      ) {
        invalidEntry = true;
        return false;
      }
      previousNumber = revision.number;
      if (revision.deleted) return false;
      revision.safeURL = safeRevisionURL(revision.url, false);
      if (!revision.safeURL) {
        invalidEntry = true;
        return false;
      }
      if (revision.number > 1) {
        var safeDiff = safeRevisionURL(revision.diff_url, true);
        if (
          !safeDiff ||
          new URL(safeDiff).pathname.replace(/[^/]+$/, "") !==
            new URL(revision.safeURL).pathname.replace(/[^/]+$/, "")
        ) {
          invalidEntry = true;
          return false;
        }
      }
      return true;
    });
    if (
      invalidEntry ||
      metadata.revisions[0].number !== 1 ||
      !live.some(function (revision) {
        return revision.number === embedded;
      })
    )
      throw new Error("revision entries are invalid");
    var current = live.find(function (revision) {
      return revision.number === embedded;
    });
    var currentPage = new URL(window.location.href);
    currentPage.search = "";
    currentPage.hash = "";
    if (
      !current ||
      !currentUploadURL ||
      new URL(current.safeURL || "").pathname.replace(/[^/]+$/, "") !== currentUploadURL.pathname ||
      !currentPage.pathname.startsWith(currentUploadURL.pathname)
    ) {
      throw new Error("current revision URL is invalid");
    }
    var latest = Math.max.apply(
      null,
      live.map(function (revision) {
        return revision.number;
      }),
    );
    if (latest !== metadata.latest_revision) throw new Error("latest is invalid");

    var heading = d.querySelector("[data-revision-heading]");
    if (!heading) {
      heading = d.createElement("p");
      heading.className = "revision-heading";
      heading.setAttribute("data-revision-heading", "");
      var renderedView = d.getElementById("rendered");
      if (!renderedView) throw new Error("rendered view is unavailable");
      renderedView.prepend(heading);
    }
    var stale = embedded < latest;
    var label = stale
      ? "Revision " + embedded + " of " + latest
      : "Revision " + embedded + " (Latest)";
    var visualLabel = d.createElement("span");
    visualLabel.className = "revision-picker-label";
    visualLabel.textContent = label;
    visualLabel.setAttribute("aria-hidden", "true");
    var select = d.createElement("select");
    select.setAttribute("aria-label", "Document revision");
    live.forEach(function (revision) {
      var option = d.createElement("option");
      option.value = revision.safeURL || "";
      option.textContent =
        revision.number === latest
          ? "Revision " + revision.number + " (Latest)"
          : "Revision " + revision.number + " of " + latest;
      option.selected = revision.number === embedded;
      select.appendChild(option);
    });
    select.addEventListener("change", function () {
      var selected = select.selectedIndex;
      if (selected < 0 || selected >= live.length) return;
      var target = live[selected];
      var entryURL = target.safeURL || "";
      if (window.location.hash === "#airplan-all-changes") {
        window.location.assign(entryURL + (target.number > 1 ? "#airplan-all-changes" : ""));
        return;
      }
      var embeddedEntrypoint = entrypointMeta
        ? new URL(entrypointMeta.content, window.location.href).href
        : "";
      if (!pagePathMeta || currentPage.href === embeddedEntrypoint || !revisionChainMeta) {
        window.location.assign(entryURL);
        return;
      }
      heading!.setAttribute("aria-busy", "true");
      select.disabled = true;
      var selectedDirectory = new URL("./", entryURL);
      var markerURL = new URL(".airplan.json", selectedDirectory);
      markerURL.searchParams.set(
        "_airplan",
        Date.now().toString(36) + Math.random().toString(36).slice(2),
      );
      fetch(markerURL, { cache: "no-store", credentials: "same-origin" })
        .then(readBoundedMarker)
        .then(function (marker) {
          var pages = validateMarker(marker, selectedDirectory, target, revisionChainMeta!.content);
          window.location.assign(
            revisionDestination(entryURL, pages.get(pagePathMeta!.content) || null),
          );
        })
        .catch(function () {
          console.warn("airplan: selected revision page map is unavailable or invalid");
          window.location.assign(entryURL);
        });
    });
    heading.replaceChildren(visualLabel, select);
    heading.classList.add("is-picker");
    heading.classList.toggle("is-stale", stale);
    d.body.classList.toggle("airplan-stale-revision", stale);
  }
  if (versionsMeta) {
    var versionsURL = new URL(versionsMeta.content, window.location.href);
    versionsURL.searchParams.set(
      "_airplan",
      Date.now().toString(36) + Math.random().toString(36).slice(2),
    );
    fetch(versionsURL, { cache: "no-store", credentials: "same-origin" })
      .then(function (response) {
        if (response.status === 404) return null;
        if (!response.ok) throw new Error("metadata request failed");
        return response.json();
      })
      .then(function (metadata) {
        if (metadata === null) return;
        if (
          !metadata ||
          metadata.schema !== "airplan-versions" ||
          metadata.version !== 1 ||
          !Array.isArray(metadata.revisions) ||
          metadata.revisions.length < 2
        ) {
          throw new Error("metadata is invalid");
        }
        renderVersions(metadata as VersionsMetadata);
        window.dispatchEvent(
          new CustomEvent("airplan:versions", {
            detail: metadata,
          }),
        );
      })
      .catch(function () {
        console.warn("airplan: revision metadata is unavailable or invalid");
      });
  }

  // Visually-hidden live region so screen readers announce copy
  // feedback even when focus doesn't stay on the button.
  var live = d.createElement("div");
  live.className = "sr-status";
  live.setAttribute("aria-live", "polite");
  d.body.appendChild(live);

  // Printing should never omit collapsed disclosure content. Restore the
  // reader's open/closed state after the print dialog finishes.
  var printClosedDetails: HTMLDetailsElement[] | null = null;
  function expandDetailsForPrint() {
    if (printClosedDetails !== null) return;
    printClosedDetails = Array.from(d.querySelectorAll<HTMLDetailsElement>("details:not([open])"));
    printClosedDetails.forEach(function (details) {
      details.open = true;
    });
  }
  function restoreDetailsAfterPrint() {
    if (printClosedDetails === null) return;
    printClosedDetails.forEach(function (details) {
      details.open = false;
    });
    printClosedDetails = null;
  }
  window.addEventListener("beforeprint", expandDetailsForPrint);
  window.addEventListener("afterprint", restoreDetailsAfterPrint);

  // Buttons are icon-only: feedback is an icon swap (check on
  // success, x on failure) plus the live-region announcement.
  function flash(btn: HTMLButtonElement, text: string, ok: boolean) {
    live.textContent = text;
    var label = btn.querySelector(".action-label");
    var previousLabel = label ? label.textContent : "";
    if (label) label.textContent = ok ? "Copied" : "Failed";
    btn.classList.add(ok ? "is-copied" : "is-failed");
    btn.disabled = true;
    setTimeout(function () {
      btn.classList.remove("is-copied", "is-failed");
      btn.disabled = false;
      if (label) label.textContent = previousLabel;
    }, 1200);
  }

  function copyText(text: string, btn: HTMLButtonElement) {
    if (!navigator.clipboard) {
      flash(btn, "Copy failed", false);
      return;
    }
    navigator.clipboard.writeText(text).then(
      function () {
        flash(btn, "Copied!", true);
      },
      function () {
        flash(btn, "Copy failed", false);
      },
    );
  }

  // On narrow screens, replace the inline Pages list with a top-anchored
  // document popover only after the native controller is available.
  var pages = d.getElementById("pages");
  var pagesTrigger = d.querySelector<HTMLButtonElement>(".pages-trigger");
  var pagesPopover: HTMLElement | null = null;
  var pagesMedia = window.matchMedia("(max-width: 78rem)");
  var closeTocDialog = function () {};
  function pagesOpen() {
    return pagesPopover ? pagesPopover.matches(":popover-open") : false;
  }
  function closePagesPopover(restoreFocus: boolean) {
    if (!pagesPopover || !pagesOpen()) return;
    pagesPopover.hidePopover();
    if (restoreFocus && pagesTrigger && pagesMedia.matches) {
      setTimeout(function () {
        pagesTrigger!.focus();
      }, 0);
    }
  }
  if (pages && pagesTrigger) {
    var pagesList = pages.querySelector<HTMLOListElement>(".pages-list");
    if (pagesList) {
      var candidate = d.createElement("div");
      if ("popover" in candidate && typeof candidate.showPopover === "function") {
        pagesPopover = candidate;
        pagesPopover.className = "pages-popover";
        pagesPopover.id = "pages-popover";
        pagesPopover.setAttribute("popover", "auto");
        var pagesPopoverNav = d.createElement("nav");
        pagesPopoverNav.className = "pages-popover-nav";
        pagesPopoverNav.setAttribute("aria-label", "Pages");
        pagesPopoverNav.appendChild(pagesList.cloneNode(true));
        pagesPopover.appendChild(pagesPopoverNav);

        function positionPagesPopover() {
          if (!pagesTrigger || !pagesPopover) return;
          var rect = pagesTrigger.getBoundingClientRect();
          pagesPopover.style.setProperty("--pages-left", Math.max(16, rect.left) + "px");
          pagesPopover.style.setProperty("--pages-top", rect.bottom + "px");
          pagesPopover.style.setProperty(
            "--pages-width",
            Math.min(480, window.innerWidth - Math.max(16, rect.left) - 16) + "px",
          );
        }
        pagesTrigger.setAttribute("popovertarget", pagesPopover.id);
        pagesTrigger.popoverTargetElement = pagesPopover;
        pagesPopover.addEventListener("beforetoggle", function (event) {
          if ((event as ToggleEvent).newState !== "open") return;
          closeTocDialog();
          positionPagesPopover();
        });
        pagesPopover.addEventListener("toggle", function (event) {
          var open = (event as ToggleEvent).newState === "open";
          pagesTrigger!.setAttribute("aria-expanded", open ? "true" : "false");
          d.body.classList.toggle("pages-popover-open", open);
          if (open) {
            var current = pagesPopover!.querySelector('[aria-current="page"]');
            if (current) current.scrollIntoView({ block: "nearest" });
          }
          syncTocTrigger();
        });
        pagesPopoverNav.querySelectorAll<HTMLAnchorElement>("a").forEach(function (link) {
          link.addEventListener("click", function () {
            closePagesPopover(false);
          });
        });
        pagesMedia.addEventListener("change", function () {
          if (!pagesMedia.matches) closePagesPopover(false);
        });
        window.addEventListener("resize", function () {
          if (pagesOpen()) positionPagesPopover();
        });

        pagesTrigger.hidden = false;
        pagesTrigger.setAttribute("aria-expanded", "false");
        d.body.appendChild(pagesPopover);
        d.body.classList.add("pages-popover-ready");
      }
    }
  }

  // Rendered/source toggle.
  var source = d.getElementById("source");
  var changes = d.getElementById("changes");
  var allChanges = d.querySelector<HTMLElement>("[data-airplan-all-changes]");
  var toc = d.getElementById("toc");
  var tocTrigger: HTMLButtonElement | null = null;
  var tocDialog: HTMLDialogElement | null = null;
  var tocMedia = window.matchMedia("(max-width: 78rem)");

  closeTocDialog = function () {
    if (tocDialog && tocDialog.open) tocDialog.close();
  };

  function syncTocTrigger() {
    if (!toc || !tocTrigger || !tocDialog) return;
    var show = tocMedia.matches && !rendered.hidden && !tocDialog.open && !pagesOpen();
    tocTrigger.classList.toggle("is-visible", show);
    tocTrigger.tabIndex = show ? 0 : -1;
    tocTrigger.setAttribute("aria-hidden", show ? "false" : "true");
    if (tocDialog.open && (!tocMedia.matches || rendered.hidden)) {
      closeTocDialog();
    }
  }

  function activateView(view: string) {
    closePagesPopover(false);
    closeTocDialog();
    rendered.hidden = view !== "rendered";
    if (source) source.hidden = view !== "source";
    if (changes) changes.hidden = view !== "changes";
    if (toc) toc.hidden = view !== "rendered";
    d.querySelectorAll<HTMLButtonElement>(".viewtoggle button").forEach(function (button) {
      var active = button.dataset.view === view;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", active ? "true" : "false");
    });
    syncTocTrigger();
  }

  d.querySelectorAll<HTMLButtonElement>(".viewtoggle button").forEach(function (btn) {
    btn.addEventListener("click", function () {
      activateView(btn.dataset.view || "rendered");
    });
  });

  var focusAllChanges = false;
  d.querySelectorAll<HTMLAnchorElement>('.all-changes-link[href$="#airplan-all-changes"]').forEach(
    function (link) {
      link.addEventListener("click", function () {
        focusAllChanges = new URL(link.href).pathname === window.location.pathname;
      });
    },
  );
  function syncAllChanges() {
    var active = window.location.hash === "#airplan-all-changes" && !!allChanges;
    closePagesPopover(false);
    closeTocDialog();
    d.body.classList.toggle("all-changes-active", active);
    if (allChanges) allChanges.hidden = !active;
    if (active) {
      rendered.hidden = true;
      if (source) source.hidden = true;
      if (changes) changes.hidden = true;
      if (toc) toc.hidden = true;
      if (focusAllChanges) allChanges!.querySelector<HTMLElement>("h1")?.focus();
    } else {
      activateView("rendered");
    }
    focusAllChanges = false;
    syncTocTrigger();
  }
  window.addEventListener("hashchange", syncAllChanges);
  syncAllChanges();

  // Highlight the ToC entry nearest the top of the viewport. Links and
  // hierarchy are rendered server-side, so navigation still works when
  // JavaScript is disabled.
  if (toc) {
    var tocLinks = Array.from(toc.querySelectorAll<HTMLAnchorElement>('a[href^="#"]'));
    var tocList = toc.querySelector<HTMLOListElement>(".toc-list");

    if (tocList) {
      tocDialog = d.createElement("dialog");
      if (typeof tocDialog.showModal === "function") {
        tocDialog.className = "toc-dialog";
        tocDialog.id = "toc-dialog";
        tocDialog.setAttribute("aria-labelledby", "toc-dialog-title");

        var tocPanel = d.createElement("div");
        tocPanel.className = "toc-dialog-panel";
        var tocHeader = d.createElement("div");
        tocHeader.className = "toc-dialog-header";
        var tocTitle = d.createElement("h2");
        tocTitle.className = "toc-dialog-title";
        tocTitle.id = "toc-dialog-title";
        tocTitle.textContent = "Contents";
        var tocClose = d.createElement("button");
        tocClose.className = "toc-dialog-close";
        tocClose.type = "button";
        tocClose.setAttribute("aria-label", "Close table of contents");
        tocClose.innerHTML = iconClose;
        tocHeader.appendChild(tocTitle);
        tocHeader.appendChild(tocClose);

        var tocNav = d.createElement("nav");
        tocNav.className = "toc-dialog-nav";
        tocNav.setAttribute("aria-label", "Table of contents");
        tocNav.appendChild(tocList.cloneNode(true));
        tocPanel.appendChild(tocHeader);
        tocPanel.appendChild(tocNav);
        tocDialog.appendChild(tocPanel);

        tocTrigger = d.createElement("button");
        tocTrigger.className = "toc-trigger";
        tocTrigger.type = "button";
        tocTrigger.tabIndex = -1;
        tocTrigger.setAttribute("aria-label", "Open table of contents");
        tocTrigger.setAttribute("aria-controls", "toc-dialog");
        tocTrigger.setAttribute("aria-haspopup", "dialog");
        tocTrigger.setAttribute("aria-hidden", "true");
        tocTrigger.innerHTML = iconToc;

        d.body.appendChild(tocTrigger);
        d.body.appendChild(tocDialog);
        d.body.classList.add("toc-dialog-ready");

        tocTrigger.addEventListener("click", function () {
          closePagesPopover(false);
          tocDialog!.showModal();
          d.body.classList.add("toc-dialog-open");
          syncTocTrigger();
          var active = tocDialog!.querySelector("a.active");
          if (active) active.scrollIntoView({ block: "nearest" });
        });
        tocClose.addEventListener("click", closeTocDialog);
        tocDialog.addEventListener("click", function (event) {
          if (event.target === tocDialog) closeTocDialog();
        });
        tocDialog.addEventListener("keydown", function (event) {
          if (event.key === "Escape") {
            event.preventDefault();
            closeTocDialog();
          }
        });
        tocDialog.addEventListener("close", function () {
          d.body.classList.remove("toc-dialog-open");
          syncTocTrigger();
          if (tocTrigger!.classList.contains("is-visible")) {
            // Let the dialog leave the top layer before restoring focus.
            setTimeout(function () {
              tocTrigger!.focus();
            }, 50);
          }
        });
        tocNav.querySelectorAll<HTMLAnchorElement>("a").forEach(function (link) {
          link.addEventListener("click", closeTocDialog);
        });
      } else {
        tocDialog = null;
      }
    }

    var allTocLinks = tocLinks.slice();
    if (tocDialog) {
      allTocLinks = allTocLinks.concat(
        Array.from(tocDialog.querySelectorAll<HTMLAnchorElement>('a[href^="#"]')),
      );
    }
    var tocHeadings = tocLinks.map(function (link) {
      return d.getElementById((link.getAttribute("href") || "").slice(1));
    });
    function updateToc() {
      if (tocLinks.length === 0) {
        syncTocTrigger();
        return;
      }
      var current = 0;
      tocHeadings.forEach(function (heading, index) {
        if (heading && heading.getBoundingClientRect().top <= 128) {
          current = index;
        }
      });
      if (window.innerHeight + window.scrollY >= d.documentElement.scrollHeight - 2) {
        current = tocLinks.length - 1;
      }
      var activeHref = tocLinks[current].getAttribute("href");
      allTocLinks.forEach(function (link) {
        var active = link.getAttribute("href") === activeHref;
        link.classList.toggle("active", active);
        if (active) {
          link.setAttribute("aria-current", "location");
        } else {
          link.removeAttribute("aria-current");
        }
      });
      syncTocTrigger();
    }
    var tocFramePending = false;
    function scheduleTocUpdate() {
      if (tocFramePending) return;
      tocFramePending = true;
      window.requestAnimationFrame(function () {
        tocFramePending = false;
        updateToc();
      });
    }
    d.addEventListener("scroll", scheduleTocUpdate, { passive: true });
    window.addEventListener("resize", updateToc);
    updateToc();
  }

  var toolbar = d.querySelector<HTMLElement>(".toolbar");
  function publishToolbarHeight() {
    var height =
      toolbar && window.matchMedia("(max-width: 78rem)").matches
        ? toolbar.getBoundingClientRect().height
        : 0;
    d.documentElement.style.setProperty("--airplan-sticky-height", height + "px");
  }
  if (toolbar) {
    if (typeof ResizeObserver === "function") {
      new ResizeObserver(publishToolbarHeight).observe(toolbar);
    }
    window.addEventListener("resize", publishToolbarHeight);
    publishToolbarHeight();
  }

  // Copy the full original source. The highlighted block's text
  // content preserves the raw source exactly.
  const copySource = d.querySelector<HTMLButtonElement>(".copy-source");
  if (copySource && source) {
    copySource.addEventListener("click", function () {
      var pre = source!.querySelector("pre");
      copyText(pre ? pre.textContent : "", copySource);
    });
  }

  // Per-code-block copy buttons in the rendered view.
  rendered.querySelectorAll<HTMLPreElement>("pre").forEach(function (pre) {
    if (pre.classList.contains("mermaid")) return;
    var wrap = d.createElement("div");
    wrap.className = "codewrap";
    pre.parentNode?.insertBefore(wrap, pre);
    wrap.appendChild(pre);

    var btn = d.createElement("button");
    btn.className = "codecopy";
    btn.type = "button";
    btn.setAttribute("aria-label", "Copy code");
    btn.title = "Copy code";
    btn.innerHTML = iconCopy + iconCopied + iconFailed;
    btn.addEventListener("click", function () {
      var code = pre.querySelector("code");
      copyText((code || pre).textContent, btn);
    });
    wrap.appendChild(btn);
  });
})();
