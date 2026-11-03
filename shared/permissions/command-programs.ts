// Marks of a command that does more than run a list of programs: a substitution, a redirection, a subshell, a
// background job or a second line. A rule about programs cannot vouch for those, so they are always asked about.
const OPAQUE_MARKS = ['`', '$(', '>', '<', '(', ')', '\n', '&'];
const CHAINS = /&&|\|\|/g;
const SEPARATORS = /[;|]/;
const WHITESPACE = /\s+/;
const ENVIRONMENT_ASSIGNMENT = /^[A-Za-z_][A-Za-z0-9_]*=/;
const STATEMENT_SEPARATOR = ';';

const programOf = (part: string): string | undefined =>
  part.split(WHITESPACE).find((word) => word !== '' && !ENVIRONMENT_ASSIGNMENT.test(word));

const isDefined = (value: string | undefined): value is string => value !== undefined;

/**
 * The programs a shell command runs, one per part (`npm test && git status` gives npm and git), or `undefined`
 * when the command does more than that list can tell. Quotes are not understood: a separator inside quotes only
 * makes a part that no rule allows, so the command is asked about.
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
