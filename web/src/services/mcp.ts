import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { McpStatusSchema } from '../../../shared/protocol/settings.schema';
import { jsonOf, request } from './api';

import type { McpStatus } from '../../../shared/protocol/settings.schema';

const statusOf = async (response: Response | undefined): Promise<McpStatus | undefined> => {
  const parsed = McpStatusSchema.safeParse(await jsonOf(response));
  return parsed.success ? parsed.data : undefined;
};

/** The MCP servers the daemon reaches for its agents and where each stands; nothing when it does not answer. */
export const readServers = async (): Promise<McpStatus | undefined> =>
  statusOf(await request('GET', ENDPOINTS.mcp));

/** Has the daemon read its servers file again: the servers it lists now, those new or changed being reached. */
export const reloadServers = async (): Promise<McpStatus | undefined> =>
  statusOf(await request('POST', ENDPOINTS.mcp));
