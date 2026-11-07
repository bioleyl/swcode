import type { SessionSummary } from '../../../shared/protocol/session.schema';

/** What tells whether a session is at work: its turn's state, and its subagents that have not reported. */
export type SessionActivity = Pick<SessionSummary, 'status' | 'subagents'>;
