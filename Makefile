SHELL := /bin/bash

.PHONY: deps build build-arm64 build-intel run check format test-ui test-fixtures \
	test-core test-adapters check-core-coverage fixture-host run-desktop-fixture

deps:
	GOTOOLCHAIN=local go mod download
	cd apps/desktop && flutter pub get --enforce-lockfile

build:
	bash scripts/build_macos.sh

build-arm64:
	bash scripts/build_macos.sh arm64

build-intel:
	bash scripts/build_macos.sh x86_64

run:
	cd apps/desktop && flutter run -d macos --no-pub

check:
	bash scripts/check_toolchains.sh
	test -z "$$(gofmt -l cmd internal api)"
	GOTOOLCHAIN=local go vet ./...
	GOTOOLCHAIN=local go test -race ./...
	cd apps/desktop && dart format --output=none --set-exit-if-changed lib test
	cd apps/desktop && flutter analyze --no-pub
	$(MAKE) test-ui test-fixtures
	swift format lint --strict --recursive platform/macos/Package.swift platform/macos/Sources
	plutil -lint platform/macos/ShipqPlatform/Info.plist platform/macos/LaunchAgents/com.shipq.host.plist
	bash -n scripts/check_toolchains.sh scripts/build_macos.sh

format:
	gofmt -w cmd internal api
	cd apps/desktop && dart format lib test
	swift format format --in-place --recursive platform/macos/Package.swift platform/macos/Sources

test-ui:
	cd apps/desktop && flutter test --no-pub

test-fixtures:
	python3 -m unittest discover -s tests/fixtures -p 'test_*.py'

test-core test-adapters check-core-coverage fixture-host run-desktop-fixture:
	@printf '%s requires foundation tasks T020-T021; it is not implemented yet.\n' '$@' >&2
	@exit 2
