import type { PlanStep, SessionEvent } from '../protocol/session.schema.js';

/** The events of the session's last turn, from its start; a subagent's own conversation has no turns: all of it. */
export const lastTurn = (events: readonly SessionEvent[]): readonly SessionEvent[] =>
  events.slice(
    Math.max(
      events.findLastIndex((event) => event.kind === 'turnStarted'),
      0
    )
  );

const planOf = (event: SessionEvent | undefined): PlanStep[] =>
  event?.kind === 'toolFinished' && event.display?.kind === 'plan' ? [...event.display.steps] : [];

/** The plan of the session's last turn, as the agent's latest plan call left it: none until it writes one. */
export const currentPlan = (events: readonly SessionEvent[]): PlanStep[] =>
  planOf(lastTurn(events).findLast((event) => planOf(event).length > 0));

/** The steps not completed yet, pending or in progress. */
export const unfinishedSteps = (steps: readonly PlanStep[]): PlanStep[] =>
  steps.filter((step) => step.status !== 'completed');
