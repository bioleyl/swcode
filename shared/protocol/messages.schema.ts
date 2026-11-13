import { z } from 'zod';

import { SessionDefaultsSchema } from './choices.schema.js';
import { PresetChoicesSchema, PresetStatusSchema, SourceStatusSchema } from './models.schema.js';
import { MAX_PICTURES, PictureUploadSchema } from './picture.schema.js';
import {
  HostOptionSchema,
  HostRefSchema,
  MAX_AFTER_EDIT,
  MAX_PINNED_TODOS,
  NewFolderNameSchema,
  ProjectSchema,
  TodoTextSchema,
} from './project.schema.js';
import {
  PermissionDecisionSchema,
  PermissionModeSchema,
  SessionEventSchema,
  SessionKindSchema,
  SessionSummarySchema,
  ThinkingModeSchema,
  ToolDisplaySchema,
  ToolNameSchema,
} from './session.schema.js';

/**
 * Where a question beside the session (/peek) is: waiting for the agent's request running to end, being answered,
 * answered, or failed (its `message` says why).
 */
export const PEEK_STATES = ['waiting', 'answering', 'done', 'failed'] as const;
export const PeekStateSchema = z.enum(PEEK_STATES);
export type PeekState = z.infer<typeof PeekStateSchema>;

/**
 * A session whose conversation says what the palette's search typed: the event that says it last, a line around the
 * words there (the person's message, the model's answer, a summary, or a tool call's file or command).
 */
export const SessionMatchSchema = z.object({
  eventId: z.string(),
  sessionId: z.string(),
  snippet: z.string(),
});
export type SessionMatch = z.infer<typeof SessionMatchSchema>;

/** How much of a terminal the app shows, in characters: a shell is kept the same size as the view that types in it. */
export const MAX_TERMINAL_COLUMNS = 500;
export const MAX_TERMINAL_ROWS = 200;
const TerminalColumnsSchema = z.number().int().min(2).max(MAX_TERMINAL_COLUMNS);
const TerminalRowsSchema = z.number().int().min(1).max(MAX_TERMINAL_ROWS);

/** How long the name of a terminal may be: a few words, which the app lists in its side panel. */
export const MAX_TERMINAL_TITLE = 60;

/** Whether a terminal's shell is still there, and how it ended: a state of its own, since code 0 is an exit too. */
export const TerminalStateSchema = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('running') }),
  z.object({ code: z.number().int(), kind: z.literal('exited') }),
  // A signal ended it: a hangup when the daemon stops, or the person's kill.
  z.object({ kind: z.literal('signalled') }),
]);
export type TerminalState = z.infer<typeof TerminalStateSchema>;

/** A shell running in a project's folder, on its host: people type in it from the app, and the agent starts them too. */
export const TerminalSchema = z.object({
  // Where it runs: the project's folder, on the host that folder is on.
  cwd: z.string(),
  host: HostRefSchema,
  id: z.string(),
  state: TerminalStateSchema,
  // What it is called in the app: a few words, or its number.
  title: z.string(),
});
export type Terminal = z.infer<typeof TerminalSchema>;

