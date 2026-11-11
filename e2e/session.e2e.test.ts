import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import {
  askForTheEdit,
  DESKTOP,
  EDIT_FLOW,
  GREET,
  LAPTOP,
  navItem,
  overflow,
  PHONE,
  settle,
  start,
  stopRigs,
  tab,
} from './support.js';

afterEach(stopRigs);

// Long enough to read the working line between two chunks of a reply.
const SLOW_CHUNKS_MS = 2_000;

// Long enough to see the delete button held while the turn runs.
const DELETE_TURN_MS = 3_000;

/** Run on an element in the page: whether its text is longer than the room it has. */
const cutShort = (node: { clientWidth: number; scrollWidth: number }): boolean =>
  node.scrollWidth > node.clientWidth;

describe('on a computer', () => {
  it('starts a session from home, and an edit happens once approved', async () => {
    const rig = await start(EDIT_FLOW);
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await askForTheEdit(page);
    expect(readFileSync(join(rig.workspace, 'src/greet.ts'), 'utf8')).toBe(GREET);

    await page.locator('sc-permission .allow button').click();
    await settle(page);
    await page.getByText('it says').waitFor();
    expect(readFileSync(join(rig.workspace, 'src/greet.ts'), 'utf8')).toContain('Hi, ');
    expect(await overflow(page)).toBe(0);
  });

  it('puts a turn’s files back as they were before it, once confirmed', async () => {
    const rig = await start(EDIT_FLOW);
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await askForTheEdit(page);
    await page.locator('sc-permission .allow button').click();
    await settle(page);
    await page.getByText('it says').waitFor();
    const checkpoint = page.locator('sc-conversation .checkpoint');
    await checkpoint.getByText('1 file saved before this turn').waitFor();
    expect(readFileSync(join(rig.workspace, 'src/greet.ts'), 'utf8')).toContain('Hi, ');

    await checkpoint.locator('.restore').click();
    await settle(page);
    await page.locator('jl-confirm').getByRole('button', { name: 'Restore' }).click();
    await page.getByText('Restored 1 file to before that turn.').waitFor();
    expect(readFileSync(join(rig.workspace, 'src/greet.ts'), 'utf8')).toBe(GREET);
    expect(await overflow(page)).toBe(0);
  });
});

describe('a turn’s changes', () => {
  it('open in a dialog, each file as it was before the turn and as it is now', async () => {
    const rig = await start(EDIT_FLOW);
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await askForTheEdit(page);
    await page.locator('sc-permission .allow button').click();
    await settle(page);
    await page.getByText('it says').waitFor();
    const checkpoint = page.locator('sc-conversation .checkpoint');
    await checkpoint.getByText('1 file saved before this turn').waitFor();

    await checkpoint.locator('.changes').click();
    await settle(page);
    const dialog = page.locator('sc-conversation sc-checkpoint-diff jl-dialog');
    await expect.poll(async () => dialog.isVisible()).toBe(true);
    expect(await dialog.locator('.path').textContent()).toBe('src/greet.ts');
    // The line the turn changed: what it said, and what it says now.
    await expect.poll(async () => dialog.locator('sc-diff .line[data-kind="remove"]').count()).toBe(1);
    expect(await dialog.locator('sc-diff .line[data-kind="add"]').count()).toBe(1);

    await dialog.getByRole('button', { exact: true, name: 'Close' }).click();
    await settle(page);
    expect(await dialog.isVisible()).toBe(false);
    expect(await overflow(page)).toBe(0);
  });
});

