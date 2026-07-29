# config.json Reference

Each visualization directory requires a `config.json` that defines its identity, data requirements, configurable options, and editor panel.

## Full structure

```json
{
  "showTitleAndDescription": true,
  "includeInToolbar": true,
  "includeInVizSwitcher": true,
  "showDrilldown": false,
  "canSetTokens": [],
  "hasEventHandlers": false,
  "config": {
    "name": "My Visualization",
    "description": "One-line description",
    "category": "Custom",
    "icon": null,
    "dataContract": {
      "requiredDataSources": ["primary"],
      "optionalDataSources": []
    },
    "size": {
      "initialWidth": 600,
      "initialHeight": 400
    },
    "optionsSchema": {},
    "editorConfig": []
  }
}
```

## Top-level fields

| Field | Type | Description |
|-------|------|-------------|
| `showTitleAndDescription` | boolean | Show the panel title/description controls in the editor |
| `includeInToolbar` | boolean | Show in the Dashboard Studio visualization list |
| `includeInVizSwitcher` | boolean | Show in the visualization type switcher |
| `showDrilldown` | boolean | Enable drilldown support. Must be `true` to use `addDrilldownListener` or `triggerDrilldown`. After enabling on an already-installed app, **restart Splunk** — the REST layer caches `config.json` at startup. |
| `canSetTokens` | string[] | `["dynamic"]`, `["static"]`, or both. Empty array disables token-setting. |
| `hasEventHandlers` | boolean | Required alongside `showDrilldown: true` to activate drilldown event handling. |

## config fields

| Field | Description |
|-------|-------------|
| `name` | Display name in the Splunk UI and visualization picker |
| `description` | Shown in the visualization picker |
| `category` | Toolbar grouping label (e.g. `"Custom"`) |
| `icon` | Path to an icon image relative to the viz directory, or `null` for the default icon |
| `dataContract.requiredDataSources` | Usually `["primary"]` — tells Splunk a search result is required |
| `dataContract.optionalDataSources` | Additional named data sources the viz can consume but doesn't require |
| `size.initialWidth/Height` | Pixel dimensions when first dropped onto a dashboard |
| `optionsSchema` | Defines configurable properties and their types/defaults |
| `editorConfig` | Defines the editor panel UI in the Dashboard Studio sidebar |

---

## optionsSchema

Defines the options the visualization accepts. Uses a JSON Schema subset.

```json
"optionsSchema": {
    "barColor": {
        "type": "string",
        "default": "#4e9cf5"
    },
    "showLegend": {
        "type": "boolean",
        "default": true
    },
    "maxItems": {
        "type": "number",
        "default": 10
    }
}
```

**Important**: Defaults declared here are not guaranteed to be present at runtime. Always provide fallback values in your visualization code:

```js
const color = options.barColor ?? '#4e9cf5';
```

---

## editorConfig

Defines the sidebar editor panel. Each entry is a section with a label and a 2D `layout` array. Each row in `layout` is an array of editor widgets displayed side by side.

```json
"editorConfig": [
    {
        "label": "Appearance",
        "layout": [
            [
                {
                    "label": "Bar color",
                    "editor": "editor.color",
                    "option": "barColor"
                }
            ],
            [
                {
                    "label": "Show legend",
                    "editor": "editor.checkbox",
                    "option": "showLegend"
                },
                {
                    "label": "Max items",
                    "editor": "editor.number",
                    "option": "maxItems"
                }
            ]
        ]
    }
]
```

Each widget has:
- `label` — display label shown in the sidebar
- `editor` — editor component type (see below)
- `option` — key in `optionsSchema` this widget controls
- `editorProps` — optional additional props (required for `editor.select`)

### Editor types

| `editor` value | Input type | Notes |
|---------------|------------|-------|
| `editor.color` | Color picker | Returns a hex/rgba string |
| `editor.text` | Text input | Returns a string |
| `editor.number` | Numeric input | Returns a number |
| `editor.checkbox` | Checkbox / toggle | Returns a boolean |
| `editor.select` | Dropdown | Requires `editorProps.values` — **not** `items` |

### Select example

```json
{
    "label": "Alignment",
    "editor": "editor.select",
    "option": "align",
    "editorProps": {
        "values": [
            { "label": "Left",   "value": "left"   },
            { "label": "Center", "value": "center" },
            { "label": "Right",  "value": "right"  }
        ]
    }
}
```

Using `items` instead of `values` produces "No valid values provided to editor" in Dashboard Studio and the dropdown will be empty.

---

## Minimal config (no options)

```json
{
  "showTitleAndDescription": true,
  "includeInToolbar": true,
  "includeInVizSwitcher": true,
  "showDrilldown": false,
  "canSetTokens": [],
  "hasEventHandlers": false,
  "config": {
    "name": "My Viz",
    "description": "Does something useful",
    "category": "Custom",
    "icon": null,
    "dataContract": { "requiredDataSources": ["primary"], "optionalDataSources": [] },
    "size": { "initialWidth": 600, "initialHeight": 400 },
    "optionsSchema": {},
    "editorConfig": []
  }
}
```
