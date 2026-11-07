import { Bus } from '@jadis/core';
import { toast } from '@syware/jlib';
import { z } from 'zod';

import { ENDPOINTS, PROJECT_PARAM } from '../../../shared/protocol/endpoints';
import { SwcodeBaseStatusSchema } from '../../../shared/protocol/skill.schema';
import { jsonOf, request } from './api';
import { readStored, writeStored } from './device-store';

import type { SwcodeBaseStatus } from '../../../shared/protocol/skill.schema';
import type { SwcodeBasePlace, SwcodeBaseRead } from '../types/swcode-base.types';

const UNREACHABLE = 'The daemon did not answer.';

/** What the daemon answered of swcode-base, and for which project, for the pages to show it. */
export const swcodeBaseBus = new Bus<{ changed: SwcodeBaseRead }>();

// The skill this device was asked about: its name, as the versions that offered a pack of skills stored it.
const ASKED_KEY = 'swcode.swcodeBaseAsked';
const SKILL_NAME = 'swcode-base';
const NEVER_ASKED = '';

const asked = { skill: readStored(ASKED_KEY, z.string(), NEVER_ASKED) };

/** Whether to ask this device whether to install swcode-base: it is nowhere in sight, and it was not asked yet. */
export const wantsSwcodeBase = (status: SwcodeBaseStatus): boolean =>
  !status.installed && asked.skill !== SKILL_NAME;

/** Says this device was asked, so it is not asked again. */
export const markSwcodeBaseAsked = (): void => {
  asked.skill = SKILL_NAME;
  writeStored(ASKED_KEY, asked.skill);
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
 * Where swcode-base is: with a project, whether that one or its host's home has it; without one, this computer's
 * home alone, which is what the startup offer asks about.
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
 * Has the daemon install swcode-base with npx skills in the place, on the project's host: its home, or the project
 * itself; this computer's home without a project. Where it is once it was: npx fetches it from GitHub, which takes a
 * few seconds.
 */
export const installSwcodeBase = (
  place: SwcodeBasePlace,
  projectId?: string
): Promise<SwcodeBaseStatus | undefined> =>
  ask(SWCODE_BASE_INSTALLS[place], SWCODE_BASE_ENDPOINTS[place], projectId);

/** Takes it out of the place with npx skills, as `installSwcodeBase` puts it in: where it is once it left. */
export const removeSwcodeBase = (
  place: SwcodeBasePlace,
  projectId?: string
): Promise<SwcodeBaseStatus | undefined> => ask('DELETE', SWCODE_BASE_ENDPOINTS[place], projectId);

/** Installs swcode-base in this computer's home, as the offer's action does: why it could not, in a toast. */
async function installInHome(): Promise<void> {
  const installed = await installSwcodeBase('home');
  const problem = installed === undefined ? UNREACHABLE : installed.problem;
  if (problem !== undefined) {
    toast({ heading: problem, tone: 'danger' });
  }
}

/**
 * Asks once whether to install swcode-base, on a device where this computer's home does not have it. Its action has
 * npx skills install it in the home; closing the toast asks again on another device only.
 */
export function offerSwcodeBase(status: SwcodeBaseStatus): void {
  if (!wantsSwcodeBase(status)) {
    return;
  }
  markSwcodeBaseAsked();
  toast({
    actionLabel: 'Install',
    duration: 0,
    heading: 'swcode has a skill of coding rules',
    icon: 'book-open',
    onAction: () => void installInHome(),
    text: 'swcode-base is not in your ~/.agents/skills. Install it there with npx skills, for every project?',
    tone: 'info',
  });
}
