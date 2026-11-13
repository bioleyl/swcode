import { chmodSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

import { afterAll, describe, expect, it } from 'vitest';

import {
  checkPasses,
  makeWorkspace,
  onlyOf,
  readTasks,
  runFinishedOf,
  runTask,
  tableOf,
  totalsOf,
} from '../../scripts/eval';

import type { EvalResult, EvalTask } from '../../scripts/eval';

// On the disk beside the tasks, as the runner's own workspaces are.
const WORK = resolve(import.meta.dirname, '../.work', `test-${process.pid}`);
const TASK_COUNT = 12;
const EXECUTABLE = 0o755;

afterAll(() => rmSync(WORK, { force: true, recursive: true }));

const result = (changes: Partial<EvalResult>): EvalResult => ({
  completionTokens: 80,
  outcome: 'done',
  promptTokens: 1200,
  seconds: 4.5,
  steps: 3,
  success: true,
  task: 'off-by-one',
  toolCalls: 2,
  ...changes,
});

/** A way to do each task, as the files it leaves: every check must pass on it, so no check asks the impossible. */
const SOLUTIONS: Readonly<Record<string, Readonly<Record<string, string>>>> = {
  'add-function': {
    'strings.mjs':
      "export const capitalize = (text) => (text === '' ? '' : text[0].toUpperCase() + text.slice(1));\nexport const slugify = (text) => text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');\n",
  },
  'find-and-report': { 'answer.txt': 'src/net/limits.mjs\n4500\n' },
  'fix-failing-function': {
    'sum.mjs': 'export const sum = (numbers) => numbers.reduce((total, number) => total + number, 0);\n',
  },
  'fix-json-config': {
    'config.json': '{ "name": "shop", "port": 8080, "debug": false, "features": ["search", "export"] }\n',
  },
  'fix-shell-script': {
    'count.sh':
      '#!/bin/sh\ntotal=0\nfor file in "$@"; do\n  lines=$(wc -l < "$file")\n  total=$((total + lines))\ndone\necho "$total"\n',
  },
  'multi-file-refactor': {
    'src/dates.mjs':
      "export const formatDate = (iso) => {\n  const [year, month, day] = iso.split('-');\n  return `${day}.${month}.${year}`;\n};\n",
    'src/orders.mjs':
      "import { formatDate } from './dates.mjs';\n\nexport const orderLine = (order) => `#${order.id} on ${formatDate(order.placed)}`;\n",
    'src/users.mjs':
      "import { formatDate } from './dates.mjs';\n\nexport const userLine = (user) => `${user.name}, since ${formatDate(user.joined)}`;\n",
  },
  'off-by-one': {
    'paginate.mjs':
      'export const page = (items, number, size) => items.slice((number - 1) * size, number * size);\nexport const pageCount = (count, size) => Math.ceil(count / size);\n',
  },
  'remove-dead-code': {
    'shapes.mjs':
      'const double = (value) => value * 2;\n\nexport const area = (shape) => shape.w * shape.h;\n\nexport const perimeter = (shape) => double(shape.w + shape.h);\n',
  },
  'rename-symbol': {
    'src/cart.mjs':
      "import { priceWithTax } from './price.mjs';\n\nexport const cartTotal = (items) => items.reduce((total, item) => total + priceWithTax(item.price), 0);\n",
    'src/price.mjs': 'export const priceWithTax = (price) => Math.round(price * 1.08);\n',
    'src/report.mjs':
      "import { priceWithTax } from './price.mjs';\n\nexport const reportLine = (item) => `${item.name}: ${priceWithTax(item.price)}`;\n",
  },
  'update-readme': {
    'README.md':
      '# serve\n\n## Install\n\nCopy `cli.mjs` somewhere on your PATH.\n\n## Options\n\n| Flag | Default |\n| --- | --- |\n| `--port` | `8080` |\n| `--host` | `127.0.0.1` |\n| `--quiet` | `false` |\n',
  },
  'write-test': {
    'clamp.test.mjs':
      "import assert from 'node:assert/strict';\nimport { test } from 'node:test';\n\nimport { clamp } from './clamp.mjs';\n\ntest('under', () => assert.equal(clamp(-1, 0, 3), 0));\ntest('inside', () => assert.equal(clamp(2, 0, 3), 2));\ntest('over', () => assert.equal(clamp(5, 0, 3), 3));\n",
  },
  'yaml-indentation': {
    'deploy.yml':
      'services:\n  web:\n    image: shop:1.4\n    ports:\n      - "8080:80"\n  worker:\n    image: shop-worker:1.4\n    environment:\n      QUEUE: jobs\n    restart: always\n',
  },
};

describe('what a run printed', () => {
  it('is read from its last runFinished line, the other lines left aside', () => {
    const output = [
      '{"kind":"toolCall","tool":"bash"}',
      'a line that is not JSON',
      '{"kind":"runFinished","outcome":"failed","steps":1,"toolCalls":0,"promptTokens":10,"completionTokens":2,"seconds":0.5}',
      '{"kind":"runFinished","outcome":"done","sessionId":"s1","steps":3,"toolCalls":2,"promptTokens":1200,"completionTokens":80,"seconds":4.5}',
      '',
    ].join('\n');
    expect(runFinishedOf(output)).toEqual({
      completionTokens: 80,
      outcome: 'done',
      promptTokens: 1200,
      seconds: 4.5,
      steps: 3,
      toolCalls: 2,
    });
  });

  it('is nothing when the program ended without saying, and a failure when it says something else', () => {
    expect(runFinishedOf('{"kind":"toolCall"}\nKilled\n')).toBeUndefined();
    expect(runFinishedOf('')).toBeUndefined();
    expect(runFinishedOf('{"kind":"runFinished","outcome":"exploded","steps":"many"}')).toEqual({
      completionTokens: 0,
      outcome: 'failed',
      promptTokens: 0,
      seconds: 0,
      steps: 0,
      toolCalls: 0,
    });
  });
});

describe('the scores', () => {
  const results = [
    result({}),
    result({ outcome: 'stopped', promptTokens: 30_000, seconds: 120, steps: 40, success: false, task: 'rename-symbol' }),
  ];

  it('add up', () => {
    expect(totalsOf(results)).toEqual({
      completionTokens: 160,
      promptTokens: 31_200,
      seconds: 124.5,
      steps: 43,
      successes: 1,
      tasks: 2,
      toolCalls: 4,
    });
    expect(totalsOf([])).toMatchObject({ successes: 0, tasks: 0 });
  });

  it('make a table of aligned columns, a line a task and the totals', () => {
    expect(tableOf(results).split('\n')).toEqual([
      'task           ok   outcome  steps  calls  prompt tok  completion tok  seconds',
      'off-by-one     yes  done         3      2        1200              80      4.5',
      'rename-symbol  no   stopped     40      2       30000              80    120.0',
      'total          1/2              43      4       31200             160    124.5',
    ]);
  });

  it('are asked for one task or several with --only', () => {
    expect(onlyOf(['--only', 'off-by-one', '--only', 'write-test'])).toEqual(['off-by-one', 'write-test']);
    expect(onlyOf([])).toEqual([]);
    expect(onlyOf(['--only'])).toEqual([]);
  });
});

describe('the tasks', () => {
  const tasks = readTasks();

  it('are a dozen, each with a prompt, a check and the time it has', () => {
    expect(tasks).toHaveLength(TASK_COUNT);
    expect(Object.keys(SOLUTIONS).sort()).toEqual(tasks.map((task) => task.name));
    for (const task of tasks) {
      expect(task.prompt.length).toBeGreaterThan(0);
      expect(task.timeoutSeconds).toBeGreaterThan(0);
      expect(existsSync(task.files)).toBe(true);
    }
  });

  it.each(tasks.map((task) => [task.name, task] as const))(
    '%s: its check fails on the starting files, and passes once the task is done',
    (name, task) => {
      const workspace = makeWorkspace(task, join(WORK, 'tasks'));
      expect(checkPasses(task, workspace)).toBe(false);
      for (const [path, text] of Object.entries(SOLUTIONS[name] ?? {})) {
        mkdirSync(dirname(join(workspace, path)), { recursive: true });
        writeFileSync(join(workspace, path), text);
      }
      expect(checkPasses(task, workspace)).toBe(true);
    }
  );
});

describe('the runner', () => {
  const task: EvalTask = {
    check: 'grep -q fixed out.txt',
    files: join(WORK, 'no-files'),
    name: 'fake-task',
    prompt: 'Write fixed in out.txt',
    timeoutSeconds: 20,
  };

  /** A program that takes what `swcode run` takes, does the task and says what it cost. */
  const fakeSwcode = (lastLine: string): string => {
    mkdirSync(WORK, { recursive: true });
    const program = join(WORK, `fake-swcode-${lastLine.length}`);
    writeFileSync(
      program,
      [
        '#!/bin/sh',
        'printf "%s\\n" "$@" > "$0.args"',
        'while [ $# -gt 0 ]; do',
        '  case "$1" in --cwd) cwd=$2; shift ;; esac',
        '  shift',
        'done',
        'echo fixed > "$cwd/out.txt"',
        `echo '{"kind":"toolCall","tool":"write"}'`,
        `echo '${lastLine}'`,
        '',
      ].join('\n')
    );
    chmodSync(program, EXECUTABLE);
    return program;
  };

  it('gives the task to the program in a fresh workspace, then checks what it left there', () => {
    const bin = fakeSwcode(
      '{"kind":"runFinished","outcome":"done","sessionId":"s1","steps":3,"toolCalls":2,"promptTokens":1200,"completionTokens":80,"seconds":4.5}'
    );
    const workFolder = join(WORK, 'runs');
    expect(runTask(task, { bin, workFolder })).toEqual({
      completionTokens: 80,
      outcome: 'done',
      promptTokens: 1200,
      seconds: 4.5,
      steps: 3,
      success: true,
      task: 'fake-task',
      toolCalls: 2,
    });
    const workspace = join(workFolder, 'fake-task');
    expect(readFileSync(`${bin}.args`, 'utf8').trim().split('\n')).toEqual([
      'run',
      '--json',
      '--cwd',
      workspace,
      '--timeout',
      '20s',
      'Write fixed in out.txt',
    ]);
    // The workspace is a repository of its own, the task's files committed.
    expect(existsSync(join(workspace, '.git'))).toBe(true);
  });

  it('counts a program that ended without its last line as a failure, whatever its files say', () => {
    const silent = runTask(task, { bin: fakeSwcode('not the last line'), workFolder: join(WORK, 'silent') });
    expect(silent).toMatchObject({ outcome: 'failed', steps: 0, success: true, task: 'fake-task' });
    const missing = runTask(task, { bin: join(WORK, 'no-such-program'), workFolder: join(WORK, 'missing') });
    expect(missing).toMatchObject({ outcome: 'failed', promptTokens: 0, success: false });
  });
});
