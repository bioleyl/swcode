import { Bus } from '@jadis/core';
import { toast } from '@syware/jlib';
import { z } from 'zod';

import { ENDPOINTS, PROJECT_PARAM } from '../../../shared/protocol/endpoints';
import { SwcodeBaseStatusSchema } from '../../../shared/protocol/skill.schema';
import { jsonOf, request } from './api';
import { readStored, writeStored } from './device-store';

import type { SwcodeBaseStatus } from '../../../shared/protocol/skill.schema';
import type { SwcodeBasePlace, SwcodeBaseRead } from '../types/swcode-base.types';

/** What the daemon answered of swcode-base, and for which project, for the pages to show it. */
export const swcodeBaseBus = new Bus<{ changed: SwcodeBaseRead }>();

// The pack this device was last asked about, by its names, so a new pack asks again.
const ASKED_KEY = 'swcode.swcodeBaseAsked';
const NEVER_ASKED = '';

const asked = { pack: readStored(ASKED_KEY, z.string(), NEVER_ASKED) };

/**
 * Whether to ask this machine whether to install the pack: carried, nowhere in sight, and not asked for this pack
 * yet. The pack's names are the key: a new pack asks again.
 */
export const wantsSwcodeBase = (status: SwcodeBaseStatus): boolean =>
  status.carried && !status.installed && asked.pack !== status.names.join(',');

/** Says this device was asked, so it is not asked again for this pack. */
export const markSwcodeBaseAsked = (status: SwcodeBaseStatus): void => {
  asked.pack = status.names.join(',');
  writeStored(ASKED_KEY, asked.pack);
};

/**
 * Asks the daemon, and tells the bus what it answered and for which project, unless it answered a problem; nothing
 * when it did not answer.
 */
async function ask(method: string, path: string, projectId?: string): Promise<SwcodeBaseStatus | undefined> {
  const query = projectId === undefined ? '' : `?${new URLSearchParams({ [PROJECT_PARAM]: projectId })}`;
  const parsed = SwcodeBaseStatusSchema.safeParse(await jsonOf(await request(method, `${path}${query}`)));
  if (!parsed.success) {
    return undefined;
  }
  if (parsed.data.problem === undefined) {
    swcodeBaseBus.emit('changed', { projectId, status: parsed.data });
  }
  return parsed.data;
}

/**
 * Whether this swcode carries the swcode-base skills, and where they are: with a project, whether that one has
 * them; without one, the home alone, which is what the startup offer asks about.
 */
export const readSwcodeBase = (projectId?: string): Promise<SwcodeBaseStatus | undefined> =>
  ask('GET', ENDPOINTS.swcodeBase, projectId);

// Each place's address, and how it is written there.
const SWCODE_BASE_ENDPOINTS: Readonly<Record<SwcodeBasePlace, string>> = {
  home: ENDPOINTS.swcodeBaseHome,
  project: ENDPOINTS.swcodeBase,
};
const SWCODE_BASE_INSTALLS: Readonly<Record<SwcodeBasePlace, string>> = { home: 'PUT', project: 'POST' };

/**
 * Writes them in the place, on the project's host: its home, or the project itself; this computer's home without a
 * project. What is there once they were.
 */
export const installSwcodeBase = (
  place: SwcodeBasePlace,
  projectId?: string
): Promise<SwcodeBaseStatus | undefined> =>
  ask(SWCODE_BASE_INSTALLS[place], SWCODE_BASE_ENDPOINTS[place], projectId);

/** Takes them out of the place, as `installSwcodeBase` puts them in: what is there once they left. */
export const removeSwcodeBase = (
  place: SwcodeBasePlace,
  projectId?: string
): Promise<SwcodeBaseStatus | undefined> => ask('DELETE', SWCODE_BASE_ENDPOINTS[place], projectId);

/**
 * Asks once, for each pack, whether to install it: on a machine where neither the home nor any project has a
 * skill. Its action installs the skills in the home; closing the toast asks again on another device, or after the
 * pack changes.
 */
export function offerSwcodeBase(status: SwcodeBaseStatus): void {
  if (!wantsSwcodeBase(status)) {
    return;
  }
  markSwcodeBaseAsked(status);
  toast({
    actionLabel: 'Install',
    duration: 0,
    heading: 'swcode comes with skills',
    icon: 'book-open',
    onAction: () => void installSwcodeBase('home'),
    text: 'No skill was found in ~/.agents/skills nor in a project. Install the swcode-base skills in your home?',
    tone: 'info',
  });
}
