import { ClaudeSourcesSchema, ClaudeViewSchema } from '../../../shared/protocol/claude.schema';
import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { jsonOf, request } from './api';

import type { ClaudeSources, ClaudeView } from '../../../shared/protocol/claude.schema';

const HOST_PARAM = 'host';

/** What the daemon finds of Claude Code on a machine, by its host key; nothing when the daemon does not answer. */
export async function loadClaude(hostKey: string): Promise<ClaudeView | undefined> {
  const query = new URLSearchParams({ [HOST_PARAM]: hostKey });
  const parsed = ClaudeViewSchema.safeParse(await jsonOf(await request('GET', `${ENDPOINTS.claude}?${query}`)));
  return parsed.success ? parsed.data : undefined;
}

/** Saves which of Claude Code's skills swcode reads: the sessions read them so from their next turn. */
export async function saveClaudeSources(sources: ClaudeSources): Promise<ClaudeSources | undefined> {
  const parsed = ClaudeSourcesSchema.safeParse(await jsonOf(await request('PUT', ENDPOINTS.claude, sources)));
  return parsed.success ? parsed.data : undefined;
}
