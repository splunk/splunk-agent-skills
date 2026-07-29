# Dashboard Studio Definition Reference

Working JSON shapes for a Studio (`<dashboard version="2">`) definition. The canonical full-dashboard template comes first; the rest are curated, individually-validated snippets to drop into it. Every snippet here has been rendered against Splunk and confirmed to work.

## When to load

Load for **every** conversion (step 9 of the workflow) — this is the source of the on-disk XML wrapper, the full-dashboard JSON structure, and the validated data-source / layout / input / time-range snippets.

---

## On-disk file format

A Studio dashboard is stored as an XML wrapper whose `<definition>` holds the JSON as CDATA:

```xml
<dashboard version="2" theme="light">
    <label>My Dashboard</label>
    <description></description>
    <definition><![CDATA[
        { ...the JSON definition below... }
    ]]></definition>
    <assets><![CDATA[{}]]></assets>
</dashboard>
```

`theme` is `light` or `dark`. `<label>`/`<description>` mirror the JSON `title`/`description`.

---

## Canonical full template

Assemble every conversion into this structure. Keys with no content for a given dashboard may be omitted (e.g. drop `inputs` if there are no form inputs).

```json
{
  "title": "<from classic <label>>",
  "description": "<from classic <description>, or empty string>",
  "dataSources":    { "ds_<id>": { ... } },
  "inputs":         { "input_<id>": { ... } },
  "visualizations": { "viz_<id>": { ... } },
  "defaults":       { "tokens": { "default": { ... } } },
  "expressions":    { "conditions": { ... }, "eval": { ... } },
  "layout": {
    "globalInputs": ["input_<id>", "..."],
    "options": { "submitButton": true },
    "tabs": { "items": [{ "label": "New tab", "layoutId": "layout_1" }] },
    "layoutDefinitions": {
      "layout_1": {
        "type": "<absolute | grid>",
        "options": {
          "width": 1440,
          "height": "<computed>",
          "display": "auto",
          "backgroundColor": "<see viz-mapping.md Visual parity>"
        },
        "structure": [
          { "item": "viz_<id>", "type": "block", "position": { "x": 0, "y": 0, "w": 1440, "h": 350 } }
        ]
      }
    }
  }
}
```

- The `layout` node holds a `tabs` list plus a `layoutDefinitions` object (one entry per tab). Everything else is a top-level key.
- For **grid** layout, omit `display: "auto"` from `layoutDefinitions.layout_1.options`.
- `backgroundColor` goes inside `layoutDefinitions.layout_1.options`, not `layout.options`.
- Always include top-level `title` and `description`, and list every form input in `layout.globalInputs`.

---

## Minimal complete example

The smallest valid dashboard — one table fed by one search, on a full-width grid row:

```json
{
  "title": "Example",
  "description": "",
  "dataSources": {
    "ds_main": {
      "type": "ds.search",
      "options": { "query": "| makeresults count=5 | streamstats count as n | table n" },
      "name": "Main Search"
    }
  },
  "visualizations": {
    "viz_main": { "type": "splunk.table", "title": "Results", "dataSources": { "primary": "ds_main" } }
  },
  "layout": {
    "globalInputs": [],
    "tabs": { "items": [{ "label": "New tab", "layoutId": "layout_1" }] },
    "layoutDefinitions": {
      "layout_1": {
        "type": "grid",
        "options": { "width": 1440, "height": 300, "backgroundColor": "#FFFFFF" },
        "structure": [
          { "item": "viz_main", "type": "block", "position": { "x": 0, "y": 0, "w": 1440, "h": 250 } }
        ]
      }
    }
  }
}
```

---

## Data sources

```json
"dataSources": {
  "ds_standalone": {
    "type": "ds.search",
    "options": {
      "query": "<original SPL>",
      "queryParameters": { "earliest": "-24h@h", "latest": "now" }
    },
    "name": "Standalone Search"
  },
  "ds_base": {
    "type": "ds.search",
    "options": { "query": "index=_internal" },
    "name": "Base Search"
  },
  "ds_chain": {
    "type": "ds.chain",
    "options": { "extend": "ds_base", "query": "| stats count" },
    "name": "Chain Search"
  }
}
```

- A `ds.chain` references its parent via `extend`. A base search referenced directly (no chained SPL) needs no chain entry — point the viz's `dataSources.primary` at the base key.
- Datasource keys and `name` values allow letters, numbers, spaces, dashes, underscores, periods — no parentheses or slashes.

---

## Grid layout rule

Every grid row must span the full canvas width (1440px) with **no gaps**. A panel that does not reach a canvas edge or a neighboring panel on every side renders a `Viz panel incorrectly configured` error (e.g. `viz_b expected a viz or canvas edge directly to its right at x=1200`).

| Panels per row | Width each |
|---|---|
| 1 | 1440 |
| 2 | 720 |
| 3 | 480 |
| 4 | 360 |

```json
"structure": [
  { "item": "viz_a", "type": "block", "position": { "x": 0,   "y": 0, "w": 720, "h": 250 } },
  { "item": "viz_b", "type": "block", "position": { "x": 720, "y": 0, "w": 720, "h": 250 } }
]
```

Absolute layout has no such constraint — panels may be placed at any x/y with gaps.

---

## Dynamic dropdown input

Search-populated dropdowns use a `dataSources.primary` plus an `encoding` block. See `form-inputs.md` for label≠value and mixed static+dynamic variants.

```json
"inputs": {
  "input_src": {
    "type": "input.dropdown",
    "title": "Source",
    "options": { "token": "src" },
    "dataSources": { "primary": "ds_input_src" },
    "encoding": { "label": "primary.src", "value": "primary.src" }
  }
}
```

Reference result fields by name (`primary.<fieldName>`), not position (`primary[0]`).

---

## Global time range input

The default time picker most converted dashboards want:

```json
"inputs": {
  "input_global_trp": {
    "type": "input.timerange",
    "title": "Global Time Range",
    "options": { "token": "global_time", "defaultValue": "-24h@h,now" }
  }
},
"defaults": {
  "dataSources": {
    "global": {
      "options": { "queryParameters": { "earliest": "$global_time.earliest$", "latest": "$global_time.latest$" } }
    }
  }
}
```

`defaultValue` for a timerange is a comma-separated string, not an object. Reference it in a datasource as `$global_time.earliest$` / `$global_time.latest$`.
