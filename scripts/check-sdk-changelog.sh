#!/bin/sh
set -eu

lang="${1:?usage: check-sdk-changelog.sh <go|py|ts> <version>}"
version="${2:?usage: check-sdk-changelog.sh <go|py|ts> <version>}"
version="${version#v}"

root="$(git rev-parse --show-toplevel)"
if [ -d "$root/sdk/$lang" ]; then
  changelog="$root/sdk/$lang/CHANGELOG.md"
elif [ -d "$root/../sdk/$lang" ]; then
  changelog="$root/../sdk/$lang/CHANGELOG.md"
else
  echo "error: sdk/$lang not found" >&2
  exit 1
fi

if [ ! -f "$changelog" ]; then
  echo "error: missing $changelog" >&2
  exit 1
fi

entry="$(awk -v version="$version" '
  /^##[[:space:]]+/ {
    current = ($0 ~ ("(^|[[:space:]])v?" version "($|[^0-9])"))
    seen_next = NR > 1 && found
  }
  seen_next { exit }
  current {
    found = 1
    if ($0 !~ /^##[[:space:]]+/) print
  }
' "$changelog")"

if [ -z "$(printf '%s' "$entry" | tr -d '[:space:]')" ]; then
  echo "error: $changelog has no non-empty entry for $version" >&2
  exit 1
fi

if printf '%s\n' "$entry" | grep -qi 'TODO'; then
  echo "error: $changelog entry for $version still contains TODO" >&2
  exit 1
fi
