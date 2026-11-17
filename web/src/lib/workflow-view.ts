import type { FlowGraph, FlowNode, FlowNodeStatus, Tone } from '@syware/jlib';
import type {
  Workflow,
  WorkflowDraft,
  WorkflowEdge,
  WorkflowRunStatus,
  WorkflowRunStep,
  WorkflowStep,
  WorkflowStepStatus,
} from '../../../shared/protocol/workflow.schema';

/** The longest a node's second line says of a role. */
const ROLE_LINE = 60;
const ELLIPSIS = '…';

/** How a run reads: a new status does not compile until it is here. */
export const RUN_STATUS: Readonly<Record<WorkflowRunStatus, { label: string; tone: Tone }>> = {
  done: { label: 'Done', tone: 'ok' },
  failed: { label: 'Failed', tone: 'danger' },
  running: { label: 'Running', tone: 'info' },
  stopped: { label: 'Stopped', tone: 'neutral' },
  waiting: { label: 'Waiting', tone: 'warn' },
};

/** How a step reads: a new status does not compile until it is here. */
export const STEP_STATUS: Readonly<Record<WorkflowStepStatus, { label: string; tone: Tone }>> = {
  done: { label: 'Done', tone: 'ok' },
  failed: { label: 'Failed', tone: 'danger' },
  pending: { label: 'Pending', tone: 'neutral' },
  running: { label: 'Running', tone: 'info' },
  stopped: { label: 'Stopped', tone: 'neutral' },
  waiting: { label: 'Waiting', tone: 'warn' },
};

/** What a run's node shows of its step: nothing while it is pending, and a problem where it stopped. */
const NODE_STATUS: Readonly<Record<WorkflowStepStatus, FlowNodeStatus | undefined>> = {
  done: 'done',
  failed: 'failed',
  pending: undefined,
  running: 'running',
  stopped: 'failed',
  waiting: 'waiting',
};

const roleLine = (role: string): string | undefined => {
  const line = role.trim().split('\n')[0] ?? '';
  if (line === '') {
    return undefined;
  }
  return line.length <= ROLE_LINE ? line : `${line.slice(0, ROLE_LINE).trimEnd()}${ELLIPSIS}`;
};

const starts = (edges: readonly WorkflowEdge[]): ReadonlySet<string> => new Set(edges.map((edge) => edge.to));

/** The flow the editor shows: a node a step, its exits the edges after it. */
export function flowOf(
  steps: readonly WorkflowStep[],
  edges: readonly WorkflowEdge[],
  run?: readonly WorkflowRunStep[]
): FlowGraph {
  const reached = starts(edges);
  const byStep = new Map((run ?? []).map((step) => [step.stepId, step]));
  const nodes: FlowNode[] = steps.map((step) => {
    const done = byStep.get(step.id);
    const role = roleLine(step.role);
    return {
      description: role,
      icon: 'user',
      id: step.id,
      label: step.name,
      position: step.position,
      ...(step.exits.length === 0
        ? {}
        : { ports: step.exits.map((exit) => ({ id: exit.id, label: exit.label })) }),
      start: !reached.has(step.id),
      ...(done === undefined
        ? {}
        : {
            ...(NODE_STATUS[done.status] === undefined ? {} : { status: NODE_STATUS[done.status] }),
            ...(done.status === 'stopped' ? { problem: 'Stopped.' } : {}),
          }),
    };
  });
  return { edges: edges.map((edge) => ({ from: edge.from, id: edge.id, to: edge.to })), nodes };
}

/** A workflow as the editor holds it: its dates and id left to the daemon. */
export const draftOf = (workflow: Workflow): WorkflowDraft => ({
  description: workflow.description,
  draft: workflow.draft,
  edges: workflow.edges.map((edge) => ({ ...edge })),
  name: workflow.name,
  ...(workflow.projectId === undefined ? {} : { projectId: workflow.projectId }),
  steps: workflow.steps.map((step) => ({ ...step, position: { ...step.position } })),
});

/** A new workflow, as the editor opens it: no step yet, and global until its person says otherwise. */
export const emptyWorkflow = (): WorkflowDraft => ({
  description: '',
  draft: true,
  edges: [],
  name: 'New workflow',
  steps: [],
});

/** Why a workflow cannot run, or nothing when it can: the same checks the daemon makes. */
export function graphProblem(steps: readonly WorkflowStep[], edges: readonly WorkflowEdge[]): string {
  if (steps.length === 0) {
    return 'A workflow needs at least one step.';
  }
  const ids = new Set<string>();
  for (const step of steps) {
    if (step.id === '' || ids.has(step.id)) {
      return 'Each step needs an id of its own.';
    }
    ids.add(step.id);
    if (step.name.trim() === '') {
      return 'Each step needs a name.';
    }
    if (step.prompt.trim() === '') {
      return `The step ${step.name} needs what it has to do.`;
    }
  }
  for (const step of steps) {
    if (step.exits.length === 0) {
      continue;
    }
    const exits = new Set<string>();
    for (const exit of step.exits) {
      if (exit.id === '' || exits.has(exit.id)) {
        return `The step ${step.name} has two outputs with one id.`;
      }
      exits.add(exit.id);
      if (exit.label.trim() === '' || exit.when.trim() === '') {
        return `The output ${exit.id} of ${step.name} needs a name and when it is taken.`;
      }
    }
    for (const edge of edges) {
      if (edge.from === step.id && !exits.has(edge.port ?? '')) {
        return `The step ${step.name} has no output ${edge.port ?? ''}.`;
      }
    }
    for (const exit of step.exits) {
      if (edges.filter((edge) => edge.from === step.id && edge.port === exit.id).length !== 1) {
        return `The output ${exit.id} of ${step.name} goes to one step only.`;
      }
    }
  }
  for (const step of steps) {
    if (step.exits.length === 0 && edges.filter((edge) => edge.from === step.id).length > 1) {
      return `The step ${step.name} goes several ways: give it outputs.`;
    }
  }
  const incoming = new Map<string, number>();
  const after = new Map<string, string[]>();
  for (const edge of edges) {
    if (!ids.has(edge.from) || !ids.has(edge.to)) {
      return 'An edge joins a step that is not in the workflow.';
    }
    if (edge.from === edge.to) {
      return 'A step cannot hand its work to itself.';
    }
    if (edge.port !== undefined) {
      const source = steps.find((step) => step.id === edge.from);
      if (source === undefined || !source.exits.some((exit) => exit.id === edge.port)) {
        return `The step ${source?.name ?? edge.from} has no output ${edge.port}.`;
      }
    }
    incoming.set(edge.to, (incoming.get(edge.to) ?? 0) + 1);
    after.set(edge.from, [...(after.get(edge.from) ?? []), edge.to]);
  }
  const starts = steps.filter((step) => (incoming.get(step.id) ?? 0) === 0);
  if (starts.length !== 1) {
    return `A workflow starts at one step: it has ${starts.length}.`;
  }
  const seen = new Set<string>([starts[0]?.id ?? '']);
  const queue = [starts[0]?.id ?? ''];
  while (queue.length > 0) {
    const at = queue.shift() ?? '';
    for (const next of after.get(at) ?? []) {
      if (!seen.has(next)) {
        seen.add(next);
        queue.push(next);
      }
    }
  }
  if (seen.size !== steps.length) {
    return 'Every step is reached from the start: one of them is off the path.';
  }
  return '';
}
