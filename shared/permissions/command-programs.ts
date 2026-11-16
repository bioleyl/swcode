// The marks of a command that does more than run a list of programs, outside its quotes: a substitution, a
// redirection, a subshell, a background job or a second line. A rule about programs cannot vouch for those, so they
// are always asked about.
const OPAQUE_MARKS: ReadonlySet<string> = new Set(['`', '>', '<', '(', ')', '\n']);
const SINGLE_QUOTE = "'";
const DOUBLE_QUOTE = '"';
const BACKSLASH = '\\';
const WHITESPACE = /\s+/;
const ENVIRONMENT_ASSIGNMENT = /^[A-Za-z_][A-Za-z0-9_]*=/;
// ssh's options whose value is a word of its own (`-p 2222`): skipping the option alone would make its value look
// like the host.
const SSH_OPTIONS_WITH_A_WORD: ReadonlySet<string> = new Set([
  'b',
  'c',
  'D',
  'E',
  'e',
  'F',
  'I',
  'i',
  'J',
  'L',
  'l',
  'm',
  'O',
  'o',
  'p',
  'Q',
  'R',
  'S',
  'W',
  'w',
]);
// Programs that run what their words say rather than a thing of their own: allowing one by its name would allow
// anything after it.
const RUNNERS: ReadonlySet<string> = new Set([
  'bash',
  'bunx',
  'command',
  'dash',
  'doas',
  'env',
  'eval',
  'exec',
  'find',
  'fish',
  'nice',
  'nohup',
  'npx',
  'parallel',
  'pnpx',
  'sh',
  'ssh',
  'sudo',
  'time',
  'timeout',
  'watch',
  'xargs',
  'zsh',
]);
// The words that make a program named for itself run something else: git's own settings name programs it runs, and
// npm runs any package's.
const RUNNING_WORDS: Readonly<Record<string, readonly string[]>> = {
  git: ['-c', '--config-env', '--exec-path'],
  npm: ['exec', 'x'],
};

const runsItsWords = (program: string, words: readonly string[]): boolean =>
  RUNNERS.has(program)
  || (RUNNING_WORDS[program] ?? []).some((marker) =>
    words.some((word) => word === marker || word.startsWith(`${marker}=`))
  );

/**
 * What an ssh command reaches: its host, which is the unit a person allows, since what runs there is that host's own
 * shell and no list of programs can tell it. `ssh marvin 'cd x && ls'` names `ssh marvin`.
 */
const sshTarget = (words: readonly string[]): string | undefined => {
  if (words[0] !== 'ssh') {
    return undefined;
  }
  for (let index = 1; index < words.length; index += 1) {
    const word = words[index] ?? '';
    if (word === '--') {
      continue;
    }
    if (word.startsWith('-') && word.length > 1) {
      if (word.length === 2 && SSH_OPTIONS_WITH_A_WORD.has(word.slice(1))) {
        index += 1;
      }
      continue;
    }
    return `ssh ${word}`;
  }
  return undefined;
};

/**
 * The program a part of a command runs, when its name says what runs: not after a variable set for it (PATH,
 * LD_PRELOAD and git's own change what a name means), and not a program that runs what its words say, unless its
 * words name the host it reaches.
 */
const programOf = (part: string): string | undefined => {
  const [program, ...words] = part.split(WHITESPACE).filter((word) => word !== '');
  if (program === undefined || ENVIRONMENT_ASSIGNMENT.test(program)) {
    return undefined;
  }
  return runsItsWords(program, words) ? sshTarget([program, ...words]) : program;
};

const isDefined = (value: string | undefined): value is string => value !== undefined;

/**
 * The statements of a command, split at its separators (`;`, `|`, `&&`, `||`) and not inside its quotes, or
 * `undefined` when the command does more than a list of programs can tell: a substitution the shell runs (not one in
 * single quotes, which quotes it), a redirection, a subshell, a background job or a second line. A separator inside
 * quotes belongs to the argument, so `ssh marvin 'cd x && ls'` is one statement.
 */
const statementsOf = (command: string): string[] | undefined => {
  const statements: string[] = [];
  let current = '';
  let quote: string | undefined;
  const flush = (): void => {
    const part = current.trim();
    if (part !== '') {
      statements.push(part);
    }
    current = '';
  };
  for (let index = 0; index < command.length; index += 1) {
    const char = command[index] ?? '';
    const next = command[index + 1];
    if (quote === SINGLE_QUOTE) {
      if (char === SINGLE_QUOTE) {
        quote = undefined;
      } else {
        current += char;
      }
      continue;
    }
    if (quote === DOUBLE_QUOTE) {
      if (char === DOUBLE_QUOTE) {
        quote = undefined;
      } else if (char === BACKSLASH && next !== undefined) {
        index += 1;
        current += next;
      } else if (char === '`' || (char === '$' && next === '(')) {
        return undefined;
      } else {
        current += char;
      }
      continue;
    }
    if (char === SINGLE_QUOTE || char === DOUBLE_QUOTE) {
      quote = char;
    } else if (char === BACKSLASH && next !== undefined) {
      index += 1;
      current += next;
    } else if (char === ';' || char === '|') {
      flush();
    } else if (char === '&') {
      if (next !== '&') {
        return undefined;
      }
      index += 1;
      flush();
    } else if (char === '$' && next === '(') {
      return undefined;
    } else if (OPAQUE_MARKS.has(char)) {
      return undefined;
    } else {
      current += char;
    }
  }
  if (quote !== undefined) {
    return undefined;
  }
  flush();
  return statements;
};

/**
 * The programs a shell command runs, one per statement (`npm test && git status` gives npm and git), or `undefined`
 * when the command does more than that list can tell: it sets a variable first, one of its programs runs what its
 * words say (except ssh, which is named by the host it reaches), or it does something a list of programs cannot
 * describe. daemon/permissions decides with the same rule, in Go: the tests of both hold the same cases.
 */
export function commandPrograms(command: string): string[] | undefined {
  const statements = statementsOf(command);
  if (statements === undefined) {
    return undefined;
  }
  const programs = statements.map(programOf);
  return programs.length > 0 && programs.every(isDefined) ? programs : undefined;
}
