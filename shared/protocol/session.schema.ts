import { z } from 'zod';

import { PictureRefSchema } from './picture.schema.js';
import { HostRefSchema, LOCAL_HOST } from './project.schema.js';
import { SkillSummarySchema } from './skill.schema.js';

/** The tools the agent can call. The model sees these names, the app picks icons and labels from them. */
export const TOOL_NAMES = [
  'read',
  'write',
  'edit',
  'bash',
  'grep',
  'glob',
  'agent',
  'plan',
  'question',
  'websearch',
  'webfetch',
  'image',
  // A chat's: it draws a picture and shows it in the chat, where the agent's image saves it in the workspace.
  'draw',
  'output',
  'kill',
  'skill',
  'saveskill',
  'screenshot',
] as const;
export const ToolNameSchema = z.enum(TOOL_NAMES);
export type ToolName = z.infer<typeof ToolNameSchema>;

/** How a person answers a permission request: this call only, every similar call in the session, or no. */
export const PERMISSION_DECISIONS = ['allowOnce', 'allowSession', 'deny'] as const;
export const PermissionDecisionSchema = z.enum(PERMISSION_DECISIONS);
export type PermissionDecision = z.infer<typeof PermissionDecisionSchema>;

/**
 * `auto`: edits and commands go through. `acceptEdits`: edits go through, commands wait for a person. `ask`: both
 * wait.
 */
export const PERMISSION_MODES = ['auto', 'acceptEdits', 'ask'] as const;
export const PermissionModeSchema = z.enum(PERMISSION_MODES);
export type PermissionMode = z.infer<typeof PermissionModeSchema>;

/**
 * How much the model thinks before it answers: as much as it does by itself (`full`), less (`medium`, `low`, the
 * levels Qwen 3.8 and gpt-oss templates know), or not at all (`off`, instruct).
 */
export const THINKING_MODES = ['full', 'medium', 'low', 'off'] as const;
export const ThinkingModeSchema = z.enum(THINKING_MODES);
export type ThinkingMode = z.infer<typeof ThinkingModeSchema>;

/**
 * What a session is: an agent at work in a project (`code`), or a conversation with the model, which reads the web
 * and nothing else (`chat`).
 */
export const SESSION_KINDS = ['code', 'chat'] as const;
export const SessionKindSchema = z.enum(SESSION_KINDS);
export type SessionKind = z.infer<typeof SessionKindSchema>;

export const SESSION_STATUSES = ['idle', 'working', 'waiting'] as const;
export const SessionStatusSchema = z.enum(SESSION_STATUSES);
export type SessionStatus = z.infer<typeof SessionStatusSchema>;

/** How a turn ended: the model was done, someone stopped it, it failed, or it ran out of steps. */
export const TURN_OUTCOMES = ['done', 'stopped', 'failed', 'limit'] as const;
export const TurnOutcomeSchema = z.enum(TURN_OUTCOMES);
export type TurnOutcome = z.infer<typeof TurnOutcomeSchema>;

export const DIFF_LINE_KINDS = ['add', 'remove', 'context', 'gap'] as const;

export const DiffLineSchema = z.object({
  kind: z.enum(DIFF_LINE_KINDS),
  newLine: z.number().int().optional(),
  oldLine: z.number().int().optional(),
  text: z.string(),
});
export type DiffLine = z.infer<typeof DiffLineSchema>;

/** A question the agent asks the person: answers to pick, one or several; the person may also write their own. */
export const QuestionSchema = z.object({
  header: z.string(),
  multiSelect: z.boolean(),
  options: z.array(z.object({ description: z.string(), label: z.string() })),
  question: z.string(),
});
export type Question = z.infer<typeof QuestionSchema>;

/** Why swcode spoke to the model in the middle of a turn: its plan had steps left, or the turn ran out of steps. */
export const REMINDER_REASONS = ['plan', 'stepLimit'] as const;
export type ReminderReason = (typeof REMINDER_REASONS)[number];

/** Where a step of the agent's plan stands: the names of Claude Code's task list, which the models know. */
export const PLAN_STEP_STATUSES = ['pending', 'in_progress', 'completed'] as const;
export const PlanStepSchema = z.object({ content: z.string(), status: z.enum(PLAN_STEP_STATUSES) });
export type PlanStep = z.infer<typeof PlanStepSchema>;
export type PlanStepStatus = PlanStep['status'];

/**
 * What the app shows for a tool call besides its one-line summary: the change to a file, some text, the
 * agent's plan as it stands, or a picture it made (the daemon keeps a copy to show, at `url`).
 */
export const ToolDisplaySchema = z.discriminatedUnion('kind', [
  z.object({
    added: z.number().int(),
    kind: z.literal('diff'),
    lines: z.array(DiffLineSchema),
    path: z.string(),
    removed: z.number().int(),
  }),
  z.object({ kind: z.literal('text'), text: z.string() }),
  z.object({ kind: z.literal('plan'), steps: z.array(PlanStepSchema) }),
  z.object({ kind: z.literal('image'), path: z.string(), url: z.string() }),
]);
export type ToolDisplay = z.infer<typeof ToolDisplaySchema>;
export type DiffDisplay = Extract<ToolDisplay, { kind: 'diff' }>;

/** A tool call as the model wrote it: its arguments are the model's JSON text, valid or not. */
export const ToolCallSchema = z.object({ arguments: z.string(), id: z.string(), name: z.string() });
export type ToolCall = z.infer<typeof ToolCallSchema>;

export const UsageSchema = z.object({
  completionTokens: z.number().int(),
  promptTokens: z.number().int(),
});
export type Usage = z.infer<typeof UsageSchema>;

