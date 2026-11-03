import { css } from '@jadis/core';
import { JlIcon } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { completedSteps, planProgress, runningStep } from '../lib/plan';
import { ScPlanSteps } from '../molecules/sc-plan-steps';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-plan.css?inline';

import type { PlanStep } from '../../../shared/protocol/session.schema';

const HEAD_ICON_SIZE = 14;

/**
 * The agent's plan above the composer, as it stands: open while there is something left to do, one line once
 * every step is done and the agent stopped. A click opens or folds it; folded, the line names the step in progress.
 */
export class ScPlan extends ScElement {
  static readonly selector = 'sc-plan';

  readonly steps = this.useChange<PlanStep[]>([], () => this.render());
  readonly working = this.useChange(false, () => this.render());
  // What the person chose with a click, over the default, until the agent writes a new plan.
  private readonly _choice = this.useChange<boolean | undefined>(undefined, () => this.render());

  private readonly _refs = this.useRefs((ref) => ({
    current: ref('.current'),
    head: ref<HTMLButtonElement>('.head'),
    progress: ref('.progress'),
    steps: ref<ScPlanSteps>('sc-plan-steps'),
  }));

  templateHtml(): Node {
    return (
      <section class="card">
        <button class="head" type="button" aria-expanded="true">
          <JlIcon name="list-checks" size={HEAD_ICON_SIZE} />
          <span class="title">Plan</span>
          <span class="progress" />
          <span class="current" hidden />
          <JlIcon class="chevron" name="chevron-down" size={HEAD_ICON_SIZE} />
        </button>
        <ScPlanSteps />
      </section>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.on(this._refs.head, 'click', () => this._choice.set(!this.isOpen()));
  }

  /** A new plan from the agent shows as the agent wants it seen, whatever was chosen for the last one. */
  show(steps: PlanStep[]): void {
    if (steps.length !== this.steps.get().length) {
      this._choice.set(undefined);
    }
    this.steps.set(steps);
  }

  private isOpen(): boolean {
    const steps = this.steps.get();
    return this._choice.get() ?? (this.working.get() || completedSteps(steps) < steps.length);
  }

  private render(): void {
    const steps = this.steps.get();
    const open = this.isOpen();
    this.hidden = steps.length === 0;
    this._refs.progress.textContent = planProgress(steps);
    this.showCurrent(open ? undefined : runningStep(steps));
    this._refs.steps.steps.set(steps);
    this._refs.steps.live.set(this.working.get());
    this._refs.steps.hidden = !open;
    this._refs.head.setAttribute('aria-expanded', String(open));
    this.reflect('open', open);
  }

  private showCurrent(step: PlanStep | undefined): void {
    this._refs.current.textContent = step?.content ?? '';
    this._refs.current.hidden = step === undefined;
  }
}

ScPlan.register();
