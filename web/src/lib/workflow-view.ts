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

/** The graph's anchors: the brief goes on from Start, and End finishes the run. */
export const START_STEP_ID = 'start';
export const END_STEP_ID = 'end';

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
    const start = step.kind === 'start';
    const finish = step.kind === 'end';
    return {
      description: start ? 'The brief goes on from here' : finish ? 'The run is done' : roleLine(step.role),
      icon: start ? 'rocket' : finish ? 'check' : 'user',
      id: step.id,
      label: step.name,
      position: step.position,
      ...(finish
        ? { ports: [] }
        : step.exits.length === 0
          ? {}
          : { ports: step.exits.map((exit) => ({ id: exit.id, label: exit.label })) }),
      ...(start ? { tone: 'ok' as const } : {}),
      start: start || !reached.has(step.id),
      ...(done === undefined
        ? {}
        : {
            ...(NODE_STATUS[done.status] === undefined ? {} : { status: NODE_STATUS[done.status] }),
            ...(done.status === 'stopped' ? { problem: 'Stopped.' } : {}),
          }),
    };
  });
  return {
    edges: edges.map((edge) => ({
      from: edge.from,
      id: edge.id,
      to: edge.to,
      ...(edge.port === undefined ? {} : { port: edge.port }),
    })),
    nodes,
  };
}

/** A workflow as the editor holds it: its dates and id left to the daemon, with Start and End for an older one. */
export const draftOf = (workflow: Workflow): WorkflowDraft => {
  const draft: WorkflowDraft = {
    description: workflow.description,
    draft: workflow.draft,
    edges: workflow.edges.map((edge) => ({ ...edge })),
    name: workflow.name,
    ...(workflow.projectId === undefined ? {} : { projectId: workflow.projectId }),
    steps: workflow.steps.map((step) => ({ ...step, position: { ...step.position } })),
  };
  if (draft.steps.some((step) => step.kind === 'start' || step.kind === 'end')) {
    return draft;
  }
  // A workflow from before the anchors: Start above its first step, End below, and its one-way steps leading there.
  const incoming = new Set(draft.edges.map((edge) => edge.to));
  const outgoing = new Set(draft.edges.map((edge) => edge.from));
  const first = draft.steps.find((step) => !incoming.has(step.id));
  const top = Math.min(...draft.steps.map((step) => step.position.y), 0);
  const bottom = Math.max(...draft.steps.map((step) => step.position.y), 0);
  const start: WorkflowStep = {
    exits: [],
    id: START_STEP_ID,
    kind: 'start',
    name: 'Start',
    position: { x: 0, y: top - 136 },
    prompt: '',
    role: '',
  };
  const end: WorkflowStep = {
    exits: [],
    id: END_STEP_ID,
    kind: 'end',
    name: 'End',
    position: { x: 0, y: bottom + 136 },
    prompt: '',
    role: '',
  };
  const edges = [...draft.edges];
  if (first !== undefined) {
    edges.push({ from: start.id, id: `edge-${crypto.randomUUID()}`, to: first.id });
  }
  for (const step of draft.steps) {
    if (!outgoing.has(step.id) && step.exits.length === 0) {
      edges.push({ from: step.id, id: `edge-${crypto.randomUUID()}`, to: end.id });
    }
  }
  return { ...draft, edges, steps: [start, ...draft.steps, end] };
};

/** A new workflow, as the editor opens it: no step yet, and global until its person says otherwise. */
export const emptyWorkflow = (): WorkflowDraft => ({
  description: '',
  draft: true,
  edges: [],
  name: 'New workflow',
  steps: [
    {
      exits: [],
      id: START_STEP_ID,
      kind: 'start',
      name: 'Start',
      position: { x: 0, y: -136 },
      prompt: '',
      role: '',
    },
    { exits: [], id: END_STEP_ID, kind: 'end', name: 'End', position: { x: 0, y: 136 }, prompt: '', role: '' },
  ],
});

