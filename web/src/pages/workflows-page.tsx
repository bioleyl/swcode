import { createElement, css } from '@jadis/core';
import {
  confirmDialog,
  JlAlert,
  JlBadge,
  JlButton,
  JlCard,
  JlEmpty,
  JlField,
  JlFlow,
  JlInput,
  JlListItem,
  JlPage,
  JlSelect,
  JlTextarea,
} from '@syware/jlib';

import {
  MAX_WORKFLOW_DESCRIPTION,
  MAX_WORKFLOW_NAME,
  MAX_WORKFLOW_PROMPT,
  MAX_WORKFLOW_ROLE,
  MAX_WORKFLOW_STEP_NAME,
} from '../../../shared/protocol/workflow.schema';
import { ScElement } from '../base/sc-element';
import { draftOf, emptyWorkflow, flowOf, graphProblem, RUN_STATUS } from '../lib/workflow-view';
import { daemonBus } from '../services/daemon';
import { hrefOf, NEW_WORKFLOW, navigate, sessionPath, WORKFLOWS_PATH, workflowPath } from '../services/navigation';
import { presetChoices, presetsBus } from '../services/presets';
import { allProjects } from '../services/project-list';
import {
  confirmWorkflow,
  isRunning,
  removeWorkflow,
  saveWorkflow,
  stopWorkflowRun,
  workflowOf,
  workflowRuns,
  workflows,
  workflowsBus,
} from '../services/workflows';
import { BASE_STYLES } from '../styles/shared';
import styles from './workflows-page.css?inline';

import type { FlowAddRequest, FlowGraph, SelectOption, Tone } from '@syware/jlib';
import type {
  Workflow,
  WorkflowDraft,
  WorkflowExit,
  WorkflowRun,
  WorkflowStep,
} from '../../../shared/protocol/workflow.schema';

const APP_NAME = 'swcode';
const GLOBAL = '';
const NEW_STEP_PROMPT = 'Do this step’s work.';
const STEP_NAME = 'Step';
const SELECT_A_STEP = 'Select a step to change what it is and what it does.';
const UNSAVED = 'This workflow is not saved yet.';
const NO_WORKFLOWS =
  'No workflow yet. Write one here, or ask an agent for a team: it proposes one, which its person confirms.';
const NO_RUNS = 'No run yet. Choose a workflow in the composer when you start a session.';
const NEW_HINT =
  'Teams of sessions, global or a project’s. A session picks one with its first message; each step runs in a session of its own, handed the previous step’s report.';
const STEPS_HINT =
  'Start feeds the brief to the step after it; End finishes the run. Drag a step to move it; the + under a step adds the next one. A step’s outputs let it go several ways, each with when it is taken; draw an edge from an output to its next step.';
const REMOVED = 'The workflow is removed. Its runs stay as history.';
// How long a page waits for the workflow its address names to come with the list.
const PENDING_WORKFLOW_MS = 1500;

/**
 * The workflows: the teams of sessions a person writes here or an agent proposes, global or a project's, and their
 * runs. The editor is jl-flow: a node a step, an edge the work it hands on.
 */
export class WorkflowsPage extends ScElement {
  static readonly selector = 'workflows-page';

  readonly route = this.useAttributes({ id: () => this.openRoute() });

  private _draft: WorkflowDraft = emptyWorkflow();
  // The id the draft saves under; empty until a new one is saved.
  private _editingId = '';
  // The route id whose draft the editor holds, so a redraw does not replace what the person typed.
  private _draftKey = '';
  // Each output's form of the step selected, by its exit id: the values are read when the step is saved.
  private readonly _exitRows = new Map<string, { label: JlInput; when: JlTextarea }>();
  private _dirty = false;
  private _editorShown = false;
  private _pendingId = '';
  private _selected = '';
  // The id of a new workflow just saved, until its first list comes back.
  private _savedId = '';

