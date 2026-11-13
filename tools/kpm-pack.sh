#!/bin/sh
# Build the KPM package and the one-package repository index from dist/netbird-kindle.
#
#   tools/kpm-pack.sh VERSION [BASE_URL]
#
# VERSION is a tag such as v0.2.0; anything after the three numbers is ignored.
# BASE_URL is where the .kpkg will be downloaded from. It defaults to this version's GitHub
# release, and a file:// URL makes an index you can try on a Kindle before releasing.
set -eu

version=${1:?usage: kpm-pack.sh VERSION [BASE_URL]}
numbers=$(printf '%s\n' "$version" | sed -En 's/^v?([0-9]+)\.([0-9]+)\.([0-9]+).*/\1 \2 \3/p')
[ -n "$numbers" ] || numbers="0 0 0"
# shellcheck disable=SC2086
set -- $numbers "${2:-}"
major=$1 minor=$2 patch=$3
base=${4:-https://github.com/workshop144/netbird-kindle/releases/download/v$major.$minor.$patch}

root=$(cd "$(dirname "$0")/.." && pwd)
[ -d "$root/dist/netbird-kindle" ] || { echo "kpm-pack: run 'make dist' first" >&2; exit 1; }
stage="$root/dist/kpkg"
name="netbird-kindle_$major.$minor.${patch}_kindlehf-kindlepw2.kpkg"

rm -rf "$stage" && mkdir -p "$stage"
cp -R "$root/dist/netbird-kindle" "$stage/payload"
cp "$root/kpm/install.sh" "$root/kpm/uninstall.sh" "$root/kpm/launch.sh" "$stage/"
manifest() { sed -e "s/@MAJOR@/$major/" -e "s/@MINOR@/$minor/" -e "s/@PATCH@/$patch/" "$root/kpm/manifest.json"; }
manifest > "$stage/manifest.json"

# A .kpkg is a gzipped tar with the package's files at the top level.
( cd "$stage" && COPYFILE_DISABLE=1 tar -czf "../$name" manifest.json install.sh uninstall.sh launch.sh payload )

field() { manifest | sed -n "s/^  \"$1\": \(.*[^,]\),*\$/\1/p"; }
cat > "$root/dist/repo.json" <<JSON
{
  "manifest_version": 2,
  "id": "workshop144",
  "name": "netbird-kindle",
  "description": "NetBird on a Kindle that has no tunnel driver (https://github.com/workshop144/netbird-kindle)",
  "packages": {
    "netbird-kindle": {
      "name": $(field name),
      "author": $(field author),
      "description": $(field description),
      "artifacts": [
        {
          "url": "$base/$name",
          "version": [$major, $minor, $patch],
          "dependencies": [],
          "supported_platforms": $(field supported_platforms)
        }
      ]
    }
  }
}
JSON
echo "kpm-pack: dist/$name and dist/repo.json"
