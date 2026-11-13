import type { AllowedRule } from '../../../shared/protocol/project.schema';

/** One thing a project's sessions do without asking, as its Config tab lists it. */
export interface AllowedRow {
  rule: AllowedRule;
  name: string;
}
