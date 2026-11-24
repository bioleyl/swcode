import { z } from 'zod';

/** The most a workflow, its steps and its runs may hold. */
export const MAX_WORKFLOW_NAME = 80;
export const MAX_WORKFLOW_DESCRIPTION = 2000;
export const MAX_WORKFLOW_STEP_NAME = 60;
export const MAX_WORKFLOW_ROLE = 8000;
export const MAX_WORKFLOW_PROMPT = 8000;
export const MAX_WORKFLOW_STEPS = 50;
export const MAX_WORKFLOW_EXITS = 10;

/** A workflow step is a role; Start and End only anchor the graph, and run no session. */
export const WorkflowStepKindSchema = z.enum(['step', 'start', 'end']);
export type WorkflowStepKind = z.infer<typeof WorkflowStepKindSchema>;
export const MAX_WORKFLOW_EDGES = 100;
export const MAX_WORKFLOWS = 100;
export const MAX_WORKFLOW_RUNS = 50;
export const MAX_WORKFLOW_BRIEF = 20_000;
/** The longest the run's list says of what a step reported. */
export const MAX_WORKFLOW_STEP_SUMMARY = 200;

/** Where a node sits on the editor's canvas: its top left corner. */
export const WorkflowPointSchema = z.object({ x: z.number(), y: z.number() });
export type WorkflowPoint = z.infer<typeof WorkflowPointSchema>;

/** An output of a step: what it is called, and when the step takes it. Its edge leaves by this id. */
export const WorkflowExitSchema = z.object({
  id: z
    .string()
    .trim()
    .min(1)
    .max(80)
    .regex(/^[A-Za-z0-9_-]+$/, 'letters, digits, - and _'),
  label: z.string().trim().min(1).max(MAX_WORKFLOW_STEP_NAME),
  // The condition the step's agent reads: take this output when ...
  when: z.string().trim().min(1).max(MAX_WORKFLOW_PROMPT),
});
export type WorkflowExit = z.infer<typeof WorkflowExitSchema>;

/** One role of a workflow: the session it starts, what it is told to do, and where its node sits. */
export const WorkflowStepSchema = z.object({
  // Its outputs, each with the condition the agent takes it under; an edge leaves by the output's id.
  exits: z.array(WorkflowExitSchema).max(MAX_WORKFLOW_EXITS).default([]),
  // Paths are made from it: letters, digits, - and _ alone, as the edges name it too.
  id: z
    .string()
    .trim()
    .min(1)
    .max(80)
    .regex(/^[A-Za-z0-9_-]+$/, 'letters, digits, - and _'),
  // A role, or the Start or End of the graph.
  kind: WorkflowStepKindSchema.default('step'),
  // What the step is called: PO, Dev, QA.
  name: z.string().trim().min(1).max(MAX_WORKFLOW_STEP_NAME),
  position: WorkflowPointSchema,
  // The preset its session works with; the project's when left out.
  preset: z.string().optional(),
  // What the step has to do. The run adds the brief and the previous step's handoff to it. Empty on Start and End.
  prompt: z.string().trim().max(MAX_WORKFLOW_PROMPT).default(''),
  // The role the step's session works under: given before its task, as its instructions.
  role: z.string().trim().max(MAX_WORKFLOW_ROLE),
});
export type WorkflowStep = z.infer<typeof WorkflowStepSchema>;

/** A step after another: the first step has no edge leading to it. */
export const WorkflowEdgeSchema = z.object({
  from: z.string(),
  id: z.string(),
  // The exit it leaves by, when a node has several; the first exit when left out.
  port: z.string().optional(),
  to: z.string(),
});
export type WorkflowEdge = z.infer<typeof WorkflowEdgeSchema>;

/**
 * A workflow: the roles of a team, each a session of its own, and the order they hand their work on in. It belongs to
 * one project when it says so, and is offered in every project otherwise. A draft waits for its person to confirm it.
 */
export const WorkflowSchema = z.object({
  createdAt: z.iso.datetime(),
  description: z.string().trim().max(MAX_WORKFLOW_DESCRIPTION).default(''),
  draft: z.boolean().default(false),
  edges: z.array(WorkflowEdgeSchema).max(MAX_WORKFLOW_EDGES).default([]),
  id: z.string(),
  name: z.string().trim().min(1).max(MAX_WORKFLOW_NAME),
  // The project it is offered in; every project when left out.
  projectId: z.string().optional(),
  steps: z.array(WorkflowStepSchema).max(MAX_WORKFLOW_STEPS).default([]),
  updatedAt: z.iso.datetime(),
});
export type Workflow = z.infer<typeof WorkflowSchema>;

/** What a workflow needs to be saved or proposed, before the daemon gives it its id and dates. */
export const WorkflowDraftSchema = WorkflowSchema.omit({ createdAt: true, id: true, updatedAt: true });
export type WorkflowDraft = z.infer<typeof WorkflowDraftSchema>;

/** How a step of a run stands. */
export const WorkflowStepStatusSchema = z.enum(['pending', 'running', 'waiting', 'done', 'failed', 'stopped']);
export type WorkflowStepStatus = z.infer<typeof WorkflowStepStatusSchema>;

/** How a run stands: working, waiting for the person, finished, failed, or stopped. */
export const WorkflowRunStatusSchema = z.enum(['running', 'waiting', 'done', 'failed', 'stopped']);
export type WorkflowRunStatus = z.infer<typeof WorkflowRunStatusSchema>;

/** One step of a run: the session it started, how it went, and the handoff file it left. */
export const WorkflowRunStepSchema = z.object({
  finishedAt: z.iso.datetime().optional(),
  // The file the next step reads, in the project: .swcode/workflows/<runId>/<stepId>.md.
  handoff: z.string().optional(),
  name: z.string(),
  sessionId: z.string().optional(),
  startedAt: z.iso.datetime().optional(),
  status: WorkflowStepStatusSchema.default('pending'),
  stepId: z.string(),
  // The first line of its report, for the run's list: what this step did, when its name repeats.
  summary: z.string().max(MAX_WORKFLOW_STEP_SUMMARY).optional(),
});
export type WorkflowRunStep = z.infer<typeof WorkflowRunStepSchema>;

/** A workflow at work: its brief, its run session, and the session of each step. */
export const WorkflowRunSchema = z.object({
  brief: z.string().max(MAX_WORKFLOW_BRIEF),
  createdAt: z.iso.datetime(),
  id: z.string(),
  // Why it failed, when it did.
  problem: z.string().optional(),
  projectId: z.string(),
  // The session that shows the run and starts its steps; it works on no role itself.
  sessionId: z.string(),
  status: WorkflowRunStatusSchema.default('running'),
  steps: z.array(WorkflowRunStepSchema),
  updatedAt: z.iso.datetime(),
  workflowId: z.string(),
  workflowName: z.string(),
});
export type WorkflowRun = z.infer<typeof WorkflowRunSchema>;
