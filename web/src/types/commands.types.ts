import type { CommandName } from '../lib/commands';

/** What the menu and the help say of a command: what it does, its icon, and the words it takes, if any. */
export interface CommandSpec {
  description: string;
  icon: string;
  /** What follows the command, named: /peek takes a question. A command without one runs alone. */
  argument?: string;
}

/** A command the composer's text is, and what follows it: empty for a command alone. */
export interface CommandLine {
  name: CommandName;
  argument: string;
}

/** A key of the message field, as the help shows it: its keycaps, one per key, and what it does. */
export interface FieldKey {
  keys: string[];
  does: string;
}
