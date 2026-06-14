.PHONY: dev web-install web-dev web-build go-build test test-go test-web check

dev:
	./scripts/dev.sh

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

go-build: web-build
	go build ./cmd/migate

test-go:
	go test ./...

test-web:
	cd web && npm test && npm run build

test: test-web test-go

check: test
