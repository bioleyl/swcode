import { describe, expect, it } from 'vitest';

import { commandPrograms } from '../permissions/command-programs.js';

describe('commandPrograms', () => {
  it('names the program of a simple command', () => {
    expect(commandPrograms('npm test -- --run')).toEqual(['npm']);
  });

  it('names each program of a chain or a pipe', () => {
    expect(commandPrograms('npm run build && git status; ls | wc -l')).toEqual(['npm', 'git', 'ls', 'wc']);
  });

  it.each([
    ['a variable set for it', 'PATH=.:$PATH ls'],
    ['a library loaded into it', 'LD_PRELOAD=./x.so ls'],
    ['git told what to run', 'git -c core.fsmonitor=./payload.sh status'],
    ['git told where its programs are', 'git --exec-path=. status'],
    ['a package run through npm', 'npm exec -y -- some-package'],
    ['a program that runs another', 'timeout 5 sh ./payload.sh'],
    ['a shell given a script', 'bash ./payload.sh'],
    ['a program run as root', 'sudo rm -rf /'],
    ['a program found and run', 'find . -exec rm {} +'],
  ])('cannot vouch for a program by its name with %s', (_case, command) => {
    expect(commandPrograms(command)).toBeUndefined();
  });

  it('still names git and npm for what they do themselves', () => {
    expect(commandPrograms('git commit -m fix && npm run build')).toEqual(['git', 'npm']);
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
