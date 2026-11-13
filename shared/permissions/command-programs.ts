// Marks of a command that does more than run a list of programs: a substitution, a redirection, a subshell, a
// background job or a second line. A rule about programs cannot vouch for those, so they are always asked about.
const OPAQUE_MARKS = ['`', '$(', '>', '<', '(', ')', '\n', '&'];
const CHAINS = /&&|\|\|/g;
const SEPARATORS = /[;|]/;
const WHITESPACE = /\s+/;
const ENVIRONMENT_ASSIGNMENT = /^[A-Za-z_][A-Za-z0-9_]*=/;
const STATEMENT_SEPARATOR = ';';
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
 * The program a part of a command runs, when its name says what runs: not after a variable set for it (PATH,
 * LD_PRELOAD and git's own change what a name means), and not a program that runs what its words say.
 */
const programOf = (part: string): string | undefined => {
  const [program, ...words] = part.split(WHITESPACE).filter((word) => word !== '');
  if (program === undefined || ENVIRONMENT_ASSIGNMENT.test(program) || runsItsWords(program, words)) {
    return undefined;
  }
  return program;
};

const isDefined = (value: string | undefined): value is string => value !== undefined;

/**
 * The programs a shell command runs, one per part (`npm test && git status` gives npm and git), or `undefined`
 * when the command does more than that list can tell: it sets a variable first, or one of its programs runs what
 * its words say. Quotes are not understood: a separator inside quotes only makes a part that no rule allows, so the
 * command is asked about. daemon/permissions decides with the same rule, in Go: the tests of both hold the same
 * cases.
 */
export function commandPrograms(command: string): string[] | undefined {
  const statements = command.replace(CHAINS, STATEMENT_SEPARATOR);
  if (OPAQUE_MARKS.some((mark) => statements.includes(mark))) {
    return undefined;
  }
  const programs = statements
    .split(SEPARATORS)
    .map((part) => part.trim())
    .filter((part) => part !== '')
    .map(programOf);
  return programs.length > 0 && programs.every(isDefined) ? programs : undefined;
}
