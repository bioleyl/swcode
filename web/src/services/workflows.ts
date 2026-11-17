import { Bus } from '@jadis/core';

import { daemon, daemonBus } from './daemon';

import type {
  Workflow,
  WorkflowDraft,
  WorkflowRun,
  WorkflowRunStatus,
} from '../../../shared/protocol/workflow.schema';

// The workflows live as long as the page: their bus listener never needs to stop.
const forever = new AbortController().signal;

/** The workflows and their runs, as the daemon last listed them: at connection, and again when one changes. */
export const workflowsBus = new Bus<{ changed: undefined }>();

const state = { loaded: false, runs: [] as WorkflowRun[], workflows: [] as Workflow[] };

/** Whether the daemon has said its workflows yet: a page waits for it before it says one is not there. */
export const workflowsLoaded = (): boolean => state.loaded;

export const workflows = (): readonly Workflow[] => state.workflows;
export const workflowRuns = (): readonly WorkflowRun[] => state.runs;

/** The workflows a session of a project may pick: those of that project, and the global ones. */
export const workflowsFor = (projectId: string | undefined): readonly Workflow[] =>
  state.workflows.filter(
    (workflow) =>
      !workflow.draft
      && (projectId === undefined || workflow.projectId === undefined || workflow.projectId === projectId)
  );

export const workflowOf = (id: string | undefined): Workflow | undefined =>
  state.workflows.find((workflow) => workflow.id === id);

export const runOf = (id: string | undefined): WorkflowRun | undefined => state.runs.find((run) => run.id === id);

/** The runs of a workflow, newest first. */
export const runsOfWorkflow = (workflowId: string): readonly WorkflowRun[] =>
  state.runs
    .filter((run) => run.workflowId === workflowId)
    .sort((first, second) => second.createdAt.localeCompare(first.createdAt));

export const isRunning = (status: WorkflowRunStatus): boolean => status === 'running' || status === 'waiting';

daemonBus.register(
  'message',
  (message) => {
    if (message.type !== 'workflows') {
      return;
    }
    state.workflows = message.workflows;
    state.runs = message.runs;
    state.loaded = true;
    workflowsBus.emit('changed');
  },
  forever
);

/** Saves a workflow as the editor holds it: a new one when it names no id, that one otherwise. */
export const saveWorkflow = (id: string | undefined, workflow: WorkflowDraft): void =>
  daemon.send({ type: 'saveWorkflow', workflow, ...(id === undefined ? {} : { id }) });

/** Takes a draft into the workflows a session can pick, once its graph runs. */
export const confirmWorkflow = (workflowId: string): void => daemon.send({ type: 'confirmWorkflow', workflowId });

/** Forgets a workflow, and its runs. */
export const removeWorkflow = (workflowId: string): void => daemon.send({ type: 'removeWorkflow', workflowId });

/** Stops a run: its step at work stops too, and no further step starts. */
export const stopWorkflowRun = (runId: string): void => daemon.send({ runId, type: 'stopWorkflowRun' });
