#!/bin/sh
set -eu

repo="${KAVE_REPO:-kave-io/kave}"
version="${KAVE_VERSION:-latest}"
bin_name="kave"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: required command not found: $1" >&2
    exit 1
  }
}

need curl
need tar

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  *) echo "error: unsupported OS: $os" >&2; exit 1 ;;
esac

machine="$(uname -m)"
case "$machine" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "error: unsupported architecture: $machine" >&2; exit 1 ;;
esac

if [ "$version" = "latest" ]; then
  release_path="latest/download"
else
  release_path="download/$version"
fi

base_url="https://github.com/$repo/releases/$release_path"
checksums_url="$base_url/checksums.txt"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

curl -fsSL "$checksums_url" -o "$tmp_dir/checksums.txt"

archive="$(awk -v os="$os" -v arch="$arch" '$2 ~ "^kave_[^ ]*_" os "_" arch "[.]tar[.]gz$" { print $2; exit }' "$tmp_dir/checksums.txt")"
if [ -z "$archive" ]; then
  echo "error: no CLI archive found for $os/$arch in $checksums_url" >&2
  exit 1
fi

curl -fsSL "$base_url/$archive" -o "$tmp_dir/$archive"

if command -v sha256sum >/dev/null 2>&1; then
  grep "  $archive\$" "$tmp_dir/checksums.txt" | (cd "$tmp_dir" && sha256sum -c -)
elif command -v shasum >/dev/null 2>&1; then
  grep "  $archive\$" "$tmp_dir/checksums.txt" | (cd "$tmp_dir" && shasum -a 256 -c -)
else
  echo "error: sha256sum or shasum is required to verify checksums" >&2
  exit 1
fi

tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"

if [ -w /usr/local/bin ]; then
  install_dir="/usr/local/bin"
else
  install_dir="${HOME:-}/.local/bin"
  if [ -z "$install_dir" ]; then
    echo "error: HOME is not set and /usr/local/bin is not writable" >&2
    exit 1
  fi
  mkdir -p "$install_dir"
fi

install -m 0755 "$tmp_dir/$bin_name" "$install_dir/$bin_name"
echo "installed $bin_name to $install_dir/$bin_name"