/** What the app sends the daemon over the socket. */
export const ClientMessageSchema = z.discriminatedUnion('type', [
  // A folder of its own (a coding session's), and the first message to start on at once, both optional.
  z.object({
    // Token economy from the first message on.
    concise: z.boolean().optional(),
    // This computer when left out.
    host: HostRefSchema.optional(),
    // A coding session when left out; a chat takes no folder, host or permissions.
    kind: SessionKindSchema.optional(),
    mode: PermissionModeSchema.optional(),
    // The first message's pictures; a message may be pictures alone.
    pictures: z.array(PictureUploadSchema).max(MAX_PICTURES).optional(),
    // Todos of the project appended to this session's first message: their ids link them to it.
    pinnedTodoIds: z.array(z.string()).max(MAX_PINNED_TODOS).optional(),
    pinnedTodos: z.array(TodoTextSchema).max(MAX_PINNED_TODOS).optional(),
    // The default preset when left out.
    preset: z.string().optional(),
    prompt: z.string().trim().optional(),
    thinking: ThinkingModeSchema.optional(),
    type: z.literal('createSession'),
    workspace: z.string().trim().min(1).optional(),
  }),
  z.object({ sessionId: z.string(), title: z.string().trim().min(1).max(120), type: z.literal('renameSession') }),
  z.object({ sessionId: z.string(), type: z.literal('deleteSession') }),
  // Pins a message of the session, to jump back to it from the side panel: the daemon keeps its first words as the
  // label, and answers with the sessions' list. A pin that is already there is left as it is.
  z.object({ eventId: z.string().min(1), sessionId: z.string(), type: z.literal('pinEvent') }),
  z.object({ eventId: z.string().min(1), sessionId: z.string(), type: z.literal('unpinEvent') }),
  z.object({ sessionId: z.string(), type: z.literal('clearQueue') }),
  // The queued messages go to the model at the running turn's next step, without waiting for its end.
  z.object({ sessionId: z.string(), type: z.literal('sendQueueNow') }),
  // The end of a session's conversation: at most `events` of its last events, so a long one shows quickly and the
  // older ones come as the person scrolls up. A page back online says how many of the session's events it holds
  // (`since`, the ones kept back counted): only those after them come then, and the page goes on where it was.
  // Opening a session also has its stream come to this socket (its events, its answer as it is written), in place
  // of the one it had open.
  z.object({
    events: z.number().int().positive().optional(),
    sessionId: z.string(),
    since: z.number().int().positive().optional(),
    type: z.literal('openSession'),
  }),
  // The page shows that session no more, or none yet (no `sessionId`): no session's stream comes to this socket
  // until it opens one. A socket that says neither is sent every session's events, as the desktop app's watch of
  // another computer needs them, though no answer as it is written.
  z.object({ sessionId: z.string().optional(), type: z.literal('closeSession') }),
  z.object({
    pictures: z.array(PictureUploadSchema).max(MAX_PICTURES).optional(),
    sessionId: z.string(),
    // Empty with pictures alone.
    text: z.string().trim(),
    type: z.literal('sendPrompt'),
  }),
  z.object({
    decision: PermissionDecisionSchema,
    requestId: z.string(),
    sessionId: z.string(),
    type: z.literal('answerPermission'),
  }),
  // One answer a question, in the order asked.
  z.object({
    answers: z.array(z.string().trim().min(1)).min(1),
    requestId: z.string(),
    sessionId: z.string(),
    type: z.literal('answerQuestion'),
  }),
  // The password a command asked for, typed in its card; empty for nothing given, which closes the command's input.
  // It is never kept in the session.
  z.object({
    requestId: z.string(),
    secret: z.string(),
    sessionId: z.string(),
    type: z.literal('answerPassword'),
  }),
  z.object({ sessionId: z.string(), type: z.literal('stopTurn') }),
  // Puts back the files a turn wrote or edited, as they were before it: refused while the session's turn runs.
  z.object({ sessionId: z.string(), turnId: z.string(), type: z.literal('restoreCheckpoint') }),
  // The `events` messages older than the ones a page shows, sent up from the end of its history, past the
  // `keptBack` the last history said it held back: what a page asks for as the person scrolls to the top. The
  // answer is a `sessionHistory` of them.
  z.object({
    events: z.number().int().positive(),
    keptBack: z.number().int().min(0),
    sessionId: z.string(),
    type: z.literal('olderEvents'),
  }),
  // Summarizes the conversation now (/compact), in a turn of its own: refused while a turn runs.
  z.object({ sessionId: z.string(), type: z.literal('compactSession') }),
  // The conversations that say every word typed in the palette's search; the answer goes to this socket alone.
  z.object({ query: z.string().trim().min(1), type: z.literal('searchSessions') }),
  // A question beside the session (/peek): answered from the conversation as it is, never kept in it. The app names
  // it, to tell its answer.
  z.object({
    peekId: z.string().min(1),
    question: z.string().trim().min(1),
    sessionId: z.string(),
    type: z.literal('peek'),
  }),
  // Stops a command the agent left running in the background.
  z.object({ jobId: z.string(), sessionId: z.string(), type: z.literal('stopJob') }),
  // A shell in a project's folder, on its host, for the person to type in; the agent opens terminals too. The answer
  // is a terminalOpened, with what it printed so far, and its output comes to this socket from then on.
  z.object({
    columns: TerminalColumnsSchema,
    cwd: z.string().trim().min(1),
    // This computer when left out.
    host: HostRefSchema.optional(),
    rows: TerminalRowsSchema,
    // The daemon numbers it when left out: Terminal, Terminal 2…
    title: z.string().trim().min(1).max(MAX_TERMINAL_TITLE).optional(),
    type: z.literal('openTerminal'),
  }),
  // Shows a terminal again, after a page reload or a socket that went away: its kept output, then what comes next.
  z.object({
    columns: TerminalColumnsSchema,
    rows: TerminalRowsSchema,
    terminalId: z.string(),
    type: z.literal('attachTerminal'),
  }),
  // Stops hearing its output, when the person goes back to the conversation.
  z.object({ terminalId: z.string(), type: z.literal('detachTerminal') }),
  // What the person typed: the terminal's input, as the shell reads it.
  z.object({ data: z.string(), terminalId: z.string(), type: z.literal('terminalInput') }),
  z.object({
    columns: TerminalColumnsSchema,
    rows: TerminalRowsSchema,
    terminalId: z.string(),
    type: z.literal('terminalResize'),
  }),
  // Ends the shell, and takes the terminal out of the list.
  z.object({ terminalId: z.string(), type: z.literal('closeTerminal') }),
  // The files of a turn's restore point, each as it was before the turn and as it is now: the answer is a
  // checkpointDiff, to this socket alone.
  z.object({ sessionId: z.string(), turnId: z.string(), type: z.literal('checkpointDiff') }),
  // Names a terminal, as its page and the side panel list it.
  z.object({
    terminalId: z.string(),
    title: z.string().trim().min(1).max(MAX_TERMINAL_TITLE),
    type: z.literal('renameTerminal'),
  }),
  z.object({ mode: PermissionModeSchema, sessionId: z.string(), type: z.literal('setMode') }),
  z.object({ sessionId: z.string(), thinking: ThinkingModeSchema, type: z.literal('setThinking') }),
  // The session's models from its next request on: a preset's id.
  z.object({ preset: z.string().min(1), sessionId: z.string(), type: z.literal('setPreset') }),
  // Token economy from the next message sent on: the model reads "Be concise." after each.
  z.object({ concise: z.boolean(), sessionId: z.string(), type: z.literal('setConcise') }),
  // The person saw the session as it was at that activity, on this device: every device shows it seen.
  z.object({ seenAt: z.iso.datetime(), sessionId: z.string(), type: z.literal('markSeen') }),
  // A project of a folder that exists, or of a new folder made inside it.
  z.object({
    folder: z.string().trim().min(1),
    // This computer when left out.
    host: HostRefSchema.optional(),
    newFolder: NewFolderNameSchema.optional(),
    type: z.literal('createProject'),
  }),
  z.object({ projectId: z.string(), type: z.literal('removeProject') }),
  // What the project's new sessions start with: the app's own defaults again when left out.
  z.object({
    defaults: SessionDefaultsSchema.optional(),
    projectId: z.string(),
    type: z.literal('setProjectDefaults'),
  }),
  // The command run after each file the agent writes or edits in the project; none when empty.
  z.object({
    command: z.string().trim().max(MAX_AFTER_EDIT),
    projectId: z.string(),
    type: z.literal('setProjectAfterEdit'),
  }),
  // Whether the commands of the project's sessions run in a sandbox, where its host can make one.
  z.object({ on: z.boolean(), projectId: z.string(), type: z.literal('setProjectSandbox') }),
  // Turns one of the skills the project's sessions find on or off for them, from their next turn on.
  z.object({
    name: z.string().min(1),
    on: z.boolean(),
    projectId: z.string(),
    type: z.literal('setProjectSkill'),
  }),
  // Adds a todo to the project.
  z.object({
    projectId: z.string(),
    text: TodoTextSchema,
    type: z.literal('addProjectTodo'),
  }),
  // Marks one of the project's todos done or not.
  z.object({
    done: z.boolean(),
    projectId: z.string(),
    todoId: z.string(),
    type: z.literal('setProjectTodoDone'),
  }),
  // Changes what one of the project's todos says.
  z.object({
    projectId: z.string(),
    text: TodoTextSchema,
    todoId: z.string(),
    type: z.literal('setProjectTodoText'),
  }),
  z.object({
    projectId: z.string(),
    todoId: z.string(),
    type: z.literal('removeProjectTodo'),
  }),
  // Where projects can be: this computer, its WSL distributions, the SSH endpoints.
  z.object({ type: z.literal('listHosts') }),
  // Asks whether the daemon still hears this socket: it answers `pong`. A socket can go without either end being
  // told, a phone's above all, and a page that hears nothing back opens another.
  z.object({ type: z.literal('ping') }),
  // The subfolders of a folder, to pick a project's; without a path, of the folder projects usually go in.
  z.object({
    host: HostRefSchema.optional(),
    path: z.string().trim().min(1).optional(),
    type: z.literal('listFolders'),
  }),
]);
export type ClientMessage = z.infer<typeof ClientMessageSchema>;
export type ClientMessageType = ClientMessage['type'];

