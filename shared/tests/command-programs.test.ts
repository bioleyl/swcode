import { describe, expect, it } from 'vitest';

import { commandPrograms } from '../permissions/command-programs.js';

describe('commandPrograms', () => {
  it('names the program of a simple command', () => {
    expect(commandPrograms('npm test -- --run')).toEqual(['npm']);
  });

  it('names each program of a chain or a pipe', () => {
    expect(commandPrograms('npm run build && git status; ls | wc -l')).toEqual(['npm', 'git', 'ls', 'wc']);
  });

  it('skips the variables set before the program', () => {
    expect(commandPrograms('CI=1 NODE_ENV=test npx vitest run')).toEqual(['npx']);
  });

  it.each([
    ['a substitution', 'echo $(rm -rf /)'],
    ['backticks', 'echo `whoami`'],
    ['a redirection', 'npm test > out.txt'],
    ['a background job', 'sleep 10 & rm file'],
    ['a subshell', '(cd /tmp && rm x)'],
    ['a second line', 'ls\nrm x'],
  ])('cannot vouch for a command with %s', (_case, command) => {
    expect(commandPrograms(command)).toBeUndefined();
  });

  it('cannot vouch for an empty command', () => {
    expect(commandPrograms('   ')).toBeUndefined();
  });
});
