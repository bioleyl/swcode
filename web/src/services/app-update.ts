import { registerPlugin } from '@capacitor/core';
import { z } from 'zod';

import type { AppUpdate } from '../types/app-update.types';

const UNKNOWN_ANSWER = 'The app answered something swcode does not know.';

/** The Android app's own plugin (mobile/android/app/src/main/java/com/bioleyl/swcode/UpdaterPlugin.java). */
const Updater = registerPlugin<{ check(): Promise<unknown> }>('Updater');

const ANSWER = z.object({ version: z.string() });

/** Asks the Android app to look for a newer release now: Android installs it when there is one. */
export async function checkAppUpdate(): Promise<AppUpdate> {
  try {
    const answer = ANSWER.safeParse(await Updater.check());
    if (!answer.success) {
      return { kind: 'failed', problem: UNKNOWN_ANSWER };
    }
    return answer.data.version === '' ? { kind: 'latest' } : { kind: 'newer', version: answer.data.version };
  } catch (failure) {
    return { kind: 'failed', problem: failure instanceof Error ? failure.message : String(failure) };
  }
}
