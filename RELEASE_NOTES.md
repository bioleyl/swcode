# swcode 0.12.1

Released 2026-11-16.

swcode 0.12.1 lets a preset say its model sees pictures, for a server that does not report it — a hosted API such as DeepSeek's — and lets `screenshot` show a page in the conversation without a question or a model that sees.

## Highlights

- **A preset can say "Sees pictures"**: a hosted API often does not list a model's vision, so its pictures were turned into a cannot-see note. Turn the switch on in the preset's dialog and that preset sends its model the pictures. Off, what the llm-server says decides.
- **`screenshot` without a question**: it captures the page and shows it in the conversation without asking a model or needing vision; with `see` and no vision, the picture is shown but kept out of the model's context.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Upgrading

Nothing changes for an existing install. Turn on "Sees pictures" in a preset whose server does not report it.

Install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release. swcode 0.12 offers this release itself.
