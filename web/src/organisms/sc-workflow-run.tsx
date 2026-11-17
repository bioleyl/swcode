import { createElement, css } from '@jadis/core';
import { JlBadge, JlButton, JlCard, JlFlow, JlListItem } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { flowOf, RUN_STATUS, STEP_STATUS } from '../lib/workflow-view';
import { hrefOf, sessionPath } from '../services/navigation';
import { isRunning, runOf, stopWorkflowRun, workflowOf, workflowsBus } from '../services/workflows';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-workflow-run.css?inline';

import type { WorkflowRun } from '../../../shared/protocol/workflow.schema';

const FLOW_LABEL = 'The steps of this run';

/**
 * A workflow run, on its run session's page: the graph of its steps as far as it went, the session of each step, and
 * the button that stops it. A step's session shows the run itself, and each step links to its session.
 */
export class ScWorkflowRun extends ScElement {
  static readonly selector = 'sc-workflow-run';

  readonly runId = this.useChange('', () => this.render());

  private readonly _refs = this.useRefs((ref) => ({
    card: ref(JlCard),
    flow: ref(JlFlow),
    problem: ref('.problem'),
    status: ref('.run-status'),
    steps: ref('.steps'),
    stop: ref<JlButton>('.stop'),
  }));

  templateHtml(): Node {
    return (
      <JlCard class="run" heading="Workflow" hidden>
        <div class="body">
          <p class="brief" />
          <p class="run-status" />
          <div class="flow">
            <JlFlow readonly label={FLOW_LABEL} />
          </div>
          <div class="steps" />
          <p class="problem" hidden />
        </div>
        <JlButton slot="actions" class="stop" icon="stop" variant="ghost" label="Stop" hidden />
      </JlCard>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.onBus(workflowsBus, 'changed', () => this.render());
    this._refs.stop.events.register('press', () => {
      const run = runOf(this.runId.get());
      if (run) {
        stopWorkflowRun(run.id);
      }
    });
    this.render();
  }

  private render(): void {
    const run = runOf(this.runId.get());
    if (!run || !this.isConnected) {
      this.hidden = true;
      this._refs.card.hidden = true;
      return;
    }
    this.hidden = false;
    this._refs.card.hidden = false;
    this._refs.card.heading.set(`Workflow · ${run.workflowName}`);
    const status = this.descriptionOf(run);
    this._refs.card.description.set(status);
    this._refs.status.textContent = status;
    this._refs.problem.textContent = run.problem ?? '';
    this._refs.problem.hidden = run.problem === undefined;
    this._refs.stop.hidden = !isRunning(run.status);
    this.drawFlow(run);
    this.drawSteps(run);
  }

  private descriptionOf(run: WorkflowRun): string {
    const look = RUN_STATUS[run.status];
    const steps = run.steps.filter((step) => step.status === 'done').length;
    return `${look.label} · ${steps} of ${run.steps.length} steps done`;
  }

  private drawFlow(run: WorkflowRun): void {
    const workflow = workflowOf(run.workflowId);
    if (!workflow) {
      return;
    }
    this._refs.flow.graph.set(flowOf(workflow.steps, workflow.edges, run.steps));
    requestAnimationFrame(() => this._refs.flow.fit());
  }

  /** A step, its state, and the session it ran in: a row is a link to that session. */
  private drawSteps(run: WorkflowRun): void {
    this._refs.steps.replaceChildren(...run.steps.map((step) => this.stepNode(step)));
  }

  private stepNode(step: WorkflowRun['steps'][number]): Node {
    const look = STEP_STATUS[step.status];
    const row = createElement(JlListItem, {
      props: {
        description: look.label,
        heading: step.name,
        icon: 'user',
        ...(step.sessionId === undefined ? {} : { href: hrefOf(sessionPath(step.sessionId)) }),
      },
    });
    row.append(createElement(JlBadge, { attrs: { slot: 'end' }, props: { label: look.label, tone: look.tone } }));
    return row;
  }
}

ScWorkflowRun.register();
