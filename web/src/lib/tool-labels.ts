import { commandPrograms } from '../../../shared/permissions/command-programs';

import type { ToolName } from '../../../shared/protocol/session.schema';
import type { ToolLabel } from '../types/labels.types';

const TOOL_LABELS: Readonly<Record<ToolName, ToolLabel>> = {
  agent: {
    doing: 'Running an agent:',
    icon: 'split',
    name: 'Agent',
    question: 'Start this agent?',
    writing: 'Briefing an agent',
  },
  bash: {
    doing: 'Running',
    icon: 'terminal',
    name: 'Bash',
    question: 'Run this command?',
    writing: 'Writing a command',
  },
  closetodo: {
    doing: 'Closing',
    icon: 'check',
    name: 'Close todo',
    question: 'Close this todo?',
    writing: 'Closing a todo',
  },
  draw: {
    doing: 'Drawing',
    icon: 'image',
    name: 'Draw',
    question: 'Draw this picture?',
    writing: 'Describing a picture',
  },
  edit: {
    doing: 'Editing',
    icon: 'pencil',
    name: 'Edit',
    question: 'Edit this file?',
    writing: 'Writing an edit',
  },
  glob: {
    doing: 'Listing',
    icon: 'folder-search',
    name: 'Glob',
    question: 'List these files?',
    writing: 'Listing files',
  },
  grep: {
    doing: 'Searching for',
    icon: 'search',
    name: 'Grep',
    question: 'Search these files?',
    writing: 'Searching the files',
  },
  image: {
    doing: 'Making',
    icon: 'image',
    name: 'Image',
    question: 'Make this picture?',
    writing: 'Describing a picture',
  },
  kill: {
    doing: 'Stopping',
    icon: 'stop',
    name: 'Kill',
    question: 'Stop this command?',
    writing: 'Stopping a command',
  },
  notes: {
    doing: 'Noting',
    icon: 'book-plus',
    name: 'Notes',
    question: 'Add these notes to AGENTS.md?',
    writing: 'Writing notes',
  },
  output: {
    doing: 'Reading the output of',
    icon: 'terminal',
    name: 'Output',
    question: 'Read this output?',
    writing: 'Reading a command’s output',
  },
  plan: {
    doing: 'Updating the plan:',
    icon: 'list-checks',
    name: 'Plan',
    question: 'Update the plan?',
    writing: 'Updating the plan',
  },
  question: {
    doing: 'Asking:',
    icon: 'circle-question',
    name: 'Question',
    question: 'Ask these questions?',
    writing: 'Writing questions',
  },
  read: {
    doing: 'Reading',
    icon: 'file-text',
    name: 'Read',
    question: 'Read this file?',
    writing: 'Reading a file',
  },
  readterminal: {
    doing: 'Reading the terminal',
    icon: 'terminal',
    name: 'Terminal output',
    question: 'Read this terminal?',
    writing: 'Reading a terminal’s output',
  },
  saveskill: {
    doing: 'Saving the skill',
    icon: 'book-plus',
    name: 'Save skill',
    question: 'Save this skill?',
    writing: 'Writing a skill',
  },
  screenshot: {
    doing: 'Looking at',
    icon: 'camera',
    name: 'Screenshot',
    question: 'Look at this page?',
    writing: 'Looking at a page',
  },
  skill: {
    doing: 'Loading the skill',
    icon: 'book-open',
    name: 'Skill',
    question: 'Load this skill?',
    writing: 'Loading a skill',
  },
  terminal: {
    doing: 'Running in a terminal',
    icon: 'terminal',
    name: 'Terminal',
    question: 'Run this in a terminal?',
    writing: 'Opening a terminal',
  },
  todo: {
    doing: 'Opening',
    icon: 'list-checks',
    name: 'Todo',
    question: 'Open this todo?',
    writing: 'Opening a todo',
  },
  webfetch: {
    doing: 'Reading',
    icon: 'link',
    name: 'Fetch',
    question: 'Read this page?',
    writing: 'Reading a web page',
  },
  websearch: {
    doing: 'Searching the web for',
    icon: 'globe',
    name: 'Search',
    question: 'Search the web?',
    writing: 'Searching the web',
  },
  write: {
    doing: 'Writing',
    icon: 'file-plus',
    name: 'Write',
    question: 'Write this file?',
    writing: 'Writing a file',
  },
};

const UNKNOWN_TOOL: ToolLabel = {
  doing: 'Calling',
  icon: 'circle-alert',
  name: 'Tool',
  question: 'Run this tool?',
  writing: 'Calling a tool',
};

export const toolLabel = (tool: ToolName | undefined): ToolLabel => (tool ? TOOL_LABELS[tool] : UNKNOWN_TOOL);

const ALL_EDITS = 'Allow all edits in this session';
const never = (): undefined => undefined;

const programsGrant = (command: string): string | undefined => {
  const programs = commandPrograms(command);
  return programs ? `Always allow ${[...new Set(programs)].join(', ')} in this session` : undefined;
};

// What "for the rest of the session" covers, as the daemon grants it (`grantForSession`).
const SESSION_GRANTS: Readonly<Record<ToolName, (summary: string) => string | undefined>> = {
  agent: never,
  bash: programsGrant,
  closetodo: never,
  draw: never,
  edit: () => ALL_EDITS,
  glob: never,
  grep: never,
  image: () => ALL_EDITS,
  kill: never,
  notes: () => ALL_EDITS,
  output: never,
  plan: never,
  question: never,
  read: never,
  readterminal: never,
  saveskill: () => ALL_EDITS,
  screenshot: never,
  skill: never,
  terminal: programsGrant,
  todo: never,
  webfetch: never,
  websearch: never,
  write: () => ALL_EDITS,
};

/** The label of the "for this session" button, or `undefined` when such a grant would not cover the call. */
export const sessionGrantLabel = (tool: ToolName | undefined, summary: string): string | undefined =>
  tool ? SESSION_GRANTS[tool](summary) : undefined;
