import type { SwcodeBaseStatus } from '../../../shared/protocol/skill.schema';
import type { SwcodeBasePlace } from '../types/swcode-base.types';

const PLACED: Readonly<Record<SwcodeBasePlace, (status: SwcodeBaseStatus) => boolean>> = {
  home: (status) => status.inHome,
  project: (status) => status.inProject,
};

/** Whether swcode-base is in that place, as the status has it. */
export const isSwcodeBaseIn = (status: SwcodeBaseStatus, place: SwcodeBasePlace): boolean => PLACED[place](status);
