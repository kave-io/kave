#!/bin/sh
set -eu

lang="${1:-}"
version="${2:-}"

if [ -z "$lang" ] || [ -z "$version" ]; then
  echo "usage: scripts/bump-sdk.sh <go|py|ts> <new-version>" >&2
  exit 1
fi

version="${version#v}"
tag="sdk/$lang/v$version"
root="$(git rev-parse --show-toplevel)"

if [ -d "$root/sdk/$lang" ]; then
  sdk_dir="$root/sdk/$lang"
  sdk_git="$root"
  sdk_add_path="sdk/$lang"
elif [ -d "$root/../sdk/$lang" ]; then
  sdk_dir="$root/../sdk/$lang"
  sdk_git="$sdk_dir"
  sdk_add_path="."
else
  echo "error: sdk/$lang not found" >&2
  exit 1
fi

changelog="$sdk_dir/CHANGELOG.md"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT INT TERM

case "$lang" in
  go)
    ;;
  py)
    sed -i "s/^version = \".*\"/version = \"$version\"/" "$sdk_dir/pyproject.toml"
    ;;
  ts)
    sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"$version\"/" "$sdk_dir/package.json"
    ;;
  *)
    echo "error: unknown SDK language: $lang" >&2
    exit 1
    ;;
esac

if [ ! -f "$changelog" ]; then
  printf '# %s SDK Changelog\n\n' "$lang" > "$changelog"
fi

awk -v version="$version" '
  NR == 1 {
    print
    print ""
    print "## " version
    print ""
    print "- TODO: summarize release changes."
    inserted = 1
    next
  }
  { print }
  END {
    if (!inserted) {
      print "# SDK Changelog"
      print ""
      print "## " version
      print ""
      print "- TODO: summarize release changes."
    }
  }
' "$changelog" > "$tmp"
mv "$tmp" "$changelog"

git -C "$sdk_git" add "$sdk_add_path"
git -C "$sdk_git" commit -m "Release $tag"
git -C "$sdk_git" tag -a "$tag" -m "Release $tag"

echo "created annotated tag $tag"
