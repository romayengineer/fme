# Location geocoding outliers (heat map mega-cells)

The heat map normally shows a few products per grid cell, but the Buenos Aires
corpus has one cell with **282** products. This document records the investigation
and the conclusion: it is a **data artifact of Facebook's city-level geocoding**,
not a bug in the aggregation or the renderer.

## Summary

When a listing's location is only a city (no street or neighborhood), Facebook
returns a **single fixed representative coordinate** for that city. Every such
listing therefore shares the same latitude/longitude and collapses into one grid
cell. The result is one artificial "mega-cell" that dominates the map.

## Observed symptom

Over the current `data/__Details` corpus (7-digit grid cells, `-cell=0.01`):

| Metric | Value |
|---|---|
| Detail files | 2,890 |
| Files with valid coordinates | 2,867 |
| Non-empty grid cells | 788 |
| Average products per cell | ~3.6 |
| Busiest cell | **282** |

The busiest `data/analytics/heatmap.geojson` feature is
`cell_x=-5842, cell_y=-3461` (lon `-58.42..-58.41`, lat `-34.61..-34.60`) — downtown
**Ciudad de Buenos Aires** (`CF`).

## Root cause and evidence

277 detail files carry the *byte-identical* coordinate
`-34.609680175781, -58.419799804688` (276 of them report
`LocationGeocodeCityName1 = "Ciudad de Buenos Aires"`; one has a null city field).
Together with 4 files at `-34.604187011719, -58.419799804688` and 1 at
`-34.60314, -58.41081`, they fall in the same 0.01° cell:

```
277  -34.609680175781, -58.419799804688
  4  -34.604187011719, -58.419799804688
  1  -34.60314,        -58.41081
---
282
```

The same effect recurs at smaller scale: many cities resolve to one exact point,
and a large share of Buenos Aires listings share the default longitude
`-58.419799804688`. The top duplicate coordinates:

| Count | Coordinate | City |
|---|---|---|
| 277 | `-34.609680175781, -58.419799804688` | Buenos Aires, Argentina |
| 46 | `-34.922790527344, -57.958374023438` | La Plata, Buenos Aires |
| 44 | `-34.648132324219, -58.617553710938` | Morón, Buenos Aires |
| 40 | `-34.582214355469, -58.419799804688` | Buenos Aires, Argentina |
| 38 | `-34.543762207031, -58.727416992188` | San Miguel, Buenos Aires |
| 37 | `-34.631652832031, -58.419799804688` | Buenos Aires, Argentina |
| 34 | `-38.020935058594, -57.573852539062` | Mar del Plata, Argentina |

Why the pipeline keeps them:

- **Binning.** `Aggregate` computes `int32(math.Floor(lat/cell))` and
  `int32(math.Floor(lon/cell))` (`go/analytics/heatmap/aggregate.go:24-28`), so
  identical coordinates always land in a single cell.
- **Validation.** `validCoord` (`go/analytics/source/filestore.go:156`) only rejects
  out-of-range values and Facebook's `(0,0)` "unknown" placeholder. A legitimate-looking
  city centroid like the Buenos Aires point passes through untouched.

## Impact

The mega-cell sets `Grid.MaxCount` (`go/analytics/heatmap/aggregate.go:63`), which is
the denominator used for intensity (`go/analytics/heatmap/aggregate.go:82`). Because
that one city-level point is the maximum, real density differences elsewhere are
compressed toward the low end of the intensity scale and washed out on the map.

## Reproduction

Read-only, from the repo root:

```bash
# Corpus size and valid points
ls data/__Details/detail_*.json | wc -l
for f in data/__Details/detail_*.json; do
  jq -r 'select(.LocationLatitud!=null and .LocationLongitude!=null) | 1' "$f"
done | wc -l

# Busiest cells
jq -r '.features[].properties.count' data/analytics/heatmap.geojson | sort -rn | head

# Duplicate coordinates and their cities
for f in data/__Details/detail_*.json; do
  jq -r 'select(.LocationLatitud!=null and .LocationLongitude!=null)
         | "\(.LocationLatitud),\(.LocationLongitude)\t\(.LocationGeocodeCityName2)"' "$f"
done | sort | uniq -c | sort -rn | head
```

## Where the data comes from

`LocationLatitud` / `LocationLongitude` and the `LocationGeocode*` fields are decoded
from the captured Marketplace responses and materialized into
`data/__Details/detail_<id>.json`. See
[redacted_description.md](redacted_description.md) for how those responses are
intercepted and merged.

## Mitigations (not implemented)

Options if the artifact should not dominate the map:

1. **Annotate** the cell (e.g. tooltip "city-level location / approximate") instead of
   hiding the data.
2. **Detect city-centroid fallbacks** — flag coordinates that are exact duplicates
   across many listings, or that match a known city geocode point, and separate or
   exclude them.
3. **Exclude flagged points from intensity scaling** so they do not set `MaxCount`.
4. **Jitter** defaulted points slightly so one fake hotspot does not dominate
   normalization.