  private readonly _refs = this.useRefs((ref) => ({
    addExit: ref<JlButton>('.add-exit'),
    addStep: ref<JlButton>('.add-step'),
    confirm: ref<JlButton>('.confirm'),
    description: ref<JlInput>('.description'),
    draftBadge: ref<JlBadge>('.draft-badge'),
    editorView: ref('.editor-view'),
    emptyRuns: ref<JlEmpty>('.empty-runs'),
    emptyWorkflows: ref<JlEmpty>('.empty-workflows'),
    exits: ref('.exits'),
    feedback: ref(JlAlert),
    flow: ref(JlFlow),
    list: ref('.list-view'),
    name: ref<JlInput>('.name'),
    newWorkflow: ref<JlButton>('.new-workflow'),
    page: ref(JlPage),
    project: ref<JlSelect>('.project'),
    remove: ref<JlButton>('.remove'),
    runs: ref('.runs'),
    save: ref<JlButton>('.save'),
    stepApply: ref<JlButton>('.step-apply'),
    stepCard: ref<JlCard>('.step-card'),
    stepEmpty: ref('.step-empty'),
    stepFields: ref('.step-fields'),
    stepFooter: ref('.step-footer'),
    stepName: ref<JlInput>('.step-name'),
    stepPreset: ref<JlSelect>('.step-preset'),
    stepPrompt: ref<JlTextarea>('.step-prompt'),
    stepRemove: ref<JlButton>('.step-remove'),
    stepRole: ref<JlTextarea>('.step-role'),
    stepSpecial: ref('.step-special'),
    workflows: ref('.workflows'),
  }));

