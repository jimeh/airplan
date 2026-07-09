---
name: airplan
description: >-
  Upload a plan, design doc, report, or source file and hand the user
  a shareable, browser-ready link. Use when the user asks to "share
  this plan", "upload the plan", "give me a link to the plan/doc",
  wants a document they can open in a browser or send to someone, or
  when presenting a finished plan/report that a human should review
  outside the terminal. Works for markdown, HTML, and plain-text or
  source files.
---

# airplan

`airplan` uploads a document to S3-compatible storage under an
unguessable URL and prints that URL. One command, one link, done.

## Core workflow

1. Write the document to a file — markdown and HTML are equal
   citizens; use whichever you already produced. Plain-text and
   source files work too (rendered as a highlighted code page).
2. Run `airplan <file>`.
3. Capture stdout — it is the URL and **nothing else** — and present
   it to the user as a clickable link.

```sh
airplan plan.md
# → https://plans.example.com/vq3nhk2p7r4wzt5c6ydjm3xhqd/plan.html
```

## Rules

- stdout carries only the URL (or one JSON object with `--json`).
  Warnings and errors go to stderr; a non-zero exit means no URL was
  produced — never fabricate or reuse one.
- Prefer `--json` when scripting: `{"url","key","source_url",
"bucket","bytes","content_type"}` on a single line. `source_url`
  is omitted for HTML input and under `--no-source`.
- stdin works: `airplan -` (defaults to markdown). Give it a name
  with `--slug` and `--title` since there's no filename to infer
  from.
- Configuration comes from `~/.config/airplan/config.toml`,
  `AIRPLAN_*` env vars, or flags — if the tool errors about missing
  endpoint/bucket, tell the user to configure it rather than
  guessing values.

## Useful flags

| Flag                     | Purpose                                      |
| ------------------------ | -------------------------------------------- |
| `--json` / `-j`          | machine-readable result                      |
| `--slug S` / `-s`        | filename part of the URL                     |
| `--title T` / `-t`       | page title                                   |
| `--format md\|html\|txt` | override detection                           |
| `--lang go`              | highlight language for text from stdin       |
| `--profile P` / `-p`     | named config profile                         |
| `--no-source`            | don't upload the original alongside the page |
| `--open` / `-o`          | open in browser (interactive use)            |

To inspect the rendered page locally without uploading it, run
`airplan preview plan.md > plan.html` or use `--output plan.html`.
Preview uses the same renderer but does not require S3 credentials or
write the upload manifest.

## Examples

```sh
# Share a plan the user asked to review
airplan --title "Auth refactor plan" auth-plan.md

# Pipe a report straight from the conversation
cat report.md | airplan --slug q3-report -

# Share a source file as a highlighted, linkable page
airplan pkg/server/handler.go

# Scripted: capture the URL
url=$(airplan --json plan.md | jq -r .url)
```

Caveats to relay when relevant: anyone with the link can read the
page (that is the point — the URL itself is the capability), and
uploads persist until deleted.
