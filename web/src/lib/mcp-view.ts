import type { Tone } from '@syware/jlib';
import type { McpServer, McpServerState } from '../../../shared/protocol/settings.schema';

/** How each state of a server shows beside its name: a new state does not compile until it is here. */
export const SERVER_STATES: Readonly<Record<McpServerState, { label: string; tone: Tone }>> = {
  failed: { label: 'Failed', tone: 'danger' },
  ready: { label: 'Ready', tone: 'ok' },
  starting: { label: 'Starting', tone: 'info' },
};

const ONE = 1;

const toolsLine = (count: number): string => (count === ONE ? '1 tool' : `${count} tools`);

/** What a server's row says under its name: its command or address, then its tools, or why it has none. */
export const serverLine = (server: McpServer): string => {
  if (server.state === 'failed') {
    return `${server.target} · ${server.problem ?? 'It did not start.'}`;
  }
  if (server.state === 'starting') {
    return server.target;
  }
  return `${server.target} · ${toolsLine(server.tools)}`;
};

/** Whether any server is still being reached: the list is then read again shortly. */
export const anyStarting = (servers: readonly McpServer[]): boolean =>
  servers.some((server) => server.state === 'starting');

/** A servers file to start from, as the page shows it where none is listed. */
export const EXAMPLE_FILE = `{
  "mcpServers": {
    "docs": { "url": "https://example.com/mcp" },
    "files": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/folder"] }
  }
}`;