/**
 * Which part of a session a history answer holds: its end, drawn as the conversation; the page above what is shown,
 * added over it; or what came since the events a page back online holds, added after them.
 */
export const HistoryPartSchema = z.enum(['end', 'older', 'since']);
export type HistoryPart = z.infer<typeof HistoryPartSchema>;

/** The answer being written right now, so a page opened mid-answer shows what came so far. */
export const StreamingSchema = z.object({ messageId: z.string(), text: z.string() });
export type Streaming = z.infer<typeof StreamingSchema>;

/** What the daemon sends the app. Session events go to every open socket; history only to the one asking. */
export const ServerMessageSchema = z.discriminatedUnion('type', [
  z.object({ sessions: z.array(SessionSummarySchema), type: z.literal('sessions') }),
  z.object({ session: SessionSummarySchema, type: z.literal('sessionCreated') }),
  // The answer to a search, the newest sessions first: the query comes back, for the app to drop an answer too late.
  z.object({ matches: z.array(SessionMatchSchema), query: z.string(), type: z.literal('sessionMatches') }),
  // A page's history: the events it asked for, which part of the session they are, and how many older ones the
  // daemon kept back, so the app knows whether scrolling up has anything more to find.
  z.object({
    events: z.array(SessionEventSchema),
    keptBack: z.number().int().min(0),
    part: HistoryPartSchema,
    sessionId: z.string(),
    streaming: StreamingSchema.optional(),
    type: z.literal('sessionHistory'),
  }),
  z.object({ event: SessionEventSchema, sessionId: z.string(), type: z.literal('sessionEvent') }),
  z.object({ messageId: z.string(), sessionId: z.string(), text: z.string(), type: z.literal('assistantDelta') }),
  z.object({ messageId: z.string(), sessionId: z.string(), text: z.string(), type: z.literal('reasoningDelta') }),
  // The model started writing a call to one of its tools: its arguments (a whole file, at times) stream on unseen.
  z.object({
    messageId: z.string(),
    sessionId: z.string(),
    tool: ToolNameSchema,
    type: z.literal('toolCallWriting'),
  }),
  // What a command printed since the last of these, while it still runs: shown under its call until its result
  // takes its place. Never kept: the result holds what the command printed in all.
  z.object({ callId: z.string(), sessionId: z.string(), text: z.string(), type: z.literal('toolOutput') }),
  z.object({ message: z.string(), type: z.literal('problem') }),
  // The answer to a `ping`, to the socket that asked.
  z.object({ type: z.literal('pong') }),
  // A peek's state and its answer as it streams, to the app that asked only.
  z.object({
    message: z.string().optional(),
    peekId: z.string(),
    sessionId: z.string(),
    state: PeekStateSchema,
    type: z.literal('peekState'),
  }),
  z.object({ peekId: z.string(), sessionId: z.string(), text: z.string(), type: z.literal('peekDelta') }),
  z.object({ projects: z.array(ProjectSchema), type: z.literal('projects') }),
  // Every terminal the daemon runs: at connection, and again when one opens, ends or closes.
  z.object({ terminals: z.array(TerminalSchema), type: z.literal('terminals') }),
  // The answer to openTerminal and attachTerminal: the terminal, and what it printed so far, as base64.
  z.object({ output: z.string(), terminal: TerminalSchema, type: z.literal('terminalOpened') }),
  // What it printed since, to the sockets showing it, as base64: a terminal's output is bytes, not always UTF-8.
  z.object({ data: z.string(), terminalId: z.string(), type: z.literal('terminalOutput') }),
  // A turn's restore point, a diff a file: what it was before the turn, and what it is now.
  z.object({
    files: z.array(ToolDisplaySchema),
    sessionId: z.string(),
    turnId: z.string(),
    type: z.literal('checkpointDiff'),
  }), // The presets a session can work with: at connection, and again when they change.
  z.object({ choices: PresetChoicesSchema, type: z.literal('presets') }),
  z.object({ hosts: z.array(HostOptionSchema), type: z.literal('hosts') }),
  z.object({ project: ProjectSchema, type: z.literal('projectCreated') }),
  z.object({ message: z.string(), type: z.literal('projectRefused') }),
  // A folder's subfolders by name; no parent at the root, and a problem when the folder cannot be read.
  z.object({
    folders: z.array(z.string()),
    parent: z.string().optional(),
    path: z.string(),
    problem: z.string().optional(),
    type: z.literal('folders'),
  }),
]);
export type ServerMessage = z.infer<typeof ServerMessageSchema>;

/** What `GET /api/status` answers: where the agent works, and whether each llm-server answers. */
export const StatusSchema = z.object({
  // The name of the machine the daemon runs on: the app shows where it works.
  computer: z.string().optional(),
  // The links other machines on the network open, best first; none when the daemon listens on loopback only.
  lanUrls: z.array(z.string()).optional(),
  llama: z.object({
    // Each preset's main model, as its source says now.
    presets: z.array(PresetStatusSchema),
    sources: z.array(SourceStatusSchema),
  }),
  // The address a phone opens, when the daemon was given one (SWCODE_PUBLIC_URL).
  publicUrl: z.string().optional(),
  // How much the main agent of a new session thinks, unless the person picks otherwise.
  thinking: ThinkingModeSchema.optional(),
  // A newer version found on GitHub and not installed yet, for the app to offer on a computer: its number.
  update: z.string().optional(),
  // The build: its version and commit (0.1.0+7bd0cd6), or dev from the sources.
  version: z.string().optional(),
  workspace: z.string(),
});
export type Status = z.infer<typeof StatusSchema>;
