.PHONY: web build build-linux test vet run clean

web:
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/webwallet ./cmd/server

# Binarios estáticos para VPS Linux, sin dependencias de runtime.
build-linux: web
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/webwallet-linux-amd64 ./cmd/server
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/webwallet-linux-arm64 ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

run: build
	./bin/webwallet -config settings.json

clean:
	rm -rf bin dist
	rm -rf internal/webui/dist/assets
