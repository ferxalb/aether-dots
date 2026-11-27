#!/usr/bin/env bash
# Regenerates resources/licenses/third_party and NOTICE.txt from the modules
# actually compiled into the desktop app and the CLI on macOS (arm64, amd64)
# and Linux. Run after changing go.mod; commit the result.
set -euo pipefail
cd "$(dirname "$0")/.."

out=resources/licenses
third=$out/third_party
rm -rf "$third"
mkdir -p "$third"

modules=$(
	for target in darwin/arm64 darwin/amd64 linux/amd64; do
		GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 go list -deps \
			-f '{{if not .Standard}}{{with .Module}}{{.Path}} {{.Version}}{{end}}{{end}}' . ./cmd/aether
	done | grep -v '^aether-dots ' | sort -u
)

{
	echo "AETHER desktop and CLI distributions include software from the projects below."
	echo "Each project remains under its own license; this notice does not change those"
	echo "terms. Full license and notice files are in third_party/<module path>/."
	echo
	echo "- AETHER: FSL-1.1-ALv2 (AETHER-FSL-1.1-ALv2.txt)"
	echo "- Go toolchain $(go env GOVERSION) runtime and standard library: BSD-3-Clause (Go-BSD-3-Clause.txt)"
	while read -r path version; do
		dir=$(go list -m -f '{{.Dir}}' "$path")
		dest=$third/$path
		mkdir -p "$dest"
		found=0
		for f in "$dir"/*; do
			case "$(basename "$f")" in
			LICENSE* | LICENCE* | COPYING* | UNLICENSE* | NOTICE*)
				cp "$f" "$dest/"
				chmod 0644 "$dest/$(basename "$f")"
				found=1
				;;
			esac
		done
		if [ "$found" = 0 ]; then
			echo "no license file found for $path" >&2
			exit 1
		fi
		echo "- $path $version"
	done <<<"$modules"
	echo
	echo "The complete dependency graph and versions are recorded in go.mod and go.sum."
} >"$out/NOTICE.txt"