/** Why a workflow cannot run, or nothing when it can: the same checks the daemon makes. */
export function graphProblem(steps: readonly WorkflowStep[], edges: readonly WorkflowEdge[]): string {
  if (steps.length === 0) {
    return 'A workflow needs at least one step.';
  }
  const stepOf = new Map<string, WorkflowStep>();
  let start: WorkflowStep | undefined;
  let finish: WorkflowStep | undefined;
  for (const step of steps) {
    if (step.id === '' || stepOf.has(step.id)) {
      return 'Each step needs an id of its own.';
    }
    stepOf.set(step.id, step);
    if (step.kind === 'start') {
      if (start !== undefined) {
        return 'A workflow has one Start.';
      }
      start = step;
    } else if (step.kind === 'end') {
      if (finish !== undefined) {
        return 'A workflow has one End.';
      }
      finish = step;
    } else if (step.kind === 'step') {
      if (step.name.trim() === '') {
        return 'Each step needs a name.';
      }
      if (step.prompt.trim() === '') {
        return `The step ${step.name} needs what it has to do.`;
      }
    } else {
      return 'Each step is a role, the Start or the End.';
    }
    if (step.kind !== 'step' && step.exits.length > 0) {
      return 'The Start and the End have no outputs.';
    }
  }
  if (start === undefined || finish === undefined) {
    return 'A workflow needs one Start and one End.';
  }
  const exits = new Map<string, Set<string>>();
  for (const step of steps) {
    const ids = new Set<string>();
    for (const exit of step.exits) {
      if (exit.id === '' || ids.has(exit.id)) {
        return `The step ${step.name} has two outputs with one id.`;
      }
      ids.add(exit.id);
      if (exit.label.trim() === '' || exit.when.trim() === '') {
        return `The output ${exit.id} of ${step.name} needs a name and when it is taken.`;
      }
    }
    exits.set(step.id, ids);
  }
  let startEdges = 0;
  const outgoing = new Map<string, number>();
  for (const edge of edges) {
    const from = stepOf.get(edge.from);
    const to = stepOf.get(edge.to);
    if (from === undefined || to === undefined) {
      return 'An edge joins a step that is not in the workflow.';
    }
    if (edge.from === edge.to) {
      return 'A step cannot hand its work to itself.';
    }
    if (to.kind === 'start') {
      return 'The Start takes no edge leading to it.';
    }
    if (from.kind === 'end') {
      return 'The End goes nowhere.';
    }
    outgoing.set(from.id, (outgoing.get(from.id) ?? 0) + 1);
    if (from.kind === 'start') {
      startEdges++;
      if (edge.port !== undefined && edge.port !== '') {
        return 'The Start has no outputs.';
      }
      if (to.kind === 'end') {
        return 'The Start leads to a step, not to the End.';
      }
      continue;
    }
    if (from.exits.length > 0) {
      if (edge.port === undefined || edge.port === '') {
        return `The edge from ${from.name} leaves by no output: choose one of its outputs.`;
      }
      if (!exits.get(from.id)?.has(edge.port)) {
        return `The step ${from.name} has no output ${edge.port}.`;
      }
    } else if (edge.port !== undefined && edge.port !== '') {
      return `The step ${from.name} has no output ${edge.port}.`;
    }
  }
  if (startEdges !== 1) {
    return 'The Start leads to one step.';
  }
  if ((outgoing.get(finish.id) ?? 0) > 0) {
    return 'The End goes nowhere.';
  }
  for (const step of steps) {
    if (step.kind === 'step') {
      if ((outgoing.get(step.id) ?? 0) === 0) {
        return `The step ${step.name} goes nowhere: give it an output, or an edge to the End.`;
      }
      if (step.exits.length === 0 && (outgoing.get(step.id) ?? 0) > 1) {
        return `The step ${step.name} goes several ways: give it outputs.`;
      }
    }
    for (const exit of step.exits) {
      const used = edges.filter((edge) => edge.from === step.id && edge.port === exit.id).length;
      if (used === 0) {
        return `The output ${exit.id} of ${step.name} has no edge: it goes to one step.`;
      }
      if (used > 1) {
        return `The output ${exit.id} of ${step.name} has ${used} edges: it goes to one step only.`;
      }
    }
  }
  const after = new Map<string, string[]>();
  for (const edge of edges) {
    after.set(edge.from, [...(after.get(edge.from) ?? []), edge.to]);
  }
  const seen = new Set<string>([start.id]);
  const queue = [start.id];
  while (queue.length > 0) {
    const at = queue.shift() ?? '';
    for (const next of after.get(at) ?? []) {
      if (!seen.has(next)) {
        seen.add(next);
        queue.push(next);
      }
    }
  }
  if (!seen.has(finish.id)) {
    return 'The End is not reached: give a step an output that leads to the End.';
  }
  if (seen.size !== steps.length) {
    return 'Every step is reached from the Start: one of them is off the path.';
  }
  return '';
}
