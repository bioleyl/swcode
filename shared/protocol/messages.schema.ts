import { z } from 'zod';

import { SessionDefaultsSchema } from './choices.schema.js';
import { PresetChoicesSchema, PresetStatusSchema, SourceStatusSchema } from './models.schema.js';
import { MAX_PICTURES, PictureUploadSchema } from './picture.schema.js';
import { HostOptionSchema, HostRefSchema, NewFolderNameSchema, ProjectSchema } from './project.schema.js';
import {
  PermissionDecisionSchema,
  PermissionModeSchema,
  SessionEventSchema,
  SessionKindSchema,
  SessionSummarySchema,
  ThinkingModeSchema,
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
 * A session whose conversation says what the palette's search typed: where it says it, a line around the words (the
 * person's message, the model's answer, a summary, or a tool call's file or command).
 */
export const SessionMatchSchema = z.object({ sessionId: z.string(), snippet: z.string() });
export type SessionMatch = z.infer<typeof SessionMatchSchema>;

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
    // The default preset when left out.
    preset: z.string().optional(),
    prompt: z.string().trim().optional(),
    thinking: ThinkingModeSchema.optional(),
    type: z.literal('createSession'),
    workspace: z.string().trim().min(1).optional(),
  }),
  z.object({ sessionId: z.string(), title: z.string().trim().min(1).max(120), type: z.literal('renameSession') }),
  z.object({ sessionId: z.string(), type: z.literal('deleteSession') }),
  z.object({ sessionId: z.string(), type: z.literal('clearQueue') }),
  // The queued messages go to the model at the running turn's next step, without waiting for its end.
  z.object({ sessionId: z.string(), type: z.literal('sendQueueNow') }),
  z.object({ sessionId: z.string(), type: z.literal('openSession') }),
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
  // Turns one of the skills the project's sessions find on or off for them, from their next turn on.
  z.object({
    name: z.string().min(1),
    on: z.boolean(),
    projectId: z.string(),
    type: z.literal('setProjectSkill'),
  }),
  // Where projects can be: this computer, its WSL distributions, the SSH endpoints.
  z.object({ type: z.literal('listHosts') }),
  // The subfolders of a folder, to pick a project's; without a path, of the folder projects usually go in.
  z.object({
    host: HostRefSchema.optional(),
    path: z.string().trim().min(1).optional(),
    type: z.literal('listFolders'),
  }),
]);
export type ClientMessage = z.infer<typeof ClientMessageSchema>;
export type ClientMessageType = ClientMessage['type'];

/** The answer being written right now, so a page opened mid-answer shows what came so far. */
export const StreamingSchema = z.object({ messageId: z.string(), text: z.string() });

/** What the daemon sends the app. Session events go to every open socket; history only to the one asking. */
export const ServerMessageSchema = z.discriminatedUnion('type', [
  z.object({ sessions: z.array(SessionSummarySchema), type: z.literal('sessions') }),
  z.object({ session: SessionSummarySchema, type: z.literal('sessionCreated') }),
  // The answer to a search, the newest sessions first: the query comes back, for the app to drop an answer too late.
  z.object({ matches: z.array(SessionMatchSchema), query: z.string(), type: z.literal('sessionMatches') }),
  z.object({
    events: z.array(SessionEventSchema),
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
  z.object({ message: z.string(), type: z.literal('problem') }),
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
  // The presets a session can work with: at connection, and again when they change.
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
