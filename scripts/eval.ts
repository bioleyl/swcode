// npm run eval: the tasks of evals/tasks, each given to `swcode run` in a workspace of its own and scored on
// success (the task's check passes), steps, tokens and time, to judge a change of prompt or tool against the
// person's own llm-server. `--only <name>` runs one task (it may be given several times); SWCODE_BIN names the
// program, `swcode` on the PATH otherwise.
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const EVALS = resolve(import.meta.dirname, '../evals');
/** Where the tasks are, a folder each. */
export const TASKS_FOLDER = join(EVALS, 'tasks');
// On the disk beside the tasks, not in /tmp: a temporary folder may be memory.
const WORK_FOLDER = join(EVALS, '.work');
const RESULTS_FOLDER = join(EVALS, 'results');
const TASK_FILE = 'task.json';
const FILES_FOLDER = 'files';
const DEFAULT_BIN = 'swcode';
const BIN_VARIABLE = 'SWCODE_BIN';
const ONLY_FLAG = '--only';
const RUN_FINISHED = 'runFinished';
const MS_PER_SECOND = 1000;
// What the program gets on top of the task's own time, to end the run by itself and say how it went.
const GRACE_SECONDS = 30;
const CHECK_SECONDS = 60;
const MAX_OUTPUT_BYTES = 256 * 1024 * 1024;
const SUCCESS = 0;
const FAILURE = 1;
const COMMITTER = ['-c', 'user.name=swcode eval', '-c', 'user.email=eval@localhost', '-c', 'commit.gpgsign=false'];

/** How a run ended, as `swcode run --json` says on its last line. */
export type RunOutcome = 'done' | 'failed' | 'stopped';
const OUTCOMES: readonly RunOutcome[] = ['done', 'failed', 'stopped'];

/** A task: what the agent is asked, the files it starts from, and the command that says whether it did it. */
export interface EvalTask {
  name: string;
  prompt: string;
  /** A shell command run in the workspace once the agent is done: exit 0 means the task is done. */
  check: string;
  timeoutSeconds: number;
  /** The folder of the workspace's starting files. */
  files: string;
}

/** What a run cost, as its last line says. */
export interface RunFinished {
  outcome: RunOutcome;
  steps: number;
  toolCalls: number;
  promptTokens: number;
  completionTokens: number;
  seconds: number;
}

/** A task's score. */
export interface EvalResult extends RunFinished {
  task: string;
  success: boolean;
}

/** Every score together. */
export interface EvalTotals {
  tasks: number;
  successes: number;
  steps: number;
  toolCalls: number;
  promptTokens: number;
  completionTokens: number;
  seconds: number;
}

/** Where the runner finds the program and puts the workspaces. */
export interface RunOptions {
  bin: string;
  workFolder: string;
}

const numberOf = (value: unknown): number => (typeof value === 'number' && Number.isFinite(value) ? value : 0);

const outcomeOf = (value: unknown): RunOutcome =>
  OUTCOMES.find((outcome) => outcome === value) ?? 'failed';

const objectOf = (line: string): Record<string, unknown> | undefined => {
  try {
    const parsed: unknown = JSON.parse(line);
    return typeof parsed === 'object' && parsed !== null ? (parsed as Record<string, unknown>) : undefined;
  } catch {
    return undefined;
  }
};

/**
 * What a run cost, from what `swcode run --json` printed: its last `runFinished` line, the other lines and anything
 * that is not JSON left aside. Nothing when the program ended without saying.
 */
export const runFinishedOf = (output: string): RunFinished | undefined => {
  const last = output
    .split('\n')
    .map((line) => objectOf(line.trim()))
    .findLast((line) => line?.kind === RUN_FINISHED);
  if (!last) {
    return undefined;
  }
  return {
    completionTokens: numberOf(last.completionTokens),
    outcome: outcomeOf(last.outcome),
    promptTokens: numberOf(last.promptTokens),
    seconds: numberOf(last.seconds),
    steps: numberOf(last.steps),
    toolCalls: numberOf(last.toolCalls),
  };
};

/** The scores added up. */
export const totalsOf = (results: readonly EvalResult[]): EvalTotals =>
  results.reduce<EvalTotals>(
    (totals, result) => ({
      completionTokens: totals.completionTokens + result.completionTokens,
      promptTokens: totals.promptTokens + result.promptTokens,
      seconds: totals.seconds + result.seconds,
      steps: totals.steps + result.steps,
      successes: totals.successes + (result.success ? 1 : 0),
      tasks: totals.tasks + 1,
      toolCalls: totals.toolCalls + result.toolCalls,
    }),
    { completionTokens: 0, promptTokens: 0, seconds: 0, steps: 0, successes: 0, tasks: 0, toolCalls: 0 }
  );

const HEADERS = ['task', 'ok', 'outcome', 'steps', 'calls', 'prompt tok', 'completion tok', 'seconds'] as const;
// The first three columns are words, read from the left; the others are numbers, read from the right.
const WORD_COLUMNS = 3;
const SECONDS_DIGITS = 1;
const COLUMN_GAP = '  ';
const TOTAL_LABEL = 'total';
const NO_OUTCOME = '';

const rowOf = (result: EvalResult): string[] => [
  result.task,
  result.success ? 'yes' : 'no',
  result.outcome,
  String(result.steps),
  String(result.toolCalls),
  String(result.promptTokens),
  String(result.completionTokens),
  result.seconds.toFixed(SECONDS_DIGITS),
];

const totalsRow = (totals: EvalTotals): string[] => [
  TOTAL_LABEL,
  `${totals.successes}/${totals.tasks}`,
  NO_OUTCOME,
  String(totals.steps),
  String(totals.toolCalls),
  String(totals.promptTokens),
  String(totals.completionTokens),
  totals.seconds.toFixed(SECONDS_DIGITS),
];

