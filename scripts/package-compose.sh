#!/bin/sh
set -eu

version="${1:-${GORELEASER_CURRENT_TAG:-snapshot}}"
version="${version#v}"

root_dir="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
dist_dir="$root_dir/dist"
compose_dir="$root_dir/deploy/compose"
package_dir="$dist_dir/kave-compose_$version"

rm -rf "$package_dir"
mkdir -p "$package_dir"

for variant in standalone-sqlite standalone-postgres sidecar; do
  cp -R "$compose_dir/$variant" "$package_dir/$variant"
done

tar -C "$dist_dir" -czf "$dist_dir/kave-compose_$version.tar.gz" "kave-compose_$version"
cp "$compose_dir/standalone-sqlite/docker-compose.yml" "$dist_dir/compose.yml"
rm -rf "$package_dir"

echo "wrote dist/kave-compose_$version.tar.gz"
echo "wrote dist/compose.yml"
