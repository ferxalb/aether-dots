.PHONY: help dev run bundle package cli-help

help:
	@printf '%s\n' \
		'make dev       Run the live-reload desktop app' \
		'make run       Run the desktop app once' \
		'make bundle    Build the macOS .app without a disk image' \
		'make package   Build the macOS .app and disk image' \
		'make cli-help  Show AETHER CLI commands'

dev:
	CGO_ENABLED=0 go tool mygo dev

run:
	CGO_ENABLED=0 go run .

bundle:
	CGO_ENABLED=0 go tool mygo build -skip-dmg

package:
	CGO_ENABLED=0 go tool mygo build

cli-help:
	CGO_ENABLED=0 go run ./cmd/aether --help
