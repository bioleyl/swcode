import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { TailnetSchema } from '../../../shared/protocol/pairing.schema';
import { jsonOf, request } from './api';

import type { Tailnet } from '../../../shared/protocol/pairing.schema';

const tailnetOf = async (response: Response | undefined): Promise<Tailnet | undefined> => {
  const parsed = TailnetSchema.safeParse(await jsonOf(response));
  return parsed.success ? parsed.data : undefined;
};

/** Whether the daemon's tailnet can give swcode an HTTPS address, and the address; nothing when it does not answer. */
export const readTailnet = async (): Promise<Tailnet | undefined> =>
  tailnetOf(await request('GET', ENDPOINTS.tailnet));

/** Has Tailscale serve swcode at its tailnet address: where it stands then, with why when Tailscale refused. */
export const serveOnTailnet = async (): Promise<Tailnet | undefined> =>
  tailnetOf(await request('POST', ENDPOINTS.tailnet));
