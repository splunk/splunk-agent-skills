# Troubleshooting

**Interactions panel doesn't appear in Dashboard Studio**
→ `config.json` must have both `"showDrilldown": true` and `"hasEventHandlers": true`. After setting these on an already-installed app, **restart Splunk** — the REST layer caches `config.json` at startup and reinstall alone won't apply the change.

**Dropdown editor is empty / "No valid values provided"**
→ Use `editorProps.values` — not `items`. See `config-json.md`.

**`React is not defined` at runtime, or a `.jsx` viz is missing from `dist/`**
→ The scaffolded `build.mjs` enables `jsx: 'automatic'` only when the project's `--entry` ends in `.jsx`, and it only builds files matching that exact entry name. A `.jsx` viz in a `--entry=visualization.js` project is silently skipped (warning, exit 0). Make the entry extension match the project, or switch the project to `.jsx`. See "How the scaffolded build picks entry files" in SKILL.md.

**`var`/`const` scoping error from esbuild**
→ esbuild errors if `var x` and `const x` (or `let x`) appear in the same function scope, even in separate branches. Use distinct variable names per branch, or convert all declarations in the function to `const`/`let`.

**Canvas looks blurry or stretched during resize**
→ Use `addDimensionsListener` to set `canvas.width`/`canvas.height`, not `getBoundingClientRect()`. See `canvas.md`.

**Options appear `undefined` even though `optionsSchema` declares defaults**
→ `optionsSchema` defaults are not guaranteed at runtime. Always use fallbacks: `const color = options.barColor ?? '#4e9cf5'`.

**`payloadCallback` always returns empty / drilldown payload is stale**
→ `payloadCallback` fires synchronously before the `click` event. Do not rely on state set in a click handler. For canvas vizzes, track mouse position via `mousemove` and hit-test inside `payloadCallback` directly. See `canvas.md`.

**Viz appears blank after install**
→ Check browser console for errors. Common causes: missing `invokeImmediately: true` on `addDataSourcesListener`, or the SPL query produces no results for the configured time range.
