# Drilldown and Tokens

## Enabling drilldown

Two fields are required in `config.json` to activate drilldown:

```json
{
  "showDrilldown": true,
  "hasEventHandlers": true
}
```

> **Splunk caches `config.json` at REST layer startup.** After adding these fields to an already-installed app, you must **restart Splunk** before the Interactions panel appears in Dashboard Studio. Reinstalling alone is not enough.

---

## Drilldown API

Drilldown is on `VisualizationAPI`, imported from `@splunk/dashboard-studio-extension`:

```js
import { VisualizationAPI } from '@splunk/dashboard-studio-extension';
```

---

## Drilldown — JavaScript

### Element-based (recommended — enables Interactions panel in Dashboard Studio)

Register a DOM node. The `action` field is an event type string (e.g. `'value'`, `'linkTo'`). `payloadCallback` takes **no arguments** and returns a **flat** `{ name, value }` object (no `payload` wrapper). The returned `value` is what the Interactions UI exposes — a "Set token" or "Use predefined token" drilldown on field `value` receives it directly. Because no event is passed, close over the data you need rather than reading `event.target`.

```js
VisualizationAPI.addDrilldownListener({
    node: myElement,
    action: 'value',
    payloadCallback: () => ({
        name: 'myField',
        value: selectedValue, // closed over, not from an event arg
    }),
});
```

### Programmatic (from custom event logic)

```js
element.addEventListener('click', () => {
    VisualizationAPI.triggerDrilldown({
        action: 'setToken',
        payload: { name: 'selected_host', value: 'host-1' },
    });
});
```

`triggerDrilldown` accepts an optional `originalEvent` property for the originating DOM event.

---

## Drilldown — React

```jsx
function Bar({ label, value }) {
    const handleClick = () => {
        VisualizationAPI.triggerDrilldown({
            action: 'setToken',
            payload: { name: 'selected_item', value: label },
        });
    };
    return <div onClick={handleClick}>{label}: {value}</div>;
}
```

---

## Drilldown actions

Shape depends on the API: `triggerDrilldown` wraps args as `{ action, payload }`, while `addDrilldownListener`'s `payloadCallback` returns a **flat** object (no `payload` key).

| `action` | Used with | Description |
|----------|-----------|-------------|
| `setToken` | `triggerDrilldown` | Set a dashboard token. `payload: { name, value }` |
| `linkTo` | `triggerDrilldown` | Navigate to a URL or dashboard. `payload: { value }` |
| `value` | `addDrilldownListener` | General value action. `payloadCallback` returns flat `{ name, value }` |

---

## Canvas drilldown pattern

For canvas-based vizzes, `payloadCallback` fires synchronously before any `click` event listener — you cannot set state in a click handler and read it inside the callback. Track mouse position via `mousemove` and hit-test inside `payloadCallback` directly.

```js
let lastMouseX = -1, lastMouseY = -1;
let renderedNodes = []; // populated after each render: [{ name, x, y, radius }, ...]

function hitTestNode(cx, cy) {
    for (const n of renderedNodes) {
        const dx = cx - n.x, dy = cy - n.y;
        if (dx*dx + dy*dy <= (n.radius + 4) * (n.radius + 4)) return n;
    }
    return null;
}

canvas.addEventListener('mousemove', event => {
    const rect = canvas.getBoundingClientRect();
    lastMouseX = event.clientX - rect.left;
    lastMouseY = event.clientY - rect.top;
    canvas.style.cursor = hitTestNode(lastMouseX, lastMouseY) ? 'pointer' : 'default';
});

canvas.addEventListener('mouseleave', () => {
    canvas.style.cursor = 'default';
});

VisualizationAPI.addDrilldownListener({
    node: canvas,
    action: 'value',
    payloadCallback: () => {
        const hit = hitTestNode(lastMouseX, lastMouseY);
        return hit ? { name: 'node', value: hit.name } : { name: 'node', value: '' };
    },
});
```

At the end of `render()`, update `renderedNodes`:

```js
renderedNodes = nodes.map(n => ({ name: n.name, x: n.x, y: n.y, radius: nodeSize }));
```

---

## Tokens

Tokens are dashboard-level variables shared across all panels. Setting a token updates it dashboard-wide — every panel and input that references it reacts.

### Enable in config.json

```json
{
  "canSetTokens": ["dynamic", "static"]
}
```

- `"dynamic"` — tokens set at runtime (from user interaction)
- `"static"` — tokens set at configuration time
- Empty array — viz cannot set tokens

### Read tokens — JavaScript

```js
VisualizationAPI.addTokensListener(({ tokens }) => {
    const selectedHost = tokens?.selected_host;
    updateDisplay(selectedHost);
}, { invokeImmediately: true });
```

### Read tokens — React

```jsx
import { useTokens } from '@splunk/dashboard-studio-extension/react';

function MyViz() {
    const { tokens } = useTokens();
    const selectedHost = tokens?.selected_host;
    return <div>Selected: {selectedHost ?? 'none'}</div>;
}
```

### Set a token (via drilldown action)

Token-setting is done through the drilldown mechanism:

```js
VisualizationAPI.triggerDrilldown({
    action: 'setToken',
    payload: { name: 'selected_host', value: 'host-1' },
});
```

---

## Mode awareness

`setOptions` and token-setting via drilldown only have effect in the correct mode. Use `addModeListener` / `useMode` to conditionally show edit-only affordances.

### JavaScript

```js
VisualizationAPI.addModeListener(({ mode }) => {
    document.getElementById('edit-controls').style.display =
        mode === 'edit' ? 'block' : 'none';
}, { invokeImmediately: true });
```

### React

```jsx
import { useMode } from '@splunk/dashboard-studio-extension/react';

function MyViz() {
    const { mode } = useMode();
    return (
        <div>
            {mode === 'edit' && <EditControls />}
            <Chart />
        </div>
    );
}
```
