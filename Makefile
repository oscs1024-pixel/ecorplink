.PHONY: build build-wails clean test test-integration run

build: build-wails

build-wails:
	./scripts/build_wails.sh

test:
	go test ./internal/... -v

test-integration:
	go test ./internal/... -v -tags=integration

clean:
	rm -rf build dist bin cmd/gui/assets cmd/gui/daemon

run:
	wails3 dev
