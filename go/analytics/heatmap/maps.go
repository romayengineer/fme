package heatmap

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

// DefaultMapID is the map used when no -map is given.
const DefaultMapID = "leaflet-esri"

// MapInfo describes one selectable map renderer.
type MapInfo struct {
	ID            string // selector value, e.g. "leaflet-esri"
	Title         string // human-readable name
	Description   string
	RequiresToken bool   // true when the provider needs an API key/token
	TokenEnv      string // env var consulted when a token is required
	DocsURL       string
}

// MapOptions carries per-render values into a map template.
type MapOptions struct {
	Title  string
	Token  string            // API key/token for providers that need one
	Params map[string]string // reserved for provider-specific extras
}

type mapRenderer struct {
	info MapInfo
	tmpl *template.Template
}

// mapDefs is the ordered registry of built-in maps. Each entry maps to one
// embedded templates/<file>.
//
// Only keyless basemaps are shipped. CARTO and OpenStreetMap were dropped because
// they require an API key; Esri World Street Map renders without one. See
// docs/heatmap_basemaps.md.
var mapDefs = []struct {
	info MapInfo
	file string
}{
	{
		info: MapInfo{
			ID:          "leaflet-esri",
			Title:       "Leaflet + Esri",
			Description: "Leaflet with Esri World Street Map tiles (default; no API key).",
		},
		file: "leaflet-esri.html",
	},
}

var mapRenderers = mustLoadRenderers()

// mustLoadRenderers parses every embedded template once at init. A parse error is
// a programming mistake, so it panics rather than surfacing at request time.
func mustLoadRenderers() map[string]mapRenderer {
	renderers := make(map[string]mapRenderer, len(mapDefs))
	for _, d := range mapDefs {
		// Name the template after its file so the parsed definition becomes the
		// root we execute (these templates have no {{define}} blocks).
		tmpl := template.Must(template.New(d.file).ParseFS(templateFS, "templates/"+d.file))
		renderers[d.info.ID] = mapRenderer{info: d.info, tmpl: tmpl}
	}
	return renderers
}

// Maps returns every built-in map, in registry order.
func Maps() []MapInfo {
	infos := make([]MapInfo, 0, len(mapDefs))
	for _, d := range mapDefs {
		infos = append(infos, d.info)
	}
	return infos
}

// MapIDs returns the selectable map ids, in registry order.
func MapIDs() []string {
	ids := make([]string, 0, len(mapDefs))
	for _, d := range mapDefs {
		ids = append(ids, d.info.ID)
	}
	return ids
}

// MapByID looks up a map by id.
func MapByID(id string) (MapInfo, bool) {
	r, ok := mapRenderers[id]
	return r.info, ok
}

// RenderHTML renders the grid with the given map. An unknown id returns an error
// listing the valid ids.
func (g Grid) RenderHTML(mapID string, opts MapOptions) ([]byte, error) {
	renderer, ok := mapRenderers[mapID]
	if !ok {
		return nil, fmt.Errorf("unknown map %q (valid: %s)", mapID, strings.Join(MapIDs(), ", "))
	}

	geo, err := g.GeoJSON()
	if err != nil {
		return nil, fmt.Errorf("render geojson: %w", err)
	}

	data := templateData{
		Title:    opts.Title,
		Total:    g.Total,
		Cells:    len(g.Cells),
		MaxCount: g.MaxCount,
		CellDeg:  g.Options.CellDeg,
		GeoJSON:  template.JS(geo),
		Token:    opts.Token,
	}
	if data.Title == "" {
		data.Title = "Marketplace product heat map"
	}

	var buf bytes.Buffer
	if err := renderer.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render map %q: %w", mapID, err)
	}
	return buf.Bytes(), nil
}
