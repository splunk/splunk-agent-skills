# Canvas reference

## Sizing

Always use `addDimensionsListener` as the source of truth for canvas dimensions — do not use `getBoundingClientRect()` to set `canvas.width`/`canvas.height`. CSS stretches the canvas bitmap during panel resize before `getBoundingClientRect()` catches up.

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

---

## Interactivity (click, hover, tooltip)

Store hit rects during render and hit-test on mouse events:

```js
let hitRects = []; // populated during render: { x, y, w, h, data }
let mouseX = -1, mouseY = -1;

function hitTest(cx, cy) {
    return hitRects.find(r => cx >= r.x && cx <= r.x + r.w && cy >= r.y && cy <= r.y + r.h) ?? null;
}

canvas.addEventListener('click', event => {
    const { left, top } = canvas.getBoundingClientRect();
    const hit = hitTest(event.clientX - left, event.clientY - top);
    if (hit) render(); // e.g. toggle state
});

canvas.addEventListener('mousemove', event => {
    const { left, top } = canvas.getBoundingClientRect();
    mouseX = event.clientX - left;
    mouseY = event.clientY - top;
    canvas.style.cursor = hitTest(mouseX, mouseY) ? 'pointer' : 'default';
    drawFrame(); // redraw with tooltip
});

canvas.addEventListener('mouseleave', () => {
    mouseX = -1; mouseY = -1;
    drawFrame();
});
```

If you use `requestAnimationFrame` for entrance animation, only redraw on mousemove once animation is complete.

---

## Drilldown

To wire canvas clicks to Dashboard Studio's Interactions panel, set both `showDrilldown: true` and `hasEventHandlers: true` in `config.json`.

`payloadCallback` fires synchronously before any `click` event listener — you cannot use a click handler to set state that the callback reads. Track mouse position via `mousemove` instead:

```js
let lastMouseX = -1, lastMouseY = -1;
let renderedNodes = []; // updated at end of render()

function hitTestNode(cx, cy) { /* ... */ }

canvas.addEventListener('mousemove', event => {
    const rect = canvas.getBoundingClientRect();
    lastMouseX = event.clientX - rect.left;
    lastMouseY = event.clientY - rect.top;
    canvas.style.cursor = hitTestNode(lastMouseX, lastMouseY) ? 'pointer' : 'default';
});

canvas.addEventListener('mouseleave', () => { canvas.style.cursor = 'default'; });

VisualizationAPI.addDrilldownListener({
    node: canvas,
    action: 'value',
    payloadCallback: () => {
        const hit = hitTestNode(lastMouseX, lastMouseY);
        return hit ? { name: 'node', value: hit.name } : { name: 'node', value: '' };
    },
});
```

`payloadCallback` takes no arguments and returns a flat `{ name, value }` (no `payload` wrapper). See `drilldown-and-tokens.md` for full drilldown details.
