# swcode 0.13.1

Released 2026-11-16.

swcode 0.13.1 fixes the pairing number: it stayed on screen, but a new one was drawn every fifteen seconds, so the number read on the phone was never the one typed on the computer.

## Highlights

- **A pairing number that holds**: the page is told of every status answer, and drew a fresh number each time. It now keeps the number until the person pairs, or asks for another.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Upgrading

Nothing to do: `swcode pair` is unchanged. A phone that could not pair with 0.13.0 now can; daemons offer 0.13.1 themselves, and the Android app updates from this release.