describe('the plan', () => {
  const step = (content: string, status: string) => ({ content, status });

  it('shows above the composer as the agent works, then folds to a line once done', async () => {
    const rig = await start({
      replies: [
        {
          toolCalls: [
            { arguments: { steps: [step('Read', 'in_progress'), step('Answer', 'pending')] }, name: 'plan' },
          ],
        },
        {
          toolCalls: [
            { arguments: { steps: [step('Read', 'completed'), step('Answer', 'completed')] }, name: 'plan' },
          ],
        },
        { text: 'All done.' },
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Plan it');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('All done.').waitFor();

    const plan = page.locator('session-page sc-plan');
    await expect.poll(() => plan.locator('.progress').textContent()).toBe('2 of 2 done');
    expect(await plan.locator('sc-plan-steps').isHidden()).toBe(true);
    expect(await plan.locator('.current').isHidden()).toBe(true);
    await plan.locator('.head').click();
    await settle(page);
    expect(await plan.locator('sc-plan-steps li').count()).toBe(2);
    expect(await overflow(page)).toBe(0);
  });

  it('names the step in progress once folded, cut short where the line ends', async () => {
    const running = 'Run the whole test suite, then the end-to-end tests in Chromium, and read every failure';
    const rig = await start({
      replies: [
        {
          toolCalls: [
            { arguments: { steps: [step('Read', 'completed'), step(running, 'in_progress')] }, name: 'plan' },
          ],
        },
        { text: 'Stopping here.' },
      ],
    });
    const page = await rig.browser.newPage({ viewport: PHONE });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Plan it');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Stopping here.').waitFor();

    const plan = page.locator('session-page sc-plan');
    expect(await plan.locator('.current').isHidden()).toBe(true);
    await plan.locator('.head').click();
    await settle(page);
    const current = plan.locator('.current');
    expect(await current.textContent()).toBe(running);
    expect(await current.evaluate(cutShort)).toBe(true);
    expect(await overflow(page)).toBe(0);
  });
});

describe('token economy', () => {
  it('is on for a new session, asking the model to be concise after each message, shown as written', async () => {
    const rig = await start({ replies: [{ text: 'Short.' }] });
    const page = await rig.browser.newPage({ viewport: PHONE });
    await page.goto(rig.link);
    const composer = page.locator('home-page sc-composer');
    await composer.locator('textarea').waitFor();
    expect(await composer.locator('.summary').textContent()).toContain('Token economy');
    expect(await overflow(page)).toBe(0);
    await composer.locator('textarea').fill('Explain the plan');
    await composer.locator('textarea').press('Enter');
    await page.getByText('Short.').waitFor();
    expect(JSON.stringify(rig.llama.requests[0])).toContain('Explain the plan\\n\\nBe concise.');
    expect(await page.locator('session-page sc-conversation').textContent()).not.toContain('Be concise.');
    expect(await page.locator('session-page sc-composer .summary').textContent()).toContain('Token economy');
  });
});

describe('the working line', () => {
  it('says what the agent does: thinking, then writing a file it has not finished', async () => {
    const rig = await start({
      replies: [
        {
          chunkDelayMs: SLOW_CHUNKS_MS,
          reasoning: 'A new file, then.',
          toolCalls: [{ arguments: { content: 'export {};\n', path: 'src/new.ts' }, name: 'write' }],
        },
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Make a file');
    await page.locator('home-page textarea').press('Enter');

    const line = page.locator('session-page sc-conversation .working-text');
    // The reply's first chunk can come before the session's page listens: the line follows from the next one.
    const nextChunks = { timeout: SLOW_CHUNKS_MS * 2 };
    await expect.poll(() => line.textContent(), nextChunks).toBe('Thinking…');
    await expect.poll(() => line.textContent(), nextChunks).toBe('Writing a file…');
    expect(await overflow(page)).toBe(0);
  });
});

describe('questions', () => {
  const option = (label: string) => ({ description: `Choose ${label}.`, label });
  const asking = (questions: unknown[]) => ({
    replies: [{ toolCalls: [{ arguments: { questions }, name: 'question' }] }, { text: 'Thanks, noted.' }],
  });
  const approach = { header: 'Approach', options: [option('Map'), option('Sort')], question: 'Map or sort?' };
  const checks = {
    header: 'Checks',
    multiSelect: true,
    options: [option('Tests'), option('Lint')],
    question: 'Which checks?',
  };

  it('are answered on a computer with picks or one’s own words, then sent together', async () => {
    const rig = await start(asking([approach, checks]));
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Plan it');
    await page.locator('home-page textarea').press('Enter');
    const card = page.locator('sc-question');
    await card.getByText('Map or sort?').waitFor();

    await card.getByText('Sort', { exact: true }).click();
    await settle(page);
    await card.getByText('Tests', { exact: true }).click();
    await settle(page);
    await card.locator('.question[data-index="1"] .own').fill('Tests and the e2e suite');
    await settle(page);
    await card.locator('.send button').click();
    await settle(page);
    await page.getByText('Thanks, noted.').waitFor();
    const request = JSON.stringify(rig.llama.requests[1]);
    expect(request).toContain('→ Sort');
    expect(request).toContain('→ Tests and the e2e suite');
    expect(await overflow(page)).toBe(0);
  });

  it('come as a sheet on a phone, a tap answering a single question', async () => {
    const rig = await start(asking([approach]));
    const page = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await page.goto(rig.link);
    await tab(page, 'New').click();
    await settle(page);
    await page.locator('home-page textarea').fill('Plan it');
    await page.locator('home-page textarea').press('Enter');
    const sheet = page.locator('sc-question .card');
    await sheet.waitFor();
    // The sheet ends where the tab bar begins: none of it, its own answer included, goes under the bar.
    const box = await sheet.boundingBox();
    const tabs = await page.locator('jl-tab-bar').boundingBox();
    expect(Math.round((box?.y ?? 0) + (box?.height ?? 0))).toBe(Math.round(tabs?.y ?? 0));
    const own = await page.locator('sc-question .own').boundingBox();
    expect((own?.y ?? 0) + (own?.height ?? 0)).toBeLessThanOrEqual(tabs?.y ?? 0);
    expect(await overflow(page)).toBe(0);

    await page.locator('sc-question').getByText('Map', { exact: true }).click();
    await settle(page);
    await page.getByText('Thanks, noted.').waitFor();
    expect(JSON.stringify(rig.llama.requests[1])).toContain('→ Map');
  });
});

/**
 * A command that stops to ask for a password: a script of the workspace prints its prompt and reads the answer, so
 * the card's words reach it. The session keeps no password, so the check is on what the command printed.
 */
const PASSWORD_FLOW = {
  files: { 'ask.sh': "printf 'Password: '\nread -r line\necho got-it\n" },
  replies: [{ toolCalls: [{ arguments: { command: 'sh ask.sh' }, name: 'bash' }] }, { text: 'It took it.' }],
};

describe('passwords', () => {
  it('are typed into the card a command asks with, and the command goes on', async () => {
    const rig = await start(PASSWORD_FLOW);
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Run the command that asks');
    await page.locator('home-page textarea').press('Enter');
    const card = page.locator('sc-password');
    await card.getByText('Password:').waitFor();
    expect(await card.locator('.secret input').getAttribute('type')).toBe('password');

    await card.locator('.secret input').fill('hunter2');
    await card.locator('.secret input').press('Enter');
    await settle(page);
    await page.getByText('It took it.').waitFor();
    expect(JSON.stringify(rig.llama.requests[1])).toContain('got-it');
    // The password itself is never kept: not in what the model is shown, and not in the session's events.
    expect(JSON.stringify(rig.llama.requests[1])).not.toContain('hunter2');
    expect(JSON.stringify(rig.events())).not.toContain('hunter2');
    expect(rig.events().some((event) => event.kind === 'passwordAnswered' && event.gave)).toBe(true);
    expect(await overflow(page)).toBe(0);
  });
});

describe('deleting a session', () => {
  it('is held while the session works, and offered once its turn is over', async () => {
    const rig = await start({ replies: [{ delayMs: DELETE_TURN_MS, text: 'Done.' }] });
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Say done');
    await page.locator('home-page textarea').press('Enter');
    const header = page.locator('session-page header');

    const held = header.getByRole('button', { name: 'Stop this session before deleting it' });
    await held.waitFor();
    expect(await held.isDisabled()).toBe(true);
    await page.getByText('Done.').waitFor();
    const offered = header.getByRole('button', { name: 'Delete this session' });
    await offered.waitFor();
    expect(await offered.isEnabled()).toBe(true);
  });
});

describe('a peek', () => {
  it('answers beside a session above the composer, and stays out of the conversation', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }, { text: 'I said hello.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();

    const composer = page.locator('session-page sc-composer');
    await composer.locator('textarea').fill('/peek what did you say?');
    await composer.locator('textarea').press('Enter');
    const card = page.locator('session-page sc-peek');
    await card.getByText('I said hello.').waitFor();
    expect(await card.locator('.question').textContent()).toBe('what did you say?');
    expect(await page.locator('session-page sc-conversation').getByText('I said hello.').count()).toBe(0);
    expect(JSON.stringify(rig.llama.requests[1])).toContain('what did you say?');

    await card.locator('.close').click();
    await settle(page);
    expect(await card.isHidden()).toBe(true);
  });
});

// How long each subagent of a test thinks: the person talks with the main agent meanwhile, on the model's third slot.
const SUBAGENT_MS = 6_000;
const SUBAGENT_SLOTS = 3;
const subagentCall = (description: string, prompt: string) => ({
  arguments: { description, prompt },
  name: 'agent',
});

describe('subagents', () => {
  it('work in the background, shown at once and closed, the person talking with the agent meanwhile, then report', async () => {
    const rig = await start({
      replies: [
        {
          toolCalls: [
            subagentCall('Find the store', 'TASK-A: where?'),
            subagentCall('Find the tools', 'TASK-B: where?'),
          ],
        },
        { text: 'Still looking, nothing yet.' },
        { text: 'The store is in src/store.ts, the tools in src/tools.ts.' },
      ],
      scripts: {
        'TASK-A': [{ delayMs: SUBAGENT_MS, text: 'src/store.ts saves them.' }],
        'TASK-B': [{ delayMs: SUBAGENT_MS, text: 'src/tools.ts lists them.' }],
      },
      slots: SUBAGENT_SLOTS,
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Where are the store and the tools?');
    await page.locator('home-page textarea').press('Enter');
    // The reply that started them ended the turn: their reports wake the agent.
    await page.getByText("Waiting for the subagents' reports").waitFor();
    const running = page.locator('session-page sc-tool-row[state="running"]');
    expect(await running.count()).toBe(2);
    expect(await page.locator('session-page sc-tool-row[open]').count()).toBe(0);
    await page.locator('sc-side-panel').getByText('Find the tools').waitFor();
    expect(await page.locator('session-page sc-composer .stop').isVisible()).toBe(true);

    const composer = page.locator('session-page sc-composer textarea');
    await composer.fill('How is it going?');
    await composer.press('Enter');
    await page.getByText('Still looking, nothing yet.').waitFor();
    expect(await running.count()).toBe(2);

    await page.getByText('The store is in src/store.ts').waitFor({ timeout: SUBAGENT_MS * 2 });
    expect(await running.count()).toBe(0);
    expect(await page.locator('session-page sc-tool-row[open]').count()).toBe(0);
    await page.getByText('Report of “Find the store”, given to the agent').waitFor();
    expect(JSON.stringify(rig.llama.requests.at(-1))).toContain('src/tools.ts lists them.');
  });
});

// The events a session page opens with (`HISTORY_EVENTS` in session-page.tsx): a session's history comes a page
// that long at a time.
const SHOWN_EVENTS = 120;
// Subagents whose activity alone fills that page: none of their events draws a row of the conversation.
const QUIET_SUBAGENTS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'];
// How long each of their steps takes, and how many they take: they run past the page they fill.
const QUIET_STEP_MS = 250;
const QUIET_STEPS = 30;

/** Whether the page of events a session shows holds only subagent activity, which the conversation draws nothing for. */
const quietPage = (events: readonly { kind: string }[]): boolean =>
  events.length === SHOWN_EVENTS && events.every((event) => event.kind === 'subagentActivity');

describe('a session whose shown page draws nothing', () => {
  it('brings the older events by itself rather than showing an empty conversation', async () => {
    const rig = await start({
      files: { 'src/greet.ts': GREET },
      replies: [
        {
          toolCalls: QUIET_SUBAGENTS.map((name) => ({
            arguments: { description: name, prompt: `QUIET-${name}` },
            name: 'agent',
          })),
        },
        { delayMs: QUIET_STEP_MS * QUIET_STEPS * 2, text: 'The end.' },
      ],
      scripts: Object.fromEntries(
        QUIET_SUBAGENTS.map((name) => [
          `QUIET-${name}`,
          Array.from({ length: QUIET_STEPS }, () => ({
            delayMs: QUIET_STEP_MS,
            toolCalls: [{ arguments: { path: 'src/greet.ts' }, name: 'read' }],
          })),
        ])
      ),
      slots: QUIET_SUBAGENTS.length + 1,
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Start');
    await page.locator('home-page textarea').press('Enter');

    // The subagents' activities fill the page the session shows while they run.
    await expect
      .poll(() => quietPage(rig.events().slice(-SHOWN_EVENTS)), { timeout: QUIET_STEP_MS * QUIET_STEPS })
      .toBe(true);
    await navItem(page, 'New session').click();
    await settle(page);
    await navItem(page, 'Start').click();
    await page.evaluate('0');

    // The page it shows draws no row: the first message comes from a page above, asked by the conversation itself.
    await page.locator('session-page sc-conversation').getByText('Start', { exact: true }).waitFor();
    expect(await overflow(page)).toBe(0);
  });
});