/** The scores as a table in aligned columns, a line a task, then the totals. */
export const tableOf = (results: readonly EvalResult[]): string => {
  const rows = [[...HEADERS], ...results.map(rowOf), totalsRow(totalsOf(results))];
  const widths = HEADERS.map((_, column) => Math.max(...rows.map((row) => (row[column] ?? '').length)));
  return rows
    .map((row) =>
      row
        .map((cell, column) =>
          column < WORD_COLUMNS ? cell.padEnd(widths[column] ?? 0) : cell.padStart(widths[column] ?? 0)
        )
        .join(COLUMN_GAP)
        .trimEnd()
    )
    .join('\n');
};

const textOf = (value: unknown, name: string, field: string): string => {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`${name}: ${TASK_FILE} needs a ${field}`);
  }
  return value;
};

/** The tasks of a folder, by name: each a folder with its task.json and its files. */
export const readTasks = (folder: string = TASKS_FOLDER): EvalTask[] =>
  readdirSync(folder, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && existsSync(join(folder, entry.name, TASK_FILE)))
    .map((entry) => entry.name)
    .sort()
    .map((name) => {
      const task = objectOf(readFileSync(join(folder, name, TASK_FILE), 'utf8')) ?? {};
      const timeoutSeconds = numberOf(task.timeoutSeconds);
      if (timeoutSeconds <= 0) {
        throw new Error(`${name}: ${TASK_FILE} needs timeoutSeconds`);
      }
      return {
        check: textOf(task.check, name, 'check'),
        files: join(folder, name, FILES_FOLDER),
        name,
        prompt: textOf(task.prompt, name, 'prompt'),
        timeoutSeconds,
      };
    });

/**
 * A fresh workspace for a task: its starting files, committed in a git repository of its own, so the agent's
 * changes show as a diff.
 */
export const makeWorkspace = (task: EvalTask, workFolder: string): string => {
  const workspace = join(workFolder, task.name);
  rmSync(workspace, { force: true, recursive: true });
  mkdirSync(workspace, { recursive: true });
  if (existsSync(task.files)) {
    cpSync(task.files, workspace, { recursive: true });
  }
  spawnSync('git', ['init', '-q'], { cwd: workspace });
  spawnSync('git', ['add', '-A'], { cwd: workspace });
  spawnSync('git', [...COMMITTER, 'commit', '-q', '--allow-empty', '-m', 'The task as it starts'], {
    cwd: workspace,
  });
  return workspace;
};

/** Whether the task's check passes in a workspace. */
export const checkPasses = (task: EvalTask, workspace: string): boolean =>
  spawnSync('sh', ['-c', task.check], { cwd: workspace, timeout: CHECK_SECONDS * MS_PER_SECOND }).status ===
  SUCCESS;

/** Runs one task: the agent works in a fresh workspace, then the task's check says whether it did it. */
export const runTask = (task: EvalTask, options: RunOptions): EvalResult => {
  const workspace = makeWorkspace(task, options.workFolder);
  const started = Date.now();
  const ran = spawnSync(
    options.bin,
    ['run', '--json', '--cwd', workspace, '--timeout', `${task.timeoutSeconds}s`, task.prompt],
    {
      encoding: 'utf8',
      maxBuffer: MAX_OUTPUT_BYTES,
      stdio: ['ignore', 'pipe', 'inherit'],
      timeout: (task.timeoutSeconds + GRACE_SECONDS) * MS_PER_SECOND,
    }
  );
  // A program that ended without its last line (not found, killed, crashed) failed, in the time it took.
  const finished = runFinishedOf(ran.stdout ?? '') ?? {
    completionTokens: 0,
    outcome: 'failed' as const,
    promptTokens: 0,
    seconds: (Date.now() - started) / MS_PER_SECOND,
    steps: 0,
    toolCalls: 0,
  };
  return { ...finished, success: checkPasses(task, workspace), task: task.name };
};

/** The names after each `--only`. */
export const onlyOf = (args: readonly string[]): string[] =>
  args.flatMap((arg, index) => (arg === ONLY_FLAG ? [args[index + 1] ?? ''] : [])).filter((name) => name !== '');

const resultsFile = (now: Date): string => join(RESULTS_FOLDER, `${now.toISOString().replace(/[:.]/g, '-')}.json`);

const main = (args: readonly string[]): number => {
  const only = onlyOf(args);
  const all = readTasks();
  const unknown = only.filter((name) => !all.some((task) => task.name === name));
  if (unknown.length > 0) {
    console.error(`No such task: ${unknown.join(', ')}. The tasks are: ${all.map((task) => task.name).join(', ')}.`);
    return FAILURE;
  }
  const tasks = only.length > 0 ? all.filter((task) => only.includes(task.name)) : all;
  const options: RunOptions = { bin: process.env[BIN_VARIABLE] ?? DEFAULT_BIN, workFolder: WORK_FOLDER };
  const results = tasks.map((task) => {
    process.stderr.write(`${task.name}…\n`);
    return runTask(task, options);
  });
  console.log(tableOf(results));
  mkdirSync(RESULTS_FOLDER, { recursive: true });
  const file = resultsFile(new Date());
  writeFileSync(file, `${JSON.stringify({ results, totals: totalsOf(results) }, null, 2)}\n`);
  console.error(`\nWritten to ${file}`);
  return results.every((result) => result.success) ? SUCCESS : FAILURE;
};

// Run as a program, not when a test imports its functions.
if (process.argv[1] !== undefined && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exit(main(process.argv.slice(2)));
}
