import { once } from 'node:events';
import { createServer } from 'node:http';

import type { IncomingMessage } from 'node:http';
import type { FakeMcp } from './types/fake-mcp.types.js';

const HOST = '127.0.0.1';
const ACCEPTED = 202;
const PROTOCOL_VERSION = '2025-06-18';

interface Asked {
  id?: number | string;
  method?: string;
  params?: { arguments?: Record<string, unknown> };
}

const bodyOf = async (request: IncomingMessage): Promise<Asked> => {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(chunk as Buffer);
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8')) as Asked;
};

/** What the server answers each method: the handshake, its one tool, and a call to it. */
const RESULTS: Readonly<Record<string, (asked: Asked) => unknown>> = {
  initialize: () => ({ capabilities: {}, protocolVersion: PROTOCOL_VERSION }),
  'tools/call': (asked) => ({
    content: [{ text: `Hello, ${String(asked.params?.arguments?.name)}.`, type: 'text' }],
  }),
  'tools/list': () => ({
    tools: [
      {
        description: 'Says hello to someone.',
        inputSchema: { properties: { name: { type: 'string' } }, required: ['name'], type: 'object' },
        name: 'greet',
      },
    ],
  }),
};

/** An MCP server over HTTP, of the kind that shakes hands first: JSON answers, no stream. */
export async function startFakeMcp(): Promise<FakeMcp> {
  const server = createServer((request, response) => {
    void bodyOf(request).then((asked) => {
      // A notification is taken and gets no answer.
      if (asked.id === undefined) {
        response.writeHead(ACCEPTED).end();
        return;
      }
      const result = RESULTS[asked.method ?? '']?.(asked) ?? {};
      response.setHeader('content-type', 'application/json');
      response.end(JSON.stringify({ id: asked.id, jsonrpc: '2.0', result }));
    });
  });
  server.listen(0, HOST);
  await once(server, 'listening');
  const address = server.address();
  return {
    stop: async () => {
      server.closeAllConnections();
      server.close();
      await once(server, 'close');
    },
    url: `http://${HOST}:${typeof address === 'object' && address !== null ? address.port : 0}`,
  };
}
