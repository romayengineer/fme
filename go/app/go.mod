module github.com/romayengineer/fme/go/app

go 1.25.0

require (
	github.com/joho/godotenv v1.5.1
	github.com/mxschmitt/playwright-go v0.6201.1
	github.com/romayengineer/fme/go/browser v0.0.0
	github.com/romayengineer/fme/go/capture v0.0.0
	github.com/romayengineer/fme/go/core v0.0.0
	github.com/romayengineer/fme/go/marketplace v0.0.0
	github.com/romayengineer/fme/go/store v0.0.0
)

require (
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
)

replace github.com/romayengineer/fme/go/browser => ../browser

replace github.com/romayengineer/fme/go/capture => ../capture

replace github.com/romayengineer/fme/go/core => ../core

replace github.com/romayengineer/fme/go/marketplace => ../marketplace

replace github.com/romayengineer/fme/go/store => ../store
