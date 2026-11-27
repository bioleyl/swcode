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
    ['ssh without a host', 'ssh'],
    ['ssh with options and no host', 'ssh -o BatchMode=yes'],
  ])('cannot vouch for a program by its name with %s', (_case, command) => {
    expect(commandPrograms(command)).toBeUndefined();
  });

  it('still names git and npm for what they do themselves', () => {
    expect(commandPrograms('git commit -m fix && npm run build')).toEqual(['git', 'npm']);
  });

  it('reads a separator inside quotes as the argument', () => {
    expect(commandPrograms("git commit -m 'fix; tests' && npm run build")).toEqual(['git', 'npm']);
  });

  it('names ssh by the host it reaches', () => {
    expect(
      commandPrograms(
        "ssh marvin 'cd ~/tts/kokoro && curl -sL -o model.onnx https://example.com/model.onnx && ls -lh'"
      )
    ).toEqual(['ssh marvin']);
    expect(commandPrograms('ssh -p 2222 marvin ls')).toEqual(['ssh marvin']);
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

  it.each([
    ['a loop', 'for f in *.go; do gofmt -l $f; done'],
    ['a condition', 'if true; then rm -rf x; fi'],
    ['a negation', '! rm x'],
    ['a group', '{ rm x; }'],
    ['a file of commands', 'source ./payload.sh'],
    ['a file of commands by its dot', '. ./payload.sh'],
    ['a builtin named', 'builtin eval ls'],
  ])('cannot vouch for the shell’s own words: %s', (_case, command) => {
    expect(commandPrograms(command)).toBeUndefined();
  });

  it.each([
    ['a quote that keeps escaped quotes', "ls $'\\'' ; rm -rf x #'"],
    ['a comment hiding a line break', "ls #'\nrm -rf x #'"],
    ['a comment with a quote in it', "ls # '; rm x"],
    ['a translated string', 'echo $"a"'],
  ])('cannot vouch for quotes bash ends elsewhere: %s', (_case, command) => {
    expect(commandPrograms(command)).toBeUndefined();
  });

  it('reads a # inside a word as the word', () => {
    expect(commandPrograms('gh issue view repo#12')).toEqual(['gh']);
  });

  it('cannot vouch for an empty command', () => {
    expect(commandPrograms('   ')).toBeUndefined();
  });
});
