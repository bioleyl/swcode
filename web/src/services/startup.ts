import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { StartupViewSchema } from '../../../shared/protocol/startup.schema';
import { jsonOf, request } from './api';

import type { StartupMode, StartupView } from '../../../shared/protocol/startup.schema';

/** Whether swcode starts with its computer, and how; nothing when the daemon does not answer. */
export async function loadStartup(): Promise<StartupView | undefined> {
  const parsed = StartupViewSchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.startup)));
  return parsed.success ? parsed.data : undefined;
}

/** How it should start from now on: what it does after the change, nothing when it did not apply. */
export async function saveStartup(mode: StartupMode): Promise<StartupView | undefined> {
  const response = await request('PUT', ENDPOINTS.startup, { mode });
  const parsed = StartupViewSchema.safeParse(await jsonOf(response));
  return response?.ok && parsed.success ? parsed.data : undefined;
}
