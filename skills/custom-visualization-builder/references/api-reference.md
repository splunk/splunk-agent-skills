# Dashboard Extension API Reference

## JavaScript — `@splunk/dashboard-studio-extension`

```js
import { VisualizationAPI } from '@splunk/dashboard-studio-extension';
```

All `add*Listener` methods subscribe to state updates from Dashboard Studio and return a cleanup function. They accept an optional second argument:

```js
{
    signal?: AbortSignal;         // auto-cleanup via AbortController
    invokeImmediately?: boolean;  // fire callback immediately with current state on registration
}
```

Pass `{ invokeImmediately: true }` to `addDataSourcesListener` to receive the current data state immediately rather than waiting for the next update.

---

### Data

```js
VisualizationAPI.addDataSourcesListener(
    ({ dataSources, loading }) => {
        if (loading || !dataSources?.primary?.data) return;
        const { fields, columns } = dataSources.primary.data;
        render(fields, columns);
    },
    { invokeImmediately: true }
);

VisualizationAPI.getDataSources(); // → { dataSources, loading }
```

Data shape: `dataSources.primary.data` is `{ fields: [{name}, ...], columns: [[...], ...] }`. All values are strings.

### Options

```js
VisualizationAPI.addOptionsListener(({ options }) => { applyOptions(options); });
VisualizationAPI.getOptions();          // → { options }
VisualizationAPI.setOptions(newOpts);   // only works in edit mode; silently ignored in view mode
```

Always provide fallback values — options may be unset even if `optionsSchema` declares defaults.

### Dimensions

```js
VisualizationAPI.addDimensionsListener(({ width, height }) => { ... }, { invokeImmediately: true });
VisualizationAPI.getDimensions(); // → { width, height } in px
```

**For canvas vizzes, always use `addDimensionsListener` as the source of truth for canvas sizing** — do not use `getBoundingClientRect()` to set `canvas.width`/`canvas.height`. The CSS stretches the canvas bitmap during panel resize before `getBoundingClientRect()` catches up.

```js
VisualizationAPI.addDimensionsListener(({ width, height }) => {
    state.width = width;
    state.height = height;
    render();
}, { invokeImmediately: true });

function render() {
    const { width: w, height: h } = state;
    if (w <= 0 || h <= 0) return;
    const dpr = window.devicePixelRatio || 1;
    canvas.width  = w * dpr;
    canvas.height = h * dpr;
    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    // draw at w × h logical pixels
}
```

### Mode

```js
VisualizationAPI.addModeListener(({ mode }) => { ... }, { invokeImmediately: true });
VisualizationAPI.getMode(); // → 'view' | 'edit'
```

### Theme

```js
VisualizationAPI.addThemeListener(({ theme }) => { ... }, { invokeImmediately: true });
VisualizationAPI.getTheme(); // → 'light' | 'dark'
```

The viz runs in a sandboxed iframe — CSS custom properties from the host page are not inherited. Use hardcoded enterprise theme values for styling.

### Tokens

```js
VisualizationAPI.addTokensListener(({ tokens }) => { ... }, { invokeImmediately: true });
VisualizationAPI.getTokens(); // → { tokens }
```

Requires `canSetTokens` configured in `config.json`. See `drilldown-and-tokens.md`.

### Error state

```js
VisualizationAPI.setError('Human-readable message'); // shows error overlay on the viz
VisualizationAPI.clearError();                        // removes the error overlay
VisualizationAPI.getError();                          // → current error string or empty string
VisualizationAPI.addErrorListener(({ error }) => { ... });
```

Use `setError` to surface data validation problems (e.g. wrong field types) rather than silently rendering nothing.

### Drilldown

Requires `"showDrilldown": true` and `"hasEventHandlers": true` in `config.json`.

```js
// Register a DOM node as a drilldown target.
// payloadCallback takes NO arguments and returns a FLAT { name, value } object —
// no `payload` wrapper. Close over the data you need instead of reading an event.
VisualizationAPI.addDrilldownListener({
    node: domElement,
    action: 'value',
    payloadCallback: () => ({ name: 'category', value: selectedValue }),
});

// triggerDrilldown is different — it DOES take a wrapped { action, payload }:
VisualizationAPI.triggerDrilldown({
    action: 'setToken',
    payload: { name: 'my_token', value: 'host-1' },
    originalEvent: event, // optional
});
```

> **Two different shapes — don't conflate them.** `addDrilldownListener`'s `payloadCallback` returns a flat `{ name, value }`. `triggerDrilldown` takes `{ action, payload: { name, value } }`. Verified against a live Studio iframe: a flat-payload drilldown on field `value` correctly sets a token to the clicked label.

See `drilldown-and-tokens.md` for full patterns.

---

## React — `@splunk/dashboard-studio-extension/react`

```jsx
import {
    useDataSources,
    useDimensions,
    useOptions,
    useMode,
    useTheme,
    useTokens,
    useError,
} from '@splunk/dashboard-studio-extension/react';
```

React and React DOM are required peer dependencies: `yarn add react react-dom`

Render your component directly — do **not** wrap it in a provider:

```jsx
createRoot(document.getElementById('root')).render(<MyViz />);
```

Each hook subscribes directly to the underlying `VisualizationAPI`. Call them straight from your component.

### Hooks

| Hook | Returns | Notes |
|------|---------|-------|
| `useDataSources()` | `{ dataSources, loading }` | `dataSources.primary.data` → `{ fields, columns }` |
| `useDimensions()` | `{ width, height }` | pixels |
| `useOptions()` | `{ options, setOptions }` | `setOptions` only works in edit mode |
| `useMode()` | `{ mode }` | `'view'` or `'edit'` |
| `useTheme()` | `{ theme }` | `'light'` or `'dark'` |
| `useTokens()` | `{ tokens }` | requires `canSetTokens` in config.json |
| `useError()` | `{ error, setError, clearError }` | `error` is a string or empty string |

Always provide option fallback values:

```jsx
const { options } = useOptions();
const color = options?.barColor ?? '#4e9cf5';
```

---

## Data shape reference

```js
// dataSources.primary.data
{
    fields: [{ name: 'host' }, { name: 'count' }],
    columns: [
        ['host-1', 'host-2'],   // columns[0] — all values for fields[0]
        ['42',     '17'],       // columns[1] — all values for fields[1]
    ]
}
// Access: columns[fieldIndex][rowIndex]
// All values are strings — parse numerics with parseFloat / parseInt
```

Convert to row-oriented format:

```js
const { fields, columns } = dataSources.primary.data;
const fieldNames = fields.map(f => f.name);
const rows = columns[0].map((_, i) =>
    Object.fromEntries(fieldNames.map((name, j) => [name, columns[j][i]]))
);
// rows[0] → { host: 'host-1', count: '42' }
```

---

## Iframe architecture notes

- The viz runs in a sandboxed iframe — `window.parent` is not accessible
- CSS custom properties from the host page are not inherited
- All state (data, options, theme, tokens) arrives asynchronously — nothing is available synchronously at startup
- The first callback fires when the initial state is ready — gate rendering behind the loading/null checks
- `@splunk/themes` token values are not available at runtime — use hardcoded enterprise theme values for styling
