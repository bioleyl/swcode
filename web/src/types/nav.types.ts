import type { Project } from '../../../shared/protocol/project.schema';
import type { SessionSummary } from '../../../shared/protocol/session.schema';

/** What the navigation lists from: the projects, the sessions, and when the person last saw each session. */
export interface NavLists {
  projects: readonly Project[];
  sessions: readonly SessionSummary[];
  seen: Readonly<Record<string, string>>;
}
