import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { UpdateInstalledSchema, UpdateViewSchema } from '../../../shared/protocol/update.schema';
import { isPhone } from '../lib/devices';
import { jsonOf, request } from './api';
import { daemonBus } from './daemon';

import type { UpdateInstalled, UpdateView } from '../../../shared/protocol/update.schema';

const UNREACHABLE = 'The daemon did not answer.';

const viewOf = async (response: Response | undefined): Promise<UpdateView | undefined> => {
  const parsed = UpdateViewSchema.safeParse(await jsonOf(response));
  return response?.ok && parsed.success ? parsed.data : undefined;
};

/** The version that runs and what is known of a newer one; nothing when the daemon does not answer. */
export const loadUpdate = async (): Promise<UpdateView | undefined> =>
  viewOf(await request('GET', ENDPOINTS.update));

/** Turns looking every 10 minutes on or off: what swcode does then, nothing when it did not apply. */
export const setChecks = async (checks: boolean): Promise<UpdateView | undefined> =>
  viewOf(await request('PUT', ENDPOINTS.update, { checks }));

/** Looks for a newer version now, and gives what swcode found. */
export const checkNow = async (): Promise<UpdateView | undefined> =>
  viewOf(await request('POST', ENDPOINTS.updateCheck));

/** Whether updates are offered and installed here: on a computer, not on a phone. */
export const installsUpdates = (): boolean => !isPhone(navigator.userAgent);

/** Loads the page again once the daemon came back from going away, until the signal aborts. */
function reloadWhenBack(signal: AbortSignal): void {
  const seen = { away: false };
  daemonBus.register(
    'online',
    (online) => {
      seen.away ||= !online;
      if (online && seen.away) {
        location.reload();
      }
    },
    signal
  );
}

/**
 * Installs the newer version found. When swcode restarts on it by itself, the page loads again once it is back: the
 * app it shows comes with the new version. Heard from before asking, the daemon closing as soon as it answered.
 */
export async function installUpdate(): Promise<UpdateInstalled> {
  const waiting = new AbortController();
  reloadWhenBack(waiting.signal);
  const parsed = UpdateInstalledSchema.safeParse(await jsonOf(await request('POST', ENDPOINTS.updateInstall)));
  const installed: UpdateInstalled = parsed.success ? parsed.data : { ok: false, problem: UNREACHABLE };
  if (!installed.ok || installed.restart === 'person') {
    waiting.abort();
  }
  return installed;
}
