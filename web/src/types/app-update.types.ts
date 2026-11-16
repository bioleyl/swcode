/** What the Android app's own look left: a newer release Android is installing, this one, or why it could not look. */
export type AppUpdate =
  | { kind: 'newer'; version: string }
  | { kind: 'latest' }
  | { kind: 'failed'; problem: string };

export type AppUpdateMap = { [Update in AppUpdate as Update['kind']]: Update };
