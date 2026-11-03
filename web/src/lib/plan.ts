import type { PlanStep } from '../../../shared/protocol/session.schema';

export { currentPlan } from '../../../shared/plan/plan-state';

export const completedSteps = (steps: readonly PlanStep[]): number =>
  steps.filter((step) => step.status === 'completed').length;

/** How far the plan is, in a few words. */
export const planProgress = (steps: readonly PlanStep[]): string =>
  `${completedSteps(steps)} of ${steps.length} done`;

/** The step the agent marked as the one it is on, if any. */
export const runningStep = (steps: readonly PlanStep[]): PlanStep | undefined =>
  steps.find((step) => step.status === 'in_progress');
