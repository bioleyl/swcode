import { describe, expect, it } from 'vitest';

import { absolutePath, pathOfCall } from '../src/lib/ide-open';

describe('a tool’s path, as the editor opens it', () => {
  it('joins a relative path onto the project’s folder', () => {
    expect(absolutePath('/home/dev/app', 'src/main.ts')).toBe('/home/dev/app/src/main.ts');
  });

  it('keeps the folder’s separator, so a Windows project opens with backslashes', () => {
    expect(absolutePath('C:\\dev\\app', 'src\\main.ts')).toBe('C:\\dev\\app\\src\\main.ts');
  });

  it('does not double the separator between the two halves', () => {
    expect(absolutePath('/home/dev/app/', 'src/main.ts')).toBe('/home/dev/app/src/main.ts');
  });

  it('keeps a path that is already whole: the scratch folder, a drive elsewhere', () => {
    expect(absolutePath('/home/dev/app', '/tmp/swcode/scratch/probe.ts')).toBe('/tmp/swcode/scratch/probe.ts');
    expect(absolutePath('C:\\dev\\app', 'D:\\notes\\plan.md')).toBe('D:\\notes\\plan.md');
  });

  it('gives the folder alone for an empty path, and the path alone for no folder', () => {
    expect(absolutePath('/home/dev/app', '')).toBe('/home/dev/app');
    expect(absolutePath('', 'src/main.ts')).toBe('src/main.ts');
  });
});

describe('the file a call concerns', () => {
  const Diff = { added: 1, kind: 'diff' as const, lines: [], path: 'src/greet.ts', removed: 0 };
  const Text = { kind: 'text' as const, text: 'the file' };

  it('takes the change’s path, which wins over the summary', () => {
    expect(pathOfCall('write', 'src/greet.ts (3 lines)', Diff)).toBe('src/greet.ts');
  });

  it('takes the summary of the file tools, whose few words are the path', () => {
    expect(pathOfCall('read', 'src/greet.ts', Text)).toBe('src/greet.ts');
    expect(pathOfCall('write', 'src/greet.ts', undefined)).toBe('src/greet.ts');
    expect(pathOfCall('edit', 'src/greet.ts', Diff)).toBe('src/greet.ts');
  });

  it('names no file for a call that works on something else', () => {
    expect(pathOfCall('bash', 'npm test', Text)).toBe('');
    expect(pathOfCall(undefined, '', undefined)).toBe('');
  });
});
