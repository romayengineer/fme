module github.com/romayengineer/fme/go/app

go 1.25.0

toolchain go1.25.14

require (
	github.com/joho/godotenv v1.5.1
	github.com/romayengineer/fme/go/core v0.0.0
)

replace github.com/romayengineer/fme/go/core => ../core
