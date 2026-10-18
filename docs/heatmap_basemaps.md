# Heat map basemaps

The heat map embeds a Leaflet page backed by an online basemap. Only one basemap
is built in: **`leaflet-esri`** (Esri World Street Map), which renders **without an
API key**.

The CARTO and OpenStreetMap basemaps were removed because they **require an API
key** and did not render.

## Usage

```bash
just heatmap                 # writes data/analytics/heatmap.geojson + .leaflet-esri.html
just heatmap-serve           # build, then serve it on the printed http://127.0.0.1:... URL
just heatmap-maps            # list the available renderers
```

`-map=leaflet-esri` is the default and the only selectable value; `-map=all`
writes the same single page. Serving over HTTP (`just heatmap-serve`) is preferred
over opening the `.html` as a `file://` URL.

## Adding a provider that needs a key

The token plumbing is retained for future providers: `-map-token <key>` (or the
provider's environment variable, as declared by `TokenEnv` in
`go/analytics/heatmap/maps.go`) is passed into the template. Never commit real
tokens.
