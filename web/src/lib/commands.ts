import type { SessionKind } from '../../../shared/protocol/session.schema';
import type { SkillSummary } from '../../../shared/protocol/skill.schema';
import type { CommandLine, CommandSpec, MenuEntry } from '../types/commands.types';

/** The composer's commands, in the order the menu and the help list them. */
export const COMMAND_NAMES = ['new', 'compact', 'todo', 'peek', 'help'] as const;
export type CommandName = (typeof COMMAND_NAMES)[number];

export const COMMANDS: Readonly<Record<CommandName, CommandSpec>> = {
  compact: { description: 'Summarize the conversation to free context', icon: 'fold-vertical' },
  help: { description: 'Show the commands and keys', icon: 'circle-question' },
  new: { description: 'Start a new session with the same settings', icon: 'square-pen' },
  peek: { argument: 'question', description: 'Ask aside, without interrupting the agent', icon: 'eye' },
  todo: { argument: 'todo', description: 'Add a todo to this project', icon: 'list-checks' },
};

export const COMMAND_PREFIX = '/';
// A whole message that is a command: its name, then what follows it.
const COMMAND_LINE = /^\/([a-z]+)(?:\s+([\s\S]*))?$/i;
// The name being typed: a slash, then no space yet.
const TYPED_NAME = /^\/(\S*)$/;

const isAvailable = (name: string, available: readonly CommandName[]): name is CommandName =>
  available.some((each) => each === name);

/**
 * The command a message is, among those available: a command that takes no words runs alone, so with words after
 * it the message is words; nothing for a message.
 */
export const commandLine = (text: string, available: readonly CommandName[]): CommandLine | undefined => {
  const line = COMMAND_LINE.exec(text.trim());
  const name = (line?.[1] ?? '').toLowerCase();
  if (!isAvailable(name, available)) {
    return undefined;
  }
  const argument = (line?.[2] ?? '').trim();
  return argument === '' || COMMANDS[name].argument !== undefined ? { argument, name } : undefined;
};

/** The commands whose name starts as typed, while the message is a slash and a name: none once a space follows. */
export const commandsMatching = (text: string, available: readonly CommandName[]): CommandName[] => {
  const typed = TYPED_NAME.exec(text)?.[1];
  return typed === undefined ? [] : available.filter((name) => name.startsWith(typed.toLowerCase()));
};

/** The skills whose name starts as typed, as for the commands; one named as a command is left to the command. */
export const skillsMatching = (text: string, skills: readonly SkillSummary[]): SkillSummary[] => {
  const typed = TYPED_NAME.exec(text)?.[1]?.toLowerCase();
  return typed === undefined
    ? []
    : skills.filter(
        (skill) => !isAvailable(skill.name, COMMAND_NAMES) && skill.name.toLowerCase().startsWith(typed)
      );
};

/** The menu's rows for what is typed: the commands available, then the skills. */
export const menuMatching = (
  text: string,
  available: readonly CommandName[],
  skills: readonly SkillSummary[]
): MenuEntry[] => [
  ...commandsMatching(text, available).map((name): MenuEntry => ({ kind: 'command', name })),
  ...skillsMatching(text, skills).map((skill): MenuEntry => ({ kind: 'skill', skill })),
];

/** A message that runs a skill alone. */
export const skillLine = (name: string): string => `${COMMAND_PREFIX}${name}`;

/** A skill as the field holds it once completed: words may follow, for the skill to work on. */
export const skillText = (name: string): string => `${skillLine(name)} `;

/** A command as the field holds it once chosen: followed by a space when words come after it. */
export const commandText = (name: CommandName): string =>
  `${COMMAND_PREFIX}${name}${COMMANDS[name].argument === undefined ? '' : ' '}`;

/** The row a step up or down the menu lands on, round from the last to the first. */
export const stepIn = (active: number, step: number, count: number): number =>
  count === 0 ? 0 : (active + step + count) % count;

/** The commands of a session, by its kind: a chat has no /peek, its turns are short. */
export const SESSION_COMMANDS: Readonly<Record<SessionKind, readonly CommandName[]>> = {
  chat: ['new', 'compact', 'help'],
  code: COMMAND_NAMES,
};
