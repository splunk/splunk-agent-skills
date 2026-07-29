# HTML Panel Conversion

`<html>` panels convert to `splunk.markdown`. Convert automatically — no user confirmation needed.

## When to load

Load when the classic XML has an `<html>` panel.

## Supported conversions

- Each `<html>` block → its own `splunk.markdown` viz.
- Inline HTML formatting tags → Markdown equivalents (see Conversion rules).

## Unsupported — warn or ask

- `<iframe>` — no Studio equivalent. Warn the user and ask whether to skip those panels or proceed without them.
- `<script>` tags — strip silently, note in report.
- `<style>` tags and `<link>` stylesheets — strip silently, note in report.

## Conversion rules

- Each `<html>` block within a panel → its own `splunk.markdown` viz (one viz per block).
- Panel `<title>` → prepend as `# Title\n\n` to the markdown body of the **first** block only. Do **not** set a `title` field on the viz — `splunk.markdown` ignores it visually.
- Convert HTML to Markdown where possible:
  - `<p>` → text
  - `<a href="...">text</a>` → `[text](url)`
  - `<img src="...">` → `![](src)` (base64 src preserved as-is)
  - `<strong>`/`<b>` → `**text**`
  - `<em>`/`<i>` → `_text_`
  - `<br>` → `  \n` (two trailing spaces + newline)
  - `<h1>`–`<h6>` → `#`–`######`
  - `<ul>`/`<ol>` → `- item` / `1. item`
- If stripping `<script>`/`<style>`/`<link>` leaves an `<html>` block with no visible content, skip the viz entirely and note it.

## Report notes

- List all `<iframe>` panels skipped.
- Note any `<script>`, `<style>`, or `<link>` stripped.
- Note any `<html>` block skipped because stripping left no visible content.