const at = z.iso.datetime();

/**
 * What happened in a session, in order. The session file keeps these events, the model's messages are rebuilt
 * from them, and the app draws the conversation from them.
 */
export const SessionEventSchema = z.discriminatedUnion('kind', [
  z.object({ at, id: z.string(), kind: z.literal('turnStarted') }),
  // The pictures the person pasted with it, kept beside the session as files; concise, sent in token economy: the
  // model reads "Be concise." after the words, which the app shows as written.
  z.object({
    at,
    concise: z.boolean().optional(),
    id: z.string(),
    kind: z.literal('userMessage'),
    pictures: z.array(PictureRefSchema).optional(),
    text: z.string(),
  }),
  z.object({
    at,
    id: z.string(),
    kind: z.literal('assistantMessage'),
    // What the model thought before it answered, when it says (gpt-oss, Qwen3 with thinking on).
    reasoning: z.string().optional(),
    text: z.string(),
    toolCalls: z.array(ToolCallSchema),
    // The tokens of this reply: its prompt is the conversation so far, so it tells how full the context is.
    usage: UsageSchema.optional(),
  }),
  // The conversation so far, summarized when the context filled up: the model goes on from the summary.
  z.object({ at, id: z.string(), kind: z.literal('contextCompacted'), summary: z.string() }),
  // swcode spoke to the model in the middle of a turn, as a message: it ended its turn with steps of its plan left,
  // and the turn went on; or it reached the step limit, and was asked for its report.
  z.object({
    at,
    id: z.string(),
    kind: z.literal('reminder'),
    reason: z.enum(REMINDER_REASONS),
    text: z.string(),
  }),
  // The user's skills, as the model was told them: at a turn's start when they changed, and after a compaction.
  z.object({ at, id: z.string(), kind: z.literal('skillsListed'), skills: z.array(SkillSummarySchema) }),
  z.object({
    at,
    callId: z.string(),
    kind: z.literal('toolCall'),
    // The call in a few words: the file read, the command run.
    summary: z.string(),
    // `undefined` when the model asked for a tool that does not exist.
    tool: ToolNameSchema.optional(),
  }),
  z.object({
    at,
    callId: z.string(),
    detail: ToolDisplaySchema.optional(),
    kind: z.literal('permissionRequested'),
    requestId: z.string(),
  }),
  z.object({
    at,
    decision: PermissionDecisionSchema,
    kind: z.literal('permissionAnswered'),
    requestId: z.string(),
  }),
  // The agent asked the person questions with the question tool, and waits for the answers.
  z.object({
    at,
    callId: z.string(),
    kind: z.literal('questionAsked'),
    questions: z.array(QuestionSchema),
    requestId: z.string(),
  }),
  // One answer a question, in order; none when the turn stopped first.
  z.object({ answers: z.array(z.string()), at, kind: z.literal('questionAnswered'), requestId: z.string() }),
  z.object({ at, callId: z.string(), kind: z.literal('toolRunning') }),
  z.object({
    at,
    callId: z.string(),
    display: ToolDisplaySchema.optional(),
    kind: z.literal('toolFinished'),
    // The result in a few words, beside the summary: "142 lines", "exit code 1".
    meta: z.string().optional(),
    ok: z.boolean(),
    // What the model reads back.
    output: z.string(),
    // Pictures the model reads back too, after the result: a screenshot it asked to see.
    pictures: z.array(PictureRefSchema).optional(),
  }),
  // A subagent the main agent started with the agent tool: its own calls show under that call, in a line each.
  z.object({
    agentId: z.string(),
    at,
    callId: z.string(),
    description: z.string(),
    kind: z.literal('subagentStarted'),
    model: z.string().optional(),
  }),
  z.object({
    agentId: z.string(),
    at,
    kind: z.literal('subagentActivity'),
    meta: z.string().optional(),
    ok: z.boolean(),
    summary: z.string(),
    tool: ToolNameSchema.optional(),
  }),
  z.object({ agentId: z.string(), at, kind: z.literal('subagentFinished'), outcome: TurnOutcomeSchema }),
  z.object({
    at,
    id: z.string(),
    kind: z.literal('turnFinished'),
    message: z.string().optional(),
    outcome: TurnOutcomeSchema,
  }),
]);
export type SessionEvent = z.infer<typeof SessionEventSchema>;
export type UserMessageEvent = Extract<SessionEvent, { kind: 'userMessage' }>;
export type SessionEventKind = SessionEvent['kind'];

export const SessionSummarySchema = z.object({
  // Token economy: each message sent asks the model to be concise.
  concise: z.boolean().default(false),
  host: HostRefSchema.default(LOCAL_HOST),
  id: z.string(),
  // Commands the agent left running in the background (servers, watchers), until they end or are stopped.
  jobs: z.array(z.object({ command: z.string(), id: z.string() })).default([]),
  // A chat works in no folder: its workspace is empty, its host this computer.
  kind: SessionKindSchema.default('code'),
  mode: PermissionModeSchema,
  // The preset whose models the session works with; the default preset when left out, or when this one was removed.
  preset: z.string().optional(),
  // Messages sent while a turn ran: they start the next turn together.
  queued: z.array(z.string()),
  // The person asked for them now: they go to the model at the turn's next step instead.
  queuedNow: z.boolean().default(false),
  status: SessionStatusSchema,
  thinking: ThinkingModeSchema,
  title: z.string(),
  updatedAt: at,
  workspace: z.string(),
});
export type SessionSummary = z.infer<typeof SessionSummarySchema>;
