set shell := ["bash", "-cu"]

binary := "fme"
cmd := "go/cmd/fme"
modules := "go/analytics go/core go/marketplace go/browser go/store go/capture go/cmd/fme"

# List available recipes
default:
    @just --list

# One-time setup after cloning: Chrome profile, .env, deps, build, and browsers.
# Safe to re-run; see `just setup --help` for flags.
setup *args:
    @./scripts/setup.sh {{args}}

# Diagnose a partial or broken setup (read-only; never changes anything)
doctor:
    @./scripts/doctor.sh

# Download and tidy dependencies for every module
tidy:
    @for m in {{modules}}; do echo "tidy $m"; (cd "$m" && go mod tidy); done

# Format Go source in every module
fmt:
    @for m in {{modules}}; do (cd "$m" && go fmt ./...); done

# Run the Go vet static analyzer in every module
vet:
    @for m in {{modules}}; do (cd "$m" && go vet ./...); done

# Run golangci-lint in every module
lint:
    @for m in {{modules}}; do (cd "$m" && golangci-lint run ./...); done

# Run the test suite in every module
test:
    @for m in {{modules}}; do (cd "$m" && go test ./...); done

# Run the test suite with coverage in every module
cover:
    @for m in {{modules}}; do (cd "$m" && go test -cover ./...); done

# Build the scraper binary
build: tidy
    go build -o {{binary}} ./{{cmd}}

# Install the scraper binary into $GOBIN (defaults to $GOPATH/bin)
install: build
    #!/usr/bin/env bash
    set -euo pipefail
    dest="${GOBIN:-$(go env GOPATH)/bin}"
    mkdir -p "$dest"
    install -m 0755 {{binary}} "$dest/{{binary}}"
    echo "installed $dest/{{binary}}"

# Run the scraper with arbitrary flags, e.g. `just run -action=serve`
run *args:
    go run ./{{cmd}} {{args}}

# Search Marketplace for a keyword, e.g. `just search "macbook m1"`
search keyword *args:
    go run ./{{cmd}} -action=search -keywords="{{keyword}}" {{args}}

# Pull descriptions for all details missing one (opens any listing to grab the PDP template)
pull-description *args:
    go run ./{{cmd}} -action=pull_description {{args}}

# Remove duplicate PDP responses, keeping the newest per product id
dedupe-pdp:
    go run ./{{cmd}} -action=dedupe_pdp

# Visit each known listing to fetch details
get-details:
    go run ./{{cmd}} -action=get_details

# Extract products from scraped responses into the database
save:
    go run ./{{cmd}} -action=save

# Extract listings from raw responses into data/__Details (no descriptions)
process-data:
    go run ./{{cmd}} -action=process_data

# Backfill missing URLs on existing detail files
fill-empty:
    go run ./{{cmd}} -action=fill_empty

# Report how many data/__Details listings have no description
description-stats:
    go run ./{{cmd}} -action=description_stats

# Build a product-density heat map (GeoJSON + HTML) from data/__Details.
# The only basemap is the keyless Esri one (-map=leaflet-esri, the default); see
# docs/heatmap_basemaps.md. Tune the grid with -cell=0.005; output goes to data/analytics/.
# Opening the .html as a file can block map tiles; prefer `just heatmap-serve`.
heatmap *args:
    go run ./{{cmd}} -action=heatmap {{args}}

# List the available heat map renderers (-map=<id>)
heatmap-maps:
    go run ./{{cmd}} -action=heatmap -list-maps

# Build the heat map and serve it on http://127.0.0.1:8799/<file>.html
# (serving avoids browsers blocking some basemaps for file:// pages).
heatmap-serve *args:
    go run ./{{cmd}} -action=heatmap_serve {{args}}

# Serve the PocketBase admin/web UI
serve:
    go run ./{{cmd}} -action=serve

# Archive the scraped data directory
backup:
    #!/usr/bin/env bash
    set -euo pipefail
    time 7z a "../data_$(date +%s).zip" data/

# Sync the scraped data/ dir with a remote host (push by default; merge, no delete).
# See `just sync --help` for flags (--pull, --delete, --dry-run, --save, --host, --dir).
sync *args:
    @./scripts/sync.sh {{args}}

# Create the Python virtualenv for the ml pipeline
ml-venv:
    python3 -m venv ml/.venv

# Install ml pipeline dependencies
ml-install: ml-venv
    ml/.venv/bin/pip install -r ml/requirements.txt

# Train the price prediction models
ml-train:
    cd ml && .venv/bin/python train.py

# Remove build artifacts
clean:
    rm -f {{binary}}
