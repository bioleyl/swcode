/** biome-ignore-all lint/style/useNamingConvention: GitHub's API answers in snake_case (tag_name, html_url) */
import { createHash } from 'node:crypto';
import { once } from 'node:events';
import { createServer } from 'node:http';

import type { FakeRelease, FakeReleases } from './types/fake-releases.types.js';

const HOST = '127.0.0.1';
// Go's names of the processors, as the release names its files: the daemon asks for the one built for it.
const GO_ARCHITECTURES: Readonly<Record<string, string>> = { arm64: 'arm64', x64: 'amd64' };
const PROGRAM_FILE = `swcode-${process.platform}-${GO_ARCHITECTURES[process.arch] ?? process.arch}`;
const PROGRAM_PATH = '/program';
const SUMS_PATH = '/SHA256SUMS';
const LATEST_PATH = '/latest';
const NOT_FOUND = 404;

/** What the release's program holds: the test reads it back from the daemon's once installed. */
export const programOf = (release: FakeRelease): string => `swcode ${release.version}`;

/**
 * GitHub's API for swcode's latest release, and the release's files: the program built for this machine, and its
 * checksum.
 */
export async function startFakeReleases(release: FakeRelease): Promise<FakeReleases> {
  const program = programOf(release);
  const sum = createHash('sha256').update(program).digest('hex');
  // Known once the server listens, before any request.
  const served = { base: '' };
  const server = createServer((request, response) => {
    const base = served.base;
    const files: Readonly<Record<string, () => string>> = {
      [LATEST_PATH]: () =>
        JSON.stringify({
          assets: [
            { browser_download_url: `${base}${PROGRAM_PATH}`, name: PROGRAM_FILE },
            { browser_download_url: `${base}${SUMS_PATH}`, name: 'SHA256SUMS' },
          ],
          body: release.notes,
          html_url: `https://github.com/bioleyl/swcode/releases/tag/v${release.version}`,
          tag_name: `v${release.version}`,
        }),
      [PROGRAM_PATH]: () => program,
      [SUMS_PATH]: () => `${sum}  ${PROGRAM_FILE}\n`,
    };
    const file = files[request.url ?? ''];
    if (!file) {
      response.writeHead(NOT_FOUND).end();
      return;
    }
    response.end(file());
  });
  server.listen(0, HOST);
  await once(server, 'listening');
  const address = server.address();
  served.base = `http://${HOST}:${typeof address === 'object' && address !== null ? address.port : 0}`;
  return {
    stop: async () => {
      server.closeAllConnections();
      server.close();
      await once(server, 'close');
    },
    url: `${served.base}${LATEST_PATH}`,
  };
}