  templateHtml(): Node {
    return (
      <JlPage appName={APP_NAME} width="full">
        <JlBadge class="draft-badge" slot="meta" tone="warn" label="Draft" hidden />
        <JlButton
          class="new-workflow"
          slot="actions"
          variant="primary"
          icon="plus"
          label="New workflow"
          href={hrefOf(workflowPath(NEW_WORKFLOW))}
        />
        <JlButton class="confirm" slot="actions" label="Confirm" hidden />
        <JlButton class="save" slot="actions" variant="primary" label="Save" hidden />
        <JlButton class="remove" slot="actions" variant="ghost" icon="trash" label="Remove" hidden />
        <JlAlert hidden />
        <div class="list-view">
          <section>
            <h2>Workflows</h2>
            <div class="workflows" />
            <JlEmpty
              class="empty-workflows"
              icon="workflow"
              heading="No workflows yet"
              description={NO_WORKFLOWS}
            />
          </section>
          <section>
            <h2>Runs</h2>
            <div class="runs" />
            <JlEmpty class="empty-runs" icon="workflow" heading="No runs yet" description={NO_RUNS} />
          </section>
        </div>
        <div class="editor-view" hidden>
          <section class="about">
            <JlField label="Name">
              <JlInput class="name" autocomplete="off" maxlength={String(MAX_WORKFLOW_NAME)} />
            </JlField>
            <JlField label="Description">
              <JlTextarea class="description" rows={2} maxlength={String(MAX_WORKFLOW_DESCRIPTION)} />
            </JlField>
            <JlField label="Offered in" hint="Every project, or the one it works in.">
              <JlSelect class="project" />
            </JlField>
          </section>
          <div class="editor-grid">
            <JlCard class="flow-card" heading="Steps" description={STEPS_HINT}>
              <div class="flow">
                <JlFlow label="The steps of the workflow" />
              </div>
              <JlButton class="add-step" icon="plus" label="New step" size="sm" />
            </JlCard>
            <JlCard class="step-card" heading="Step" hidden>
              <div class="step-fields">
                <JlField label="Name">
                  <JlInput class="step-name" autocomplete="off" maxlength={String(MAX_WORKFLOW_STEP_NAME)} />
                </JlField>
                <JlField
                  label="Role"
                  hint="Who this step is, and how it works. Given to its session before its task.">
                  <JlTextarea class="step-role" rows={4} maxlength={String(MAX_WORKFLOW_ROLE)} />
                </JlField>
                <JlField
                  label="Task"
                  hint="What it has to do. The person’s brief and the previous step’s handoff come with it; the outputs say when it takes each way on.">
                  <JlTextarea class="step-prompt" rows={4} maxlength={String(MAX_WORKFLOW_PROMPT)} />
                </JlField>
                <JlField
                  label="Preset"
                  hint="The models its session works with; the project’s when left as it is.">
                  <JlSelect class="step-preset" />
                </JlField>
                <div class="outputs">
                  <div class="outputs-head">
                    <span class="outputs-title">Outputs</span>
                    <JlButton class="add-exit" icon="plus" label="Add output" size="sm" />
                  </div>
                  <p class="outputs-hint">
                    Each output is a way this step can go on, and when it takes it. Draw an edge from the output to
                    the step it goes to.
                  </p>
                  <div class="exits" />
                </div>
              </div>
              <p class="step-special" hidden />
              <div class="row step-footer">
                <JlButton class="step-remove" variant="ghost" icon="trash" label="Remove the step" />
                <JlButton class="step-apply" variant="primary" label="Save the step" />
              </div>
            </JlCard>
            <p class="step-empty">{SELECT_A_STEP}</p>
          </div>
        </div>
      </JlPage>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this.onBus(workflowsBus, 'changed', () => this.renderAll());
    this.onBus(presetsBus, 'changed', () => this.fillPresets());
    this.onBus(daemonBus, 'message', (message) => {
      if (message.type === 'problem') {
        this.say(message.message, 'danger');
      }
    });
    const flow = this._refs.flow;
    flow.events.register('change', (graph: FlowGraph) => this.graphChanged(graph));
    flow.events.register('select', (stepId: string) => this.selectStep(stepId));
    flow.events.register('add', (request: FlowAddRequest) => this.addAfter(request));
    this._refs.save.events.register('press', () => this.save());
    this._refs.confirm.events.register('press', () => this.confirm());
    this._refs.remove.events.register('press', () => void this.removeWorkflow());
    this._refs.addStep.events.register('press', () => this.addStep());
    this._refs.stepApply.events.register('press', () => this.applyStep());
    this._refs.addExit.events.register('press', () => this.addExit());
    this._refs.stepRemove.events.register('press', () => this.removeStep());
    this._refs.name.events.register('change', () => {
      this._dirty = true;
      this._refs.page.heading.set(this._refs.name.value.get().trim() || STEP_NAME);
    });
    this._refs.description.events.register('change', () => {
      this._dirty = true;
    });
    this._refs.project.events.register('change', () => {
      this._dirty = true;
    });
    if (!this._editorShown) {
      this.renderList();
    }
    this.renderAll();
  }

  private openRoute(): void {
    const id = this.route.id;
    if (id === null || id === '') {
      this._pendingId = '';
      this.showList();
      return;
    }
    if (id === NEW_WORKFLOW) {
      if (this._draftKey !== NEW_WORKFLOW) {
        this.openNew();
      }
      return;
    }
    if (this._draftKey === id || this._savedId === id) {
      this.showEditorView();
      return;
    }
    const workflow = workflowOf(id);
    if (workflow) {
      this.openWorkflow(workflow);
      return;
    }
    // Its list may still be on its way: a new workflow is saved before the daemon tells every app of it.
    this._pendingId = id;
    this.showList();
    setTimeout(() => this.giveUpPending(id), PENDING_WORKFLOW_MS);
  }

  /** The saved id of a new workflow, before its first list comes back. */
  private renderAll(): void {
    if (this._pendingId !== '') {
      const workflow = workflowOf(this._pendingId);
      if (workflow) {
        const id = this._pendingId;
        this._pendingId = '';
        this.openWorkflow(workflow);
        if (workflow.id !== id) {
          navigate(workflowPath(workflow.id));
        }
        return;
      }
    }
    if (!this._editorShown) {
      this.renderList();
      return;
    }
    const workflow = this._editingId === '' ? undefined : workflowOf(this._editingId);
    if (workflow) {
      this._draft.draft = workflow.draft;
      this.renderDraftState();
    }
  }

  /** A workflow its address named never came: the page says so rather than wait for good. */
  private giveUpPending(id: string): void {
    if (this._pendingId !== id || workflowOf(id) !== undefined || !this.isConnected) {
      return;
    }
    this._pendingId = '';
    this.say('This workflow is not here.', 'danger');
    this.showList();
  }

  private showList(): void {
    this._editorShown = false;
    this._draftKey = '';
    this._selected = '';
    this._refs.list.hidden = false;
    this._refs.editorView.hidden = true;
    this._refs.page.heading.set('Workflows');
    this._refs.page.description.set(NEW_HINT);
    this._refs.page.back.set('');
    this._refs.page.backLabel.set('');
    this._refs.newWorkflow.hidden = false;
    this._refs.save.hidden = true;
    this._refs.confirm.hidden = true;
    this._refs.remove.hidden = true;
    this._refs.draftBadge.hidden = true;
    this.renderList();
  }

  private openNew(): void {
    this._draft = emptyWorkflow();
    this._editingId = '';
    this._draftKey = NEW_WORKFLOW;
    this._dirty = false;
    this._selected = '';
    this.fillEditor();
  }

  /** Shows the editor the route names, its draft already in hand: a save that came back first. */
  private showEditorView(): void {
    this._editorShown = true;
    this._refs.list.hidden = true;
    this._refs.editorView.hidden = false;
  }

  private openWorkflow(workflow: Workflow): void {
    this._draft = draftOf(workflow);
    this._editingId = workflow.id;
    this._draftKey = workflow.id;
    this._dirty = false;
    this._selected = '';
    this.fillEditor();
  }

  private fillEditor(): void {
    this.showEditorView();
    this._refs.name.value.set(this._draft.name);
    this._refs.description.value.set(this._draft.description);
    const projects = allProjects();
    const options: SelectOption[] = [
      { label: 'Every project', value: GLOBAL },
      ...projects.map((project) => ({ label: project.name, value: project.id })),
    ];
    this._refs.project.options.set(options);
    this._refs.project.value.set(this._draft.projectId ?? GLOBAL);
    this._refs.page.back.set(hrefOf(WORKFLOWS_PATH));
    this._refs.page.backLabel.set('Workflows');
    this._refs.page.heading.set(this._draft.name || 'New workflow');
    this._refs.newWorkflow.hidden = true;
    this._refs.save.hidden = false;
    this._refs.remove.hidden = this._editingId === '';
    this.fillPresets();
    this.setGraph();
    this.fillStepForm();
    this.renderDraftState();
  }

  private renderDraftState(): void {
    this._refs.page.description.set(this.scopeText(this._draft));
    this._refs.draftBadge.hidden = !this._draft.draft;
    this._refs.confirm.hidden = !this._draft.draft || this._editingId === '';
  }

  private scopeText(draft: WorkflowDraft): string {
    const project =
      draft.projectId === undefined ? undefined : allProjects().find((each) => each.id === draft.projectId);
    const scope = project?.name ?? 'Every project';
    const steps = draft.steps.filter((step) => step.kind === 'step').length;
    return `${scope} · ${steps} step${steps === 1 ? '' : 's'}`;
  }

  private setGraph(): void {
    this._refs.flow.graph.set(flowOf(this._draft.steps, this._draft.edges));
    this._refs.flow.fit();
    if (this._selected !== '') {
      this._refs.flow.selected.set(this._selected);
    }
  }

  private renderList(): void {
    const workflowsShown = workflows();
    this._refs.workflows.replaceChildren(...workflowsShown.map((workflow) => this.workflowNode(workflow)));
    this._refs.emptyWorkflows.hidden = workflowsShown.length > 0;
    const runs = [...workflowRuns()].sort((first, second) => second.createdAt.localeCompare(first.createdAt));
    this._refs.runs.replaceChildren(...runs.map((run) => this.runNode(run)));
    this._refs.emptyRuns.hidden = runs.length > 0;
  }

  private workflowNode(workflow: Workflow): Node {
    const row = createElement(JlListItem, {
      props: {
        description: `${this.workflowScope(workflow)} · ${workflow.steps.filter((step) => step.kind === 'step').length} steps`,
        heading: workflow.name,
        href: hrefOf(workflowPath(workflow.id)),
        icon: 'workflow',
      },
    });
    if (workflow.draft) {
      row.append(createElement(JlBadge, { attrs: { slot: 'end' }, props: { label: 'Draft', tone: 'warn' } }));
    }
    return row;
  }

  private workflowScope(workflow: Workflow): string {
    return workflow.projectId === undefined
      ? 'Every project'
      : (allProjects().find((project) => project.id === workflow.projectId)?.name ?? 'One project');
  }

  private runNode(run: WorkflowRun): Node {
    const look = RUN_STATUS[run.status];
    const done = run.steps.filter((step) => step.status === 'done').length;
    const row = createElement(JlListItem, {
      props: {
        description: `${look.label} · ${done} of ${run.steps.length} steps · ${this.runPlace(run)}`,
        heading: run.workflowName,
        href: hrefOf(sessionPath(run.sessionId)),
        icon: 'workflow',
      },
    });
    row.append(createElement(JlBadge, { attrs: { slot: 'end' }, props: { label: look.label, tone: look.tone } }));
    if (isRunning(run.status)) {
      const stop = createElement(JlButton, {
        attrs: { class: 'stop', slot: 'end' },
        props: { hint: 'Stop this run', icon: 'stop', label: 'Stop', size: 'sm', variant: 'ghost' },
      });
      stop.events.register('press', () => stopWorkflowRun(run.id));
      row.append(stop);
    }
    return row;
  }

  private runPlace(run: WorkflowRun): string {
    return allProjects().find((project) => project.id === run.projectId)?.name ?? 'Every project';
  }

  private graphChanged(graph: FlowGraph): void {
    const steps = graph.nodes.flatMap((node) => {
      const step = this._draft.steps.find((each) => each.id === node.id);
      return step === undefined ? [] : [{ ...step, position: node.position }];
    });
    this._draft.steps = steps;
    this._draft.edges = graph.edges.map((edge) => ({
      from: edge.from,
      id: edge.id,
      to: edge.to,
      ...(edge.port === undefined ? {} : { port: edge.port }),
    }));
    this._dirty = true;
    if (this._selected !== '' && !steps.some((step) => step.id === this._selected)) {
      this._selected = '';
      this.fillStepForm();
    }
    this.renderDraftState();
  }

  private selectStep(stepId: string): void {
    this._selected = stepId;
    this.fillStepForm();
  }

  private fillPresets(): void {
    const options: SelectOption[] = [
      { label: 'The project’s preset', value: '' },
      ...presetChoices().presets.map((preset) => ({ label: preset.name, value: preset.id })),
    ];
    this._refs.stepPreset.options.set(options);
  }

  private fillStepForm(): void {
    const step = this._draft.steps.find((each) => each.id === this._selected);
    this._refs.stepCard.hidden = step === undefined;
    this._refs.stepEmpty.hidden = step !== undefined;
    this._exitRows.clear();
    this._refs.exits.replaceChildren();
    if (!step) {
      return;
    }
    const special = step.kind !== 'step';
    this._refs.stepFields.hidden = special;
    this._refs.stepSpecial.hidden = !special;
    this._refs.stepFooter.hidden = special;
    this._refs.stepCard.heading.set(step.kind === 'start' ? 'Start' : step.kind === 'end' ? 'End' : 'Step');
    if (special) {
      this._refs.stepSpecial.textContent =
        step.kind === 'start'
          ? 'The brief you send starts here and goes to the step this leads to.'
          : 'When a step’s output leads here, the run is finished.';
      return;
    }
    this._refs.stepSpecial.textContent = '';
    this._refs.stepName.value.set(step.name);
    this._refs.stepRole.value.set(step.role);
    this._refs.stepPrompt.value.set(step.prompt);
    this._refs.stepPreset.value.set(step.preset ?? '');
    this._refs.exits.replaceChildren(...step.exits.map((exit) => this.exitRow(exit)));
  }

  /** One of the step's outputs: its name, when to take it, and the button that removes it. */
  private exitRow(exit: WorkflowExit): Node {
    const row = document.createElement('div');
    row.className = 'exit-row';
    row.dataset.exit = exit.id;
    const label = createElement(JlInput, { props: { autocomplete: 'off', value: exit.label } });
    label.className = 'exit-label';
    const labelField = createElement(JlField, { props: { label: 'Output' } });
    labelField.append(label);
    const when = createElement(JlTextarea, { props: { rows: 2, value: exit.when } });
    when.className = 'exit-when';
    const whenField = createElement(JlField, { props: { label: 'Take it when' } });
    whenField.append(when);
    const remove = createElement(JlButton, {
      attrs: { class: 'exit-remove' },
      props: { hint: 'Remove this output', icon: 'trash', size: 'sm', variant: 'ghost' },
    });
    remove.events.register('press', () => this.removeExit(exit.id));
    this._exitRows.set(exit.id, { label, when });
    row.append(labelField, whenField, remove);
    return row;
  }

  /** A new output of the step selected; the edge it had with no output, if one, leaves by it. */
  private addExit(): void {
    const step = this._draft.steps.find((each) => each.id === this._selected);
    if (!step || step.kind !== 'step') {
      return;
    }
    const exit: WorkflowExit = {
      id: `exit-${crypto.randomUUID()}`,
      label: `Output ${step.exits.length + 1}`,
      when: 'When the work is done.',
    };
    this._draft.steps = this._draft.steps.map((each) =>
      each.id === step.id ? { ...each, exits: [...each.exits, exit] } : each
    );
    if (step.exits.length === 0) {
      this._draft.edges = this._draft.edges.map((edge) =>
        edge.from === step.id && edge.port === undefined ? { ...edge, port: exit.id } : edge
      );
    }
    this._dirty = true;
    this.setGraph();
    this.fillStepForm();
    this.renderDraftState();
  }

  private removeExit(exitId: string): void {
    const step = this._draft.steps.find((each) => each.id === this._selected);
    if (!step) {
      return;
    }
    this._draft.steps = this._draft.steps.map((each) =>
      each.id === step.id ? { ...each, exits: each.exits.filter((exit) => exit.id !== exitId) } : each
    );
    this._draft.edges = this._draft.edges.filter((edge) => !(edge.from === step.id && edge.port === exitId));
    this._dirty = true;
    this.setGraph();
    this.fillStepForm();
    this.renderDraftState();
  }

  /** Takes the step form into the draft; the problem when it cannot, empty when it did. */
  private applySelectedStep(): string {
    const step = this._draft.steps.find((each) => each.id === this._selected);
    if (!step || step.kind !== 'step') {
      return '';
    }
    const name = this._refs.stepName.value.get().trim();
    const prompt = this._refs.stepPrompt.value.get().trim();
    if (name === '') {
      return 'A step needs a name.';
    }
    if (prompt === '') {
      return 'A step needs what it has to do.';
    }
    const preset = this._refs.stepPreset.value.get();
    const role = this._refs.stepRole.value.get().trim();
    const exits = step.exits.map((exit) => {
      const row = this._exitRows.get(exit.id);
      return row === undefined
        ? exit
        : { ...exit, label: row.label.value.get().trim(), when: row.when.value.get().trim() };
    });
    if (exits.some((exit) => exit.label === '' || exit.when === '')) {
      return 'Each output needs a name and when it is taken.';
    }
    if (
      name === step.name
      && prompt === step.prompt
      && role === step.role
      && preset === (step.preset ?? '')
      && JSON.stringify(exits) === JSON.stringify(step.exits)
    ) {
      return '';
    }
    this._draft.steps = this._draft.steps.map((each) =>
      each.id === step.id
        ? {
            ...each,
            exits,
            name,
            prompt,
            role,
            ...(preset === '' ? { preset: undefined } : { preset }),
          }
        : each
    );
    this._dirty = true;
    return '';
  }

  private applyStep(): void {
    const problem = this.applySelectedStep();
    if (problem !== '') {
      this.say(problem, 'danger');
      return;
    }
    this.say('', 'info');
    this.setGraph();
    this.renderDraftState();
  }

  /** A step after the one selected, or in the middle of the canvas when none is: Start and End stay in place. */
  private addStep(): void {
    const selected = this._draft.steps.find((step) => step.id === this._selected);
    const position =
      selected === undefined
        ? this._refs.flow.centerPosition()
        : this._refs.flow.positionAfter(selected.id, selected.exits[0]?.id ?? '');
    const step = this.addStepAt(position);
    const end = this._draft.steps.find((each) => each.kind === 'end');
    if (selected?.kind === 'start') {
      if (!this._draft.edges.some((edge) => edge.from === selected.id)) {
        this.link(selected.id, step.id);
      }
    } else if (selected?.kind === 'step') {
      if (selected.exits.length > 0) {
        this.link(selected.id, step.id, selected.exits[0]?.id);
      } else {
        this._draft.edges = this._draft.edges.filter(
          (edge) => !(edge.from === selected.id && edge.to === end?.id)
        );
        this.link(selected.id, step.id);
      }
    } else if (selected === undefined) {
      const start = this._draft.steps.find((each) => each.kind === 'start');
      if (start !== undefined && !this._draft.edges.some((edge) => edge.from === start.id)) {
        this.link(start.id, step.id);
      }
    }
    this.connectEnd(step);
    this.setGraph();
  }

  private addAfter(request: FlowAddRequest): void {
    const step = this.addStepAt(request.position);
    this.link(request.from, step.id, request.port === '' ? undefined : request.port);
    this.connectEnd(step);
    this.setGraph();
  }

  private link(from: string, to: string, port?: string): void {
    this._draft.edges = [
      ...this._draft.edges,
      { from, id: `edge-${crypto.randomUUID()}`, to, ...(port === undefined || port === '' ? {} : { port }) },
    ];
  }

  /** A step with one way and nothing after it goes to the End, so a new step is part of a runnable chain. */
  private connectEnd(step: WorkflowStep): void {
    const end = this._draft.steps.find((each) => each.kind === 'end');
    if (end === undefined || step.exits.length > 0 || this._draft.edges.some((edge) => edge.from === step.id)) {
      return;
    }
    this.link(step.id, end.id);
  }

  private addStepAt(position: { x: number; y: number }): WorkflowStep {
    const id = `step-${crypto.randomUUID()}`;
    const step: WorkflowStep = {
      exits: [],
      id,
      kind: 'step',
      name: `${STEP_NAME} ${this._draft.steps.length + 1}`,
      position,
      prompt: NEW_STEP_PROMPT,
      role: '',
    };
    this._draft.steps = [...this._draft.steps, step];
    this._selected = id;
    this._dirty = true;
    this.setGraph();
    this.fillStepForm();
    this.renderDraftState();
    requestAnimationFrame(() => this._refs.flow.reveal(id));
    return step;
  }

  private removeStep(): void {
    const step = this._draft.steps.find((each) => each.id === this._selected);
    if (!step || step.kind !== 'step') {
      return;
    }
    this._draft.steps = this._draft.steps.filter((each) => each.id !== step.id);
    this._draft.edges = this._draft.edges.filter((edge) => edge.from !== step.id && edge.to !== step.id);
    this._selected = '';
    this._dirty = true;
    this.setGraph();
    this.fillStepForm();
    this.renderDraftState();
  }

  private draftFromForm(): WorkflowDraft {
    const projectId = this._refs.project.value.get();
    return {
      ...this._draft,
      description: this._refs.description.value.get().trim(),
      name: this._refs.name.value.get().trim(),
      ...(projectId === GLOBAL ? { projectId: undefined } : { projectId }),
    };
  }

  private save(): void {
    const stepProblem = this.applySelectedStep();
    if (stepProblem !== '') {
      this.say(stepProblem, 'danger');
      return;
    }
    this.setGraph();
    const draft = this.draftFromForm();
    if (draft.name === '') {
      this.say('The workflow needs a name.', 'danger');
      return;
    }
    if (!draft.draft) {
      const problem = graphProblem(draft.steps, draft.edges);
      if (problem !== '') {
        this.say(problem, 'danger');
        return;
      }
    }
    this._draft = draft;
    if (this._editingId !== '') {
      saveWorkflow(this._editingId, draft);
      this._dirty = false;
      this.say('Saved.', 'ok');
      return;
    }
    const id = crypto.randomUUID();
    this._editingId = id;
    this._draftKey = id;
    this._savedId = id;
    this._dirty = false;
    saveWorkflow(id, draft);
    this._refs.remove.hidden = false;
    this.renderDraftState();
    navigate(workflowPath(id));
    this.say('Saved.', 'ok');
  }

  private confirm(): void {
    const stepProblem = this.applySelectedStep();
    if (stepProblem !== '') {
      this.say(stepProblem, 'danger');
      return;
    }
    if (this._editingId === '') {
      this.say(UNSAVED, 'danger');
      return;
    }
    if (this._dirty) {
      this.say('Save your changes, then confirm the workflow.', 'danger');
      return;
    }
    const problem = graphProblem(this._draft.steps, this._draft.edges);
    if (problem !== '') {
      this.say(problem, 'danger');
      return;
    }
    confirmWorkflow(this._editingId);
  }

  private async removeWorkflow(): Promise<void> {
    const id = this._editingId;
    if (id === '') {
      navigate(WORKFLOWS_PATH);
      return;
    }
    const confirmed = await confirmDialog({
      confirmLabel: 'Remove',
      heading: `Remove ${this._draft.name}?`,
      text: REMOVED,
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    removeWorkflow(id);
    navigate(WORKFLOWS_PATH);
  }

  private say(text: string, tone: Tone): void {
    this._refs.feedback.tone.set(tone);
    this._refs.feedback.description.set(text);
    this._refs.feedback.hidden = text === '';
  }
}

WorkflowsPage.register();
