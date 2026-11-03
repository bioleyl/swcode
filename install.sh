#!/bin/sh
# swcode's installer for Linux, macOS and WSL: downloads the program built for this machine from the latest release,
# checks it against the release's checksums, and has it install itself as ~/.local/bin/swcode (swcode install), with
# its settings and, in WSL, a shortcut in Windows.
#
#   curl -fsSL https://raw.githubusercontent.com/bioleyl/swcode/main/install.sh | sh
#
# SWCODE_RELEASES downloads from elsewhere: a release of your own, or a folder served for a test.
set -eu

releases="${SWCODE_RELEASES:-https://github.com/bioleyl/swcode/releases/latest/download}"
checksums="SHA256SUMS"

case "$(uname -s)" in
  Linux) system=linux ;;
  Darwin) system=darwin ;;
  *)
    echo "swcode is not built for $(uname -s). On Windows, install the desktop app with install.ps1." >&2
    exit 1
    ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) architecture=amd64 ;;
  aarch64 | arm64) architecture=arm64 ;;
  *)
    echo "swcode is not built for $(uname -m) processors." >&2
    exit 1
    ;;
esac

# fetch saves a file of the release.
fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$releases/$1"
  else
    wget -q -O "$2" "$releases/$1"
  fi
}

# sha256 is a file's checksum, by whichever tool the system has.
sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  else
    shasum -a 256 "$1" | cut -d ' ' -f 1
  fi
}

program="swcode-$system-$architecture"
folder="$(mktemp -d)"
trap 'rm -rf "$folder"' EXIT

echo "Downloading $program..."
fetch "$program" "$folder/$program"
fetch "$checksums" "$folder/$checksums"
expected="$(grep " $program\$" "$folder/$checksums" | cut -d ' ' -f 1)"
if [ -z "$expected" ] || [ "$(sha256 "$folder/$program")" != "$expected" ]; then
  echo "The download does not match the release's checksum: try again." >&2
  exit 1
fi
chmod +x "$folder/$program"
"$folder/$program" install
