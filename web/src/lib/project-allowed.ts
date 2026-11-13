import type { AllowedRule, Project } from '../../../shared/protocol/project.schema';
import type { AllowedRow } from '../types/project-allowed.types';

/** What each kind of rule lets through, under the name it is about: a new kind does not compile until it is here. */
export const ALLOWED_RULE_LABELS: Readonly<Record<AllowedRule, { icon: 'terminal' | 'plug'; says: string }>> = {
  outside: { icon: 'terminal', says: 'A program, outside the sandbox too' },
  program: { icon: 'terminal', says: 'A program' },
  server: { icon: 'plug', says: 'Every tool of this MCP server' },
};

/** What a project's sessions do without asking, a row each: programs, programs outside the sandbox, then servers. */
export const allowedRows = (project: Project): AllowedRow[] => {
  const allowed = project.allowed;
  if (!allowed) {
    return [];
  }
  const rows = (rule: AllowedRule, names: readonly string[]): AllowedRow[] =>
    names.map((name) => ({ name, rule }));
  return [
    ...rows('program', allowed.programs),
    ...rows('outside', allowed.outside),
    ...rows('server', allowed.servers),
  ];
};
