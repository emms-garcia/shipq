SHELL := /bin/bash

.PHONY: deps build build-arm64 build-intel run check format test-ui test-fixtures test-platform \
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
	test -z "$$(gofmt -l cmd internal api tests)"
	python3 scripts/check_architecture.py
	python3 -m unittest discover -s scripts/tests -p 'test_*.py'
	GOTOOLCHAIN=local go vet ./...
	$(MAKE) test-core test-adapters check-core-coverage
	cd apps/desktop && dart format --output=none --set-exit-if-changed lib test
	cd apps/desktop && flutter analyze --no-pub
	$(MAKE) test-ui test-fixtures test-platform
	swift format lint --strict --recursive platform/macos/Package.swift platform/macos/Sources platform/macos/Tests apps/desktop/macos/Runner
	plutil -lint platform/macos/ShipqPlatform/Info.plist platform/macos/LaunchAgents/com.shipq.host.plist
	bash -n scripts/check_toolchains.sh scripts/build_macos.sh

format:
	gofmt -w cmd internal api tests
	cd apps/desktop && dart format lib test
	swift format format --in-place --recursive platform/macos/Package.swift platform/macos/Sources platform/macos/Tests apps/desktop/macos/Runner

test-ui:
	cd apps/desktop && flutter test --no-pub

test-fixtures:
	python3 -m unittest discover -s tests/fixtures -p 'test_*.py'

test-platform:
	swift test --package-path platform/macos --scratch-path build/swift/tests

test-core:
	GOTOOLCHAIN=local go test -count=1 -race ./internal/domain/... ./internal/application/...

test-adapters:
	SHIPQ_PLATFORM_SMOKE=0 GOTOOLCHAIN=local go test -count=1 -race ./api/... ./cmd/... ./internal/adapters/... ./internal/host/... ./tests/platform/... ./tests/support/...

check-core-coverage:
	python3 scripts/check_core_coverage.py

fixture-host:
	mkdir -p build/fixture
	SHIPQ_FIXTURE_HANDOFF="$(CURDIR)/build/fixture/connection.json" GOTOOLCHAIN=local go run ./tests/support/fixturehost

run-desktop-fixture:
	python3 scripts/run_desktop_fixture.py
