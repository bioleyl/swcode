import { css } from '@jadis/core';
import { JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-plan-steps.css?inline';

import type { PlanStep, PlanStepStatus } from '../../../shared/protocol/session.schema';

const STEP_ICON_SIZE = 14;

const STATUS_ICONS: Readonly<Record<PlanStepStatus, string>> = {
  completed: 'check',
  // biome-ignore lint/style/useNamingConvention: the status of Claude Code's task list, which the models know
  in_progress: 'loader',
  pending: 'circle',
};

const stepNode = (step: PlanStep): Node => (
  <li data-status={step.status}>
    <JlIcon name={STATUS_ICONS[step.status]} size={STEP_ICON_SIZE} />
    <span class="content">{step.content}</span>
  </li>
);

/** The steps of the agent's plan, each with where it stands; the step in progress turns while the agent works. */
export class ScPlanSteps extends ScElement {
  static readonly selector = 'sc-plan-steps';

  readonly steps = this.useChange<PlanStep[]>([], (steps) =>
    this._refs.list.replaceChildren(...steps.map(stepNode))
  );
  readonly live = this.useChange(false, (live) => this.reflect('live', live), { attribute: 'live' });

  private readonly _refs = this.useRefs((ref) => ({ list: ref('ol') }));

  templateHtml(): Node {
    return <ol />;
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }
}

ScPlanSteps.register();
