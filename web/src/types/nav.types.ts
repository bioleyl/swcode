import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';

/** What the navigation lists from: the projects, the sessions, and when the person last saw each session. */
export interface NavLists {
  projects: readonly Project[];
  sessions: readonly SessionSummary[];
  seen: Readonly<Record<string, string>>;
}

/** The + of a project's row in the navigation: the new session it opens, and the project's name, which it says. */
export interface ProjectStart {
  href: string;
  name: string;
}
