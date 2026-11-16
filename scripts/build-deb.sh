#!/usr/bin/env bash
# Builds the Debian packages from the Linux programs npm run build:binary makes: dist/swcode_<version>_<arch>.deb,
# each holding /usr/bin/swcode. Settings, sessions and paired phones live in ~/.swcode, which the package leaves
# alone; `swcode service install` puts the daemon in the person's own systemd, started as they log in.
set -eu

VERSION="$(node -p "require('./package.json').version")"
HOMEPAGE="https://github.com/bioleyl/swcode"
MAINTAINER="SyWare <bioleyl@users.noreply.github.com>"

for binary in dist/swcode-linux-*; do
  architecture="${binary##*-}"
  root="dist/deb-${architecture}"
  rm -rf "${root}"
  install -D -m 0755 "${binary}" "${root}/usr/bin/swcode"
  install -D -m 0644 LICENSE "${root}/usr/share/doc/swcode/copyright"
  mkdir -p "${root}/DEBIAN"
  cat > "${root}/DEBIAN/control" <<EOF
Package: swcode
Version: ${VERSION}
Section: devel
Priority: optional
Architecture: ${architecture}
Installed-Size: $((($(stat -c %s "${binary}") + 1023) / 1024))
Depends: ca-certificates
Maintainer: ${MAINTAINER}
Homepage: ${HOMEPAGE}
Description: a coding agent for local models on llm-server
 swcode runs a coding agent against a local llm-server: a daemon that
 runs the agent and its tools, and a web app to follow and steer it from
 a computer or a phone.
EOF
  dpkg-deb --build --root-owner-group "${root}" "dist/swcode_${VERSION}_${architecture}.deb"
  rm -rf "${root}"
done
