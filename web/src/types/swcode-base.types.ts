import type { SwcodeBaseStatus } from '../../../shared/protocol/skill.schema';

/** Where swcode-base can be: the person's ~/.agents/skills, which every project reads, or a project's own. */
export type SwcodeBasePlace = 'home' | 'project';

/** What the daemon answered, and the project it answered for: none when it answered for the home alone. */
export interface SwcodeBaseRead {
  projectId: string | undefined;
  status: SwcodeBaseStatus;
}
