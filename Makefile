.PHONY: dev web-install web-dev web-build go-build test test-go test-web check release clean-dist

dev:
	./scripts/dev.sh

web-install:
	cd web && npm ci

web-dev:
	cd web && npm run dev

web-build:
	./scripts/build-web.sh

go-build: web-build
	go build ./cmd/migate

test-go:
	go test ./...

test-web:
	cd web && npm ci --prefer-offline --no-audit && npm test && npm run build

test: test-web test-go

check:
	./scripts/check.sh

release:
	packaging/build-release.sh

clean-dist:
	rm -rf dist internal/web/static/dist
