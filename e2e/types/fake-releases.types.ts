/** swcode's latest release, as the fake GitHub serves it: its version and what it brings. */
export interface FakeRelease {
  version: string;
  notes: string;
}

export interface FakeReleases {
  /** The latest release's address in the fake GitHub's API (SWCODE_RELEASES_URL). */
  url: string;
  stop(): Promise<void>;
}
