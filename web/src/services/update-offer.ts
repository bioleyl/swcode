import { toast } from '@syware/jlib';
import { z } from 'zod';

import { INSTALLED_TEXTS } from '../lib/update-view';
import { readStored, writeStored } from './device-store';
import { installsUpdates, installUpdate } from './updates';

import type { DaemonState } from '../types/daemon.types';

// The last version this device was asked about.
const ASKED_KEY = 'swcode.updateAsked';
const NONE_ASKED = '';
const OFFER = {
  failed: 'swcode was not updated',
  heading: (version: string) => `swcode ${version} is out`,
  install: 'Install',
  installing: (version: string) => `Installing swcode ${version}…`,
  text: 'Install it now? swcode restarts on it in a few seconds. Settings › Updates says what it brings.',
} as const;

const asked = { version: readStored(ASKED_KEY, z.string(), NONE_ASKED) };

/** Installs the version offered, its progress in a toast: done, swcode restarts and the page loads again. */
async function install(version: string): Promise<void> {
  const progress = toast({
    closable: false,
    duration: 0,
    heading: OFFER.installing(version),
    progress: null,
    working: true,
  });
  const installed = await installUpdate();
  progress.update(
    installed.ok
      ? {
          closable: true,
          heading: INSTALLED_TEXTS[installed.restart](installed.version),
          tone: 'ok',
          working: false,
        }
      : { closable: true, heading: OFFER.failed, text: installed.problem, tone: 'danger', working: false }
  );
}

/**
 * Asks once on this computer, for each newer version the daemon finds, whether to install it: a toast that stays
 * until it is answered or closed. Settings › Updates offers it after that; a phone is never asked.
 */
export function offerUpdate(state: DaemonState): void {
  const version = state.kind === 'ready' ? state.status.update : undefined;
  if (version === undefined || version === asked.version || !installsUpdates()) {
    return;
  }
  asked.version = version;
  writeStored(ASKED_KEY, version);
  toast({
    actionLabel: OFFER.install,
    duration: 0,
    heading: OFFER.heading(version),
    icon: 'circle-arrow-up',
    onAction: () => void install(version),
    text: OFFER.text,
    tone: 'info',
  });
}
