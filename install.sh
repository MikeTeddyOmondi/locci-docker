#!/bin/sh
# Install the locci binary from a GitHub release of locci-docker.
#
#   curl -fsSL https://raw.githubusercontent.com/MikeTeddyOmondi/locci-docker/main/install.sh -o install.sh
#   less install.sh        # read it first
#   sh install.sh
#
# Settings (environment variables):
#   LOCCI_VERSION       release tag to install, e.g. v0.1.0 (default: the latest release)
#   LOCCI_INSTALL_DIR   where to put locci (default: /usr/local/bin, using sudo if needed)
#
# What it does:
#   1. picks the archive for this OS and CPU (Linux or macOS, amd64 or arm64);
#   2. downloads it with SHA256SUMS, SHA256SUMS.asc and release-key.asc;
#   3. if gpg is installed, checks the key's fingerprint against the one below
#      and verifies the signature on SHA256SUMS (otherwise says it skipped this);
#   4. checks the archive's SHA-256 against SHA256SUMS;
#   5. copies locci into LOCCI_INSTALL_DIR.
# Nothing else is changed. Windows: download the .zip from the releases page.

set -eu

REPO=MikeTeddyOmondi/locci-docker
# The release signing key (also in docs/RELEASING.md).
FINGERPRINT=2B9D1A57F9B77E2EEDFF0F05B5E00DB4AAE715C3

say() { printf '%s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

# Download $1 to the file $2.
fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$2" "$1"
  else
    die "needs curl or wget"
  fi
}

# The tag the releases/latest link redirects to.
latest() {
  url=https://github.com/$REPO/releases/latest
  if command -v curl >/dev/null 2>&1; then
    final=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$url")
  else
    final=$(wget -q -S --spider "$url" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1 | tr -d '\r')
  fi
  tag=${final##*/}
  case $tag in v*) printf '%s\n' "$tag" ;; *) die "couldn't find the latest release; set LOCCI_VERSION" ;; esac
}

case $(uname -s) in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS $(uname -s); download a release from https://github.com/$REPO/releases" ;;
esac
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "unsupported CPU $(uname -m)" ;;
esac

version=${LOCCI_VERSION:-$(latest)}
dir=${LOCCI_INSTALL_DIR:-/usr/local/bin}
name=locci-docker-$version-$os-$arch
base=https://github.com/$REPO/releases/download/$version

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"

say "Downloading $name.tar.gz"
fetch "$base/$name.tar.gz" "$name.tar.gz" || die "no $name.tar.gz in release $version"
fetch "$base/SHA256SUMS" SHA256SUMS
fetch "$base/SHA256SUMS.asc" SHA256SUMS.asc
fetch "$base/release-key.asc" release-key.asc

if command -v gpg >/dev/null 2>&1; then
  mkdir -m 700 gnupg
  gpg --homedir gnupg --batch --quiet --import release-key.asc 2>/dev/null
  got=$(gpg --homedir gnupg --batch --with-colons --fingerprint | awk -F: '/^fpr:/ { print $10; exit }')
  [ "$got" = "$FINGERPRINT" ] || die "release-key.asc has fingerprint $got, expected $FINGERPRINT"
  gpg --homedir gnupg --batch --quiet --verify SHA256SUMS.asc SHA256SUMS 2>/dev/null ||
    die "bad signature on SHA256SUMS"
  say "Signature on SHA256SUMS: good (key $FINGERPRINT)"
else
  say "gpg not found: skipping the signature check (checksum only)"
fi

want=$(awk -v f="$name.tar.gz" '$2 == f { print $1 }' SHA256SUMS)
[ -n "$want" ] || die "$name.tar.gz is not in SHA256SUMS"
if command -v sha256sum >/dev/null 2>&1; then
  have=$(sha256sum "$name.tar.gz" | awk '{ print $1 }')
else
  have=$(shasum -a 256 "$name.tar.gz" | awk '{ print $1 }')
fi
[ "$have" = "$want" ] || die "checksum mismatch for $name.tar.gz"
say "Checksum: good"

tar xzf "$name.tar.gz"
if [ -d "$dir" ] && [ -w "$dir" ]; then
  install -m 755 "$name/locci" "$dir/locci"
elif command -v sudo >/dev/null 2>&1; then
  say "Installing to $dir with sudo"
  sudo mkdir -p "$dir"
  sudo install -m 755 "$name/locci" "$dir/locci"
else
  die "can't write to $dir; set LOCCI_INSTALL_DIR to a directory you own"
fi

say "Installed $("$dir/locci" --version) to $dir/locci"
case :$PATH: in *:"$dir":*) ;; *) say "Note: $dir is not on your PATH" ;; esac
