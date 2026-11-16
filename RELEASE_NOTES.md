# swcode 0.13.0

Released 2026-11-16.

swcode 0.13 pairs by a number instead of a one-minute code, and comes with an Android app that installs its own next release.

## Highlights

- **Pairing by number**: the phone — or another computer — shows six digits. Run `swcode pair 123456` on the computer and it is paired. The number waits five minutes, serves once, and cannot be guessed (sixty tries a minute from one client, then a pause). Settings → Phone still shows a QR code, which now carries the address to open rather than a code.
- **An Android app**: `swcode-android.apk` in this release. It brings the same pages, pairs with any number of computers, and at every launch looks for a newer release, checks its checksum and hands it to Android's installer — Android asks once to allow installing apps from swcode. It is signed with swcode's own keystore, which later releases must keep.
- **Adding another computer** now asks for its name, address and port, and gives a number to type on that computer: the pairing code and its link are gone.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Upgrading

The command changed: `swcode pair` now takes the number an app shows (`swcode pair 123456`). The old QR-code pairing no longer exists, so each phone pairs once more with the new flow. Existing daemons offer 0.13.0 themselves, and the Windows app updates in place.

Install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform, the Android app included. Checksums are published with the release.
