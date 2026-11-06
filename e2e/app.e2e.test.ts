import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { startRig } from './rig.js';

import type { Locator, Page } from 'playwright-core';
import type { E2eRig } from './types/e2e.types.js';

const DESKTOP = { height: 900, width: 1440 };
// Wide enough for the side panel, which leaves the composer 650 pixels.
const LAPTOP = { height: 900, width: 1280 };
const PHONE = { height: 844, width: 390 };
const GREET = 'export const greet = (name: string) => "Hello, " + name;\n';

/** Reads greet.ts, asks to edit it, then answers: the flow every check below starts from. */
const EDIT_FLOW = {
  files: { 'src/greet.ts': GREET },
  replies: [
    { toolCalls: [{ arguments: { path: 'src/greet.ts' }, name: 'read' }] },
    { toolCalls: [{ arguments: { newString: 'Hi', oldString: 'Hello', path: 'src/greet.ts' }, name: 'edit' }] },
    { text: 'Done: it says **Hi** now.' },
  ],
};

// Longer than the test: the turn still runs when it ends.
const SLOW_REPLY_MS = 60_000;
const CONTEXT_COUNT = ' / ';

/** Reads greet.ts, then thinks on: stop beside send, the context count, and the composer's chip on their row. */
const WORKING_FLOW = {
  files: { 'src/greet.ts': GREET },
  model: 'qwen3.8-flash-next',
  replies: [
    { toolCalls: [{ arguments: { path: 'src/greet.ts' }, name: 'read' }] },
    { delayMs: SLOW_REPLY_MS, text: 'Too late.' },
  ],
};

// Long enough to read the working line between two chunks of a reply.
const SLOW_CHUNKS_MS = 2_000;

// Long enough to open the game and start it; the turn then ends while it runs.
const GAME_TURN_MS = 5_000;

/** Thinks a few seconds, then answers: the game pauses as the turn ends. */
const GAME_FLOW = {
  files: { 'src/greet.ts': GREET },
  replies: [{ delayMs: GAME_TURN_MS, text: 'Done.' }],
};

// Long enough to see a session at work, and to leave it for another page before its turn ends.
const LIGHT_TURN_MS = 3_000;

// Far more than a window's height of paragraphs, and a turn of the wheel up through them.
const PARAGRAPHS = 80;
const LONG_REPLY = Array.from({ length: PARAGRAPHS }, (_, index) => `Paragraph ${index + 1}.`).join('\n\n');
const WHEEL_UP = -400;

const rigs: E2eRig[] = [];

const start = async (options: Parameters<typeof startRig>[0]): Promise<E2eRig> => {
  const rig = await startRig(options);
  rigs.push(rig);
  return rig;
};

afterEach(async () => {
  await Promise.all(rigs.splice(0).map((rig) => rig.stop()));
});

const GMKTEC = { address: 'http://gmktec:4500', id: 'g1', name: 'gmktec', reachable: true };

/**
 * Run in the page before it loads: the desktop app around it, as WebView2 lets the page talk to it, showing a
 * computer among those it knows. It keeps what the page asked, sends each message back as WebView2 does, and pairs
 * with gmktec on the code 123456.
 */
const fakeDesktopApp = (current = 'local', others: readonly object[] = []): string => `(() => {
  const listeners = [];
  const computers = [{ address: 'http://localhost:4500', id: 'local', name: 'This computer', reachable: true }, ...${JSON.stringify(others)}];
  const send = (text) => setTimeout(() => listeners.forEach((listener) => listener({ data: text })), 10);
  const answer = (message) => send(JSON.stringify(message));
  window.desktopAsked = [];
  window.chrome = window.chrome || {};
  window.chrome.webview = {
    addEventListener: (type, listener) => listeners.push(listener),
    removeEventListener: (type, listener) => listeners.splice(listeners.indexOf(listener), 1),
    postMessage: (text) => {
      window.desktopAsked.push(text);
      send(text);
      const request = JSON.parse(text);
      if (request.type === 'addComputer' && request.code !== '123456') {
        answer({ message: 'The code was refused: it works once, for a minute.', type: 'computerRefused' });
        return;
      }
      if (request.type === 'addComputer') {
        computers.push(${JSON.stringify(GMKTEC)});
      }
      answer({ computers, current: '${current}', type: 'computers' });
    },
  };
})();`;

// Run in the page: the tests are typed for Node, and these few expressions need no more.
const PAGE_OVERFLOW = 'document.documentElement.scrollWidth - window.innerWidth';
const BODY_BACKGROUND = 'getComputedStyle(document.body).backgroundColor';
// Whether the page kept a token of its own, which a pairing code is traded for.
const KEPT_TOKEN = "localStorage.getItem('swcode.token') !== null";

/** How much wider than the screen the page is: nothing should ever scroll sideways. */
const overflow = (page: Page) => page.evaluate(PAGE_OVERFLOW);

/** Run on an element in the page: whether its text is longer than the room it has. */
const cutShort = (node: { clientWidth: number; scrollWidth: number }): boolean =>
  node.scrollWidth > node.clientWidth;

/** An item of the navigation down the side, by its label. */
const navItem = (page: Page, label: string) =>
  page.locator('sc-nav jl-side-nav').getByText(label, { exact: true });

/** The settings' cog, at the foot of the navigation. */
const settingsLink = (page: Page) => page.locator('sc-nav .settings');

/** The style of the first shape of a navigation item's icon, by the start of its id: its light's fill, or none. */
const navIconFill = (page: Page, id: string) =>
  page.locator(`sc-nav .item[data-id^="${id}"] svg > *`).first().getAttribute('style');

/** A topic's tab on the settings page, by its label; `selected` the one shown. */
const topicTab = (page: Page, label: string, selected?: boolean) =>
  page
    .locator('settings-page jl-tabs')
    .getByRole('tab', { exact: true, name: label, ...(selected ? { selected } : {}) });

/** The names of the skills the settings list, in order. */
const skillNames = (page: Page) =>
  page
    .locator('sc-skills-editor jl-list-item[data-name]')
    .evaluateAll((items) => items.map((item) => item.getAttribute('data-name')));

/** A tab of a phone's bar under the page, by its label. */
const tab = (page: Page, label: string) => page.locator('jl-tab-bar').getByText(label, { exact: true });

// Two frames: the router has the next page, and the outlet decided when it shows.
const NEXT_FRAMES = 'new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))';
// The pages the router shows.
const PAGES = 'home-page, session-page, sessions-page, project-page, settings-page';

// Where the shapes of each icon that turns are, in every shadow root under the side nav and the session page.
const TURNING_SHAPES = `JSON.stringify((() => {
  const found = [];
  const walk = (root) => root.querySelectorAll('*').forEach((element) => {
    if (element.localName === 'animateTransform') found.push(element.parentElement.getBoundingClientRect().toJSON());
    if (element.shadowRoot) walk(element.shadowRoot);
  });
  walk(document);
  return found;
})())`;
const TURN_SAMPLE_MS = 150;

/**
 * A click is delivered on the next command in headless Chromium; a page it opens shows once it has its data, the
 * one before staying meanwhile (jlib's page outlet): both are in the document until then.
 */
const settle = async (page: Page) => {
  await page.evaluate('0');
  await page.evaluate(NEXT_FRAMES);
  await expect.poll(() => page.locator(PAGES).count()).toBe(1);
};

/** In a session that asks before edits, picked in the composer's panel: new ones run everything. */
async function askForTheEdit(page: Page): Promise<void> {
  const composer = page.locator('home-page sc-composer');
  await composer.locator('.choices').click();
  await settle(page);
  await composer.locator('sc-session-choices').getByText('Ask before edits', { exact: true }).click();
  await settle(page);
  expect(await composer.locator('.summary').textContent()).toContain('Ask before edits');
  await page.keyboard.press('Escape');
  await page.locator('home-page textarea').fill('Make greet say Hi');
  await page.locator('home-page textarea').press('Enter');
  await page.getByText('Edit this file?').waitFor();
}

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
    const box = await sheet.boundingBox();
    expect(Math.round((box?.y ?? 0) + (box?.height ?? 0))).toBe(PHONE.height);
    expect(await overflow(page)).toBe(0);

    await page.locator('sc-question').getByText('Map', { exact: true }).click();
    await settle(page);
    await page.getByText('Thanks, noted.').waitFor();
    expect(JSON.stringify(rig.llama.requests[1])).toContain('→ Map');
  });
});

describe('projects', () => {
  it('come from a folder picked in the dialog, and a session starts in one', async () => {
    const rig = await start({
      files: { 'src/greet.ts': GREET },
      replies: [{ toolCalls: [{ arguments: { command: 'pwd' }, name: 'bash' }] }, { text: 'Listed.' }],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page .new-project').click();
    await settle(page);
    await page.locator('sc-folder-browser li button', { hasText: 'src' }).click();
    await settle(page);
    await page.locator('sc-project-dialog .confirm button', { hasText: 'Use src' }).click();
    await settle(page);
    await page.locator('home-page select').filter({ hasText: 'src' }).waitFor();

    await page.locator('home-page textarea').fill('Where am I?');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Listed.').waitFor();
    await navItem(page, 'src').click();
    await settle(page);
    await page.locator('project-page sc-session-row').getByText('Where am I?').first().waitFor();
    expect(await page.getByText(join(rig.workspace, 'src')).count()).toBeGreaterThan(0);

    await page.locator('project-page .remove').click();
    await settle(page);
    await page.getByText('Its session goes with it, their conversations for good.', { exact: false }).waitFor();
    await page.locator('jl-confirm').getByRole('button', { name: 'Remove' }).click();
    await settle(page);
    await page.locator('home-page textarea').waitFor();
    await navItem(page, 'src').waitFor({ state: 'detached' });
    expect(readFileSync(join(rig.workspace, 'src/greet.ts'), 'utf8')).toBe(GREET);
  });
});

describe('the composer', () => {
  async function startTurn(page: Page): Promise<Locator> {
    await page.locator('home-page textarea').fill('Read greet');
    await page.locator('home-page textarea').press('Enter');
    const composer = page.locator('session-page sc-composer');
    // Attached rather than visible: a phone's composer hides the count.
    await composer.locator('.context', { hasText: CONTEXT_COUNT }).waitFor({ state: 'attached' });
    await composer.locator('.stop').waitFor();
    return composer;
  }

  /** The middle of a box, up and down: chips and buttons on one row share it. */
  const middle = async (locator: Locator): Promise<number> => {
    const box = await locator.boundingBox();
    return Math.round((box?.y ?? 0) + (box?.height ?? 0) / 2);
  };

  async function expectOneRow(composer: Locator): Promise<void> {
    const row = await middle(composer.locator('.chip').first());
    expect(await middle(composer.locator('.play'))).toBe(row);
    expect(await middle(composer.locator('.stop'))).toBe(row);
    expect(await middle(composer.locator('.send'))).toBe(row);
  }

  it('keeps stop and send on the chips’ row while a turn runs, beside both panels', async () => {
    const rig = await start(WORKING_FLOW);
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.goto(rig.link);
    const composer = await startTurn(page);

    await expectOneRow(composer);
    expect(await composer.locator('.choices').isVisible()).toBe(true);
    expect(await composer.locator('.context').isVisible()).toBe(true);
    expect(await overflow(page)).toBe(0);
  });

  it('opens a game while a turn runs, which pauses when the turn ends and leads back to the session', async () => {
    const rig = await start(GAME_FLOW);
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.goto(rig.link);
    // Not startTurn: the context count comes with the only reply, as the turn ends.
    await page.locator('home-page textarea').fill('Read greet');
    await page.locator('home-page textarea').press('Enter');
    const composer = page.locator('session-page sc-composer');

    await composer.locator('.play').click();
    const game = page.locator('session-page sc-arcade sc-game');
    await game.locator('canvas').waitFor();
    await game.locator('.action', { hasText: 'Start' }).click();
    await page.evaluate('0');
    await game.locator('.screen').waitFor({ state: 'hidden' });
    await game.getByText('swcode is done').waitFor({ timeout: GAME_TURN_MS * 2 });
    await game.locator('.leave').click();
    await page.evaluate('0');
    await game.waitFor({ state: 'hidden' });
    expect(await composer.locator('.play').isHidden()).toBe(true);
  });

  it('switches between the games, the one left pausing and the one shown taking the keys', async () => {
    const rig = await start(WORKING_FLOW);
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Read greet');
    await page.locator('home-page textarea').press('Enter');
    await page.locator('session-page sc-composer .play').click();
    await page.evaluate('0');
    const arcade = page.locator('session-page sc-arcade');
    await arcade.locator('sc-game canvas').waitFor();

    await arcade.getByRole('tab', { name: 'Vim' }).click();
    await page.evaluate('0');
    const vim = arcade.locator('sc-vim-game');
    await vim.locator('.action', { hasText: 'Start' }).click();
    await page.evaluate('0');
    await page.keyboard.press('l');
    await vim.locator('.spent', { hasText: '1 keys' }).waitFor();

    await arcade.getByRole('tab', { name: 'Keyboard' }).click();
    await page.evaluate('0');
    const keyboard = arcade.locator('sc-keyboard-game');
    await keyboard.locator('.action', { hasText: 'Start' }).click();
    await page.evaluate('0');
    const first = (await keyboard.locator('.at').textContent()) ?? '';
    await page.keyboard.type(first);
    expect(await keyboard.locator('.done').textContent()).toBe(first);
    // A letter to come, in no line yet: a wrong key.
    await page.keyboard.press('z');
    await keyboard.locator('.at.missed').waitFor();

    await arcade.getByRole('tab', { name: 'Vim' }).click();
    await page.evaluate('0');
    await vim.getByText('Paused').waitFor();
  });

  it('turns the header’s spinner while a turn runs, the side nav filling the session’s bubble instead', async () => {
    const rig = await start(WORKING_FLOW);
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Read greet');
    await page.locator('home-page textarea').press('Enter');
    await page.locator('session-page sc-composer .stop').waitFor();
    const before = JSON.parse(String(await page.evaluate(TURNING_SHAPES)));
    await page.waitForTimeout(TURN_SAMPLE_MS);
    const after = JSON.parse(String(await page.evaluate(TURNING_SHAPES)));
    // The header's status; the side nav lights its icon, without a spinner.
    expect(before.length).toBeGreaterThanOrEqual(1);
    expect(after).not.toEqual(before);
    expect(await navIconFill(page, 'session:')).toContain('--jl-accent');
  });

  it('keeps them there on a phone, the model and the context count left out', async () => {
    const rig = await start(WORKING_FLOW);
    const page = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await page.goto(rig.link);
    await tab(page, 'New').click();
    await settle(page);
    const composer = await startTurn(page);

    await expectOneRow(composer);
    expect(await composer.locator('.model').isHidden()).toBe(true);
    expect(await composer.locator('.context').isHidden()).toBe(true);
    expect(await overflow(page)).toBe(0);
  });
});

describe('skills', () => {
  const ReleaseNotes = {
    description: 'Writes release notes from the commits.',
    instructions: 'List the commits since the last tag.',
    name: 'release-notes',
  };

  it('start with the coding style and Jadis, and are written, opened and deleted in the settings', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await settingsLink(page).click();
    await settle(page);
    // The Startup tab leaves once the daemon says it cannot start with this computer, moving the tabs after it.
    await topicTab(page, 'Startup').waitFor({ state: 'detached' });
    await topicTab(page, 'Skills').click();
    await settle(page);
    await topicTab(page, 'Skills', true).waitFor();
    await expect.poll(() => skillNames(page)).toEqual(['coding-style', 'jadis']);

    await page.locator('sc-skills-editor jl-list-item[data-name="jadis"]').click();
    await settle(page);
    const editor = page.locator('sc-skills-editor form');
    await expect.poll(() => editor.locator('#skill-name input').inputValue()).toBe('jadis');
    expect(await editor.locator('#skill-instructions textarea').inputValue()).toContain('# Jadis');
    await editor.locator('.instructions-field').getByText('references/api.md', { exact: false }).waitFor();

    await page.locator('sc-skills-editor .create button').click();
    await settle(page);
    await editor.locator('#skill-name input').fill(ReleaseNotes.name);
    await editor.locator('#skill-description textarea').fill(ReleaseNotes.description);
    await editor.locator('#skill-instructions textarea').fill(ReleaseNotes.instructions);
    await editor.locator('.save button').click();
    await settle(page);
    await expect.poll(() => skillNames(page)).toEqual(['coding-style', 'jadis', 'release-notes']);
    expect(
      await page.locator('sc-skills-editor jl-list-item[data-name="release-notes"]').getAttribute('selected')
    ).toBe('');

    await editor.locator('.remove button').click();
    await settle(page);
    await page.locator('jl-confirm').getByRole('button', { name: 'Delete' }).click();
    await settle(page);
    await expect.poll(() => skillNames(page)).toEqual(['coding-style', 'jadis']);
    expect(await editor.isHidden()).toBe(true);
    expect(await overflow(page)).toBe(0);
  });

  it('are made by the agent when asked, and the session shows what it was told of them', async () => {
    const rig = await start({
      replies: [{ toolCalls: [{ arguments: ReleaseNotes, name: 'saveskill' }] }, { text: 'The skill is saved.' }],
    });
    const page = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await page.goto(rig.link);
    await tab(page, 'New').click();
    await settle(page);
    await page.locator('home-page textarea').fill('Make a skill for release notes');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('The skill is saved.').waitFor();
    await page.getByText('Skills the agent was told of: coding-style, jadis').waitFor();
    expect(await overflow(page)).toBe(0);

    await page.goto(`${rig.link}#/settings/skills`);
    await page.locator('sc-skills-editor jl-list-item[data-name="release-notes"]').waitFor();
    expect(await overflow(page)).toBe(0);
  });
});

describe('settings', () => {
  const ClaudeHome = {
    '.claude/plugins/cache/jadis/jadis/1.6.0/skills/jadis/SKILL.md':
      '---\nname: jadis\ndescription: Builds apps with Jadis.\n---\nUse Jadis.',
    '.claude/plugins/installed_plugins.json': JSON.stringify({
      plugins: {
        'jadis@jadis': [{ installPath: '/home/someone/.claude/plugins/cache/jadis/jadis/1.6.0', scope: 'user' }],
      },
      version: 2,
    }),
    '.claude/skills/pixel-art/SKILL.md': '---\nname: pixel-art\ndescription: Makes sprites.\n---\nPaint.',
  };

  it('open on the models, and keep what the form holds from one topic to the other', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await settingsLink(page).click();
    await settle(page);
    await topicTab(page, 'Models', true).waitFor();
    await topicTab(page, 'Pictures').click();
    await settle(page);
    await page.locator('#setting-SWCODE_IMAGE_URL input').fill('http://garfield:8090');
    await topicTab(page, 'Network').click();
    await settle(page);
    expect(await page.locator('#setting-SWCODE_IMAGE_URL').isHidden()).toBe(true);
    expect(await page.locator('#setting-SWCODE_PORT').isVisible()).toBe(true);
    expect(page.url()).toContain('#/settings/network');
    await topicTab(page, 'Pictures').click();
    await settle(page);
    expect(await page.locator('#setting-SWCODE_IMAGE_URL input').inputValue()).toBe('http://garfield:8090');
  });

  it('list the SSH hosts, a new one added from its dialog and saved at once', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await settingsLink(page).click();
    await settle(page);
    await topicTab(page, 'SSH').click();
    await settle(page);
    const hosts = page.locator('sc-ssh-hosts');
    await hosts.getByText('No SSH host yet', { exact: false }).waitFor();
    expect(await page.locator('settings-page jl-page > jl-button.save').isHidden()).toBe(true);

    await hosts.locator('.add').click();
    await settle(page);
    await page.locator('sc-ssh-host-dialog .host input').fill('me@build box');
    await page.locator('sc-ssh-host-dialog .save').click();
    await page.locator('sc-ssh-host-dialog').getByText('One host, without spaces or commas.').waitFor();
    await page.locator('sc-ssh-host-dialog .host input').fill('me@build-box');
    await page.locator('sc-ssh-host-dialog .save').click();
    await hosts.locator('.hosts jl-list-item', { hasText: 'me@build-box' }).waitFor();
    await hosts.locator('jl-alert').getByText('Saved', { exact: false }).waitFor();
  });

  it('add an llm-server and a preset of its models, which a new session then picks in its composer', async () => {
    const rig = await start({ replies: [{ text: 'Done.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(`${rig.link}#/settings/models`);
    const editor = page.locator('sc-models-editor');
    await editor.locator('.sources jl-list-item').first().waitFor();

    await editor.locator('.add-source button').click();
    await settle(page);
    const source = page.locator('sc-source-dialog');
    await source.locator('.name input').fill('second');
    await source.locator('.address input').fill(rig.llama.url);
    await source.locator('.save button').click();
    await settle(page);
    await editor.locator('.sources jl-list-item', { hasText: 'second' }).waitFor();

    await editor.locator('.add-preset button').click();
    await settle(page);
    const preset = page.locator('sc-preset-dialog');
    await preset.locator('.name input').fill('Mixed');
    await preset.locator('.main .source select').selectOption('second');
    await settle(page);
    await preset.locator('.save button').click();
    await settle(page);
    await editor.locator('.presets jl-list-item', { hasText: 'Mixed' }).waitFor();

    const phone = await rig.browser.newPage({ viewport: PHONE });
    await phone.goto(rig.link);
    const composer = phone.locator('home-page sc-composer');
    await expect.poll(() => composer.locator('.summary').textContent()).toContain('Default');
    await composer.locator('.choices').click();
    await settle(phone);
    const choices = composer.locator('sc-session-choices');
    await choices.getByText('Mixed', { exact: true }).click();
    await settle(phone);
    expect(await overflow(phone)).toBe(0);
    expect(await composer.locator('.summary').textContent()).toContain('Mixed');
    await phone.keyboard.press('Escape');
    await composer.locator('textarea').fill('Hi');
    await composer.locator('textarea').press('Enter');
    await phone.getByText('Done.').waitFor();
    expect(await phone.locator('session-page sc-composer .summary').textContent()).toContain('Mixed');
  });

  it('show Claude Code’s skills and plugins under Plugins, and turn a plugin off for the next turns', async () => {
    const rig = await start({ homeFiles: ClaudeHome, replies: [] });
    const page = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await page.goto(`${rig.link}#/settings/plugins`);
    const plugin = page.locator('sc-claude-sources .plugin', { hasText: 'jadis@jadis' });
    await plugin.waitFor();
    expect(await plugin.textContent()).toContain('jadis:jadis');
    expect(await page.locator('sc-claude-sources .skills').textContent()).toContain('pixel-art');
    expect(await overflow(page)).toBe(0);

    await plugin.getByRole('switch').click();
    await settle(page);
    await page.locator('sc-claude-sources jl-alert').getByText('Saved', { exact: false }).waitFor();
    await plugin.getByText('set here', { exact: false }).waitFor();
    await page.reload();
    await plugin.waitFor();
    expect(await plugin.getByRole('switch').isChecked()).toBe(false);
  });

  it('leave Startup and Computers out in a browser, and set how the Windows app starts with Windows', async () => {
    const rig = await start({ replies: [] });
    const browser = await rig.browser.newPage({ viewport: DESKTOP });
    await browser.goto(`${rig.link}#/settings/models`);
    await topicTab(browser, 'Models', true).waitFor();
    await topicTab(browser, 'Startup').waitFor({ state: 'detached' });
    await topicTab(browser, 'Computers').waitFor({ state: 'detached' });

    // The Windows app's daemon answers that it can: this one runs from the command line, so the test plays it.
    const windowsApp = await rig.browser.newPage({ viewport: DESKTOP });
    const asked: string[] = [];
    await windowsApp.route('**/api/startup', async (route) => {
      const body = route.request().postData();
      asked.push(...(body === null ? [] : [body]));
      await route.fulfill({ json: { available: true, mode: body === null ? 'off' : JSON.parse(body).mode } });
    });
    await windowsApp.goto(`${rig.link}#/settings/startup`);
    const startup = windowsApp.locator('sc-startup-settings');
    await expect.poll(() => startup.getByRole('radio', { name: 'Don’t start' }).isChecked()).toBe(true);
    expect(await topicTab(windowsApp, 'Startup', true).isVisible()).toBe(true);
    await startup.getByRole('radio', { name: 'In the notification area' }).click();
    await settle(windowsApp);
    await startup.locator('jl-alert').getByText('Saved', { exact: false }).waitFor();
    expect(asked).toEqual(['{"mode":"tray"}']);
    expect(await startup.getByRole('radio', { name: 'In the notification area' }).isChecked()).toBe(true);
  });

  it('list the desktop app’s computers, pair another one with its code, and open it', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.addInitScript(fakeDesktopApp());
    await page.goto(`${rig.link}#/settings/computers`);
    const computers = page.locator('sc-computers jl-list-item.computer');
    await computers.first().getByText('This computer', { exact: true }).filter({ visible: true }).waitFor();
    await computers.first().getByText('Shown here').waitFor();

    await page.locator('#computer-address input').fill('gmktec');
    await page.locator('#computer-code input').fill('000000');
    await page.locator('sc-computers .add').click();
    await settle(page);
    await page.locator('sc-computers jl-alert').getByText('refused', { exact: false }).waitFor();
    await page.locator('#computer-code input').fill('123456');
    await page.locator('sc-computers .add').click();
    await settle(page);
    await page.locator('sc-computers jl-alert').getByText('Added', { exact: false }).waitFor();
    expect(await computers.count()).toBe(2);
    expect(await page.locator('#computer-address input').inputValue()).toBe('');

    await computers.nth(1).locator('.open').click();
    await settle(page);
    const asked = await page.evaluate('window.desktopAsked');
    expect(asked).toContainEqual('{"id":"g1","type":"openComputer"}');
    expect(asked).toContainEqual('{"address":"gmktec","code":"123456","type":"addComputer"}');
    expect(await overflow(page)).toBe(0);
  });
});

describe('the command palette', () => {
  it('opens on Ctrl K and goes to a session found by its title', async () => {
    const rig = await start({ replies: [{ text: 'Tidied.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Tidy the README');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Tidied.').waitFor();
    await navItem(page, 'New session').click();
    await settle(page);

    await page.keyboard.press('Control+k');
    const palette = page.locator('jl-command-palette');
    await palette.getByRole('combobox').fill('tidy');
    await palette.getByRole('option', { name: /Tidy the README/ }).waitFor();
    await page.keyboard.press('Enter');
    await settle(page);
    await page.locator('session-page').getByText('Tidied.').waitFor();
  });
});

describe('the computer', () => {
  it('is this one in a browser on it, without a way to switch', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    const where = page.locator('sc-nav sc-computer-switch');
    await where.locator('.name', { hasText: 'This computer' }).waitFor();
    expect((await where.getAttribute('class')) ?? '').not.toContain('remote');
    expect(await where.locator('jl-select').isHidden()).toBe(true);
  });

  it('is marked remote in the desktop app showing another one, and switches back or leads to the settings', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.addInitScript(fakeDesktopApp('g1', [GMKTEC]));
    await page.goto(rig.link);
    const where = page.locator('sc-nav sc-computer-switch');
    await where.locator('.badge', { hasText: 'Remote' }).waitFor();
    expect((await where.getAttribute('class')) ?? '').toContain('remote');
    const computers = where.locator('jl-select select');
    expect(await computers.inputValue()).toBe('g1');

    await computers.selectOption('local');
    expect(await page.evaluate('window.desktopAsked')).toContainEqual('{"id":"local","type":"openComputer"}');
    await computers.selectOption('manage');
    await topicTab(page, 'Computers', true).waitFor();
    expect(await computers.inputValue()).toBe('g1');

    const phone = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await phone.addInitScript(fakeDesktopApp('g1', [GMKTEC]));
    await phone.goto(rig.link);
    await phone.locator('sc-computer-switch .badge', { hasText: 'Remote' }).waitFor({ state: 'attached' });
    expect(await overflow(phone)).toBe(0);
  });
});

describe('pictures', () => {
  // Run in the page: a 2000 × 1000 screenshot pasted in the composer's text, which is deep in shadow roots.
  const PasteScreenshot = `(async () => {
    const find = (root) => root.querySelector('textarea#prompt')
      ?? [...root.querySelectorAll('*')].flatMap((el) => (el.shadowRoot ? [find(el.shadowRoot)] : [])).find(Boolean);
    const canvas = document.createElement('canvas');
    canvas.width = 2000;
    canvas.height = 1000;
    const context = canvas.getContext('2d');
    context.fillStyle = '#c62828';
    context.fillRect(0, 0, 2000, 1000);
    const blob = await new Promise((resolve) => canvas.toBlob(resolve, 'image/png'));
    const clipboardData = new DataTransfer();
    clipboardData.items.add(new File([blob], 'screenshot.png', { type: 'image/png' }));
    find(document).dispatchEvent(new ClipboardEvent('paste', { bubbles: true, clipboardData }));
  })()`;

  it('are pasted in the composer, made small, and sent to a model that sees', async () => {
    const rig = await start({ replies: [{ text: 'A red rectangle.' }], vision: true });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').waitFor();
    await page.evaluate(PasteScreenshot);
    await page.locator('home-page sc-composer .thumb img').waitFor();

    const shown = page.waitForResponse((response) => response.url().includes('/api/images/') && response.ok());
    await page.locator('home-page textarea').fill('What is this?');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('A red rectangle.').waitFor();
    await shown;

    expect(rig.events().find((event) => event.kind === 'userMessage')).toMatchObject({
      pictures: [{ height: 640, type: 'image/jpeg', width: 1280 }],
    });
    expect(JSON.stringify(rig.llama.requests[0])).toContain('data:image/jpeg;base64,');
    expect(await page.locator('session-page sc-message[author="user"] img').count()).toBe(1);
    expect(await overflow(page)).toBe(0);
  });

  it('say so when the model does not see them', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: PHONE });
    await page.goto(rig.link);
    await tab(page, 'New').click();
    await settle(page);
    await page.locator('home-page textarea').waitFor();
    await page.evaluate(PasteScreenshot);
    await page.getByText('This model cannot see pictures').waitFor();
    expect(await overflow(page)).toBe(0);
  });
});

describe('on a phone', () => {
  it('fits the screen on every page, with the request as a sheet', async () => {
    const rig = await start(EDIT_FLOW);
    const page = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await page.goto(rig.link);
    expect(await overflow(page)).toBe(0);

    await tab(page, 'New').click();
    await settle(page);
    await page.locator('home-page textarea').waitFor();
    expect(await overflow(page)).toBe(0);

    await askForTheEdit(page);
    expect(await overflow(page)).toBe(0);
    const sheet = await page.locator('sc-permission .card').boundingBox();
    expect(Math.round((sheet?.y ?? 0) + (sheet?.height ?? 0))).toBe(PHONE.height);
  });
});

describe('pairing a phone', () => {
  it('takes the QR code’s link once: the phone gets in, the same link again does not', async () => {
    const rig = await start({ replies: [] });
    const computer = await rig.browser.newPage({ viewport: DESKTOP });
    await computer.goto(rig.link);
    await settingsLink(computer).click();
    await settle(computer);
    await topicTab(computer, 'Phone').click();
    await settle(computer);
    expect(computer.url()).toContain('#/settings/phone');
    const link = computer.locator('sc-phone .link');
    await expect.poll(async () => (await link.textContent()) ?? '').toContain('pair=');
    const shown = new URL((await link.textContent()) ?? '');
    expect(shown.searchParams.has('token')).toBe(false);
    // The page shows the address a phone on the network reaches; the test's daemon listens here only.
    const pairing = new URL(`/?pair=${shown.searchParams.get('pair')}`, rig.link).toString();

    const phone = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await phone.goto(pairing);
    await expect.poll(() => phone.evaluate(KEPT_TOKEN)).toBe(true);
    expect(phone.url()).not.toContain('pair=');

    const stranger = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await stranger.goto(pairing);
    await stranger.locator('jl-alert').getByText('This pairing code expired').waitFor();
  });
});

describe('a long conversation', () => {
  it('scrolls under the wheel inside the session page, the composer staying at the bottom of the window', async () => {
    const rig = await start({ replies: [{ text: LONG_REPLY }] });
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Write a lot');
    await page.locator('home-page textarea').press('Enter');
    const last = page.getByText(`Paragraph ${PARAGRAPHS}.`);
    await last.waitFor();
    await settle(page);
    const composer = await page.locator('session-page sc-composer').boundingBox();
    expect((composer?.y ?? 0) + (composer?.height ?? 0)).toBeLessThanOrEqual(LAPTOP.height);

    const before = (await last.boundingBox())?.y ?? 0;
    const conversation = await page.locator('session-page sc-conversation').boundingBox();
    await page.mouse.move((conversation?.x ?? 0) + (conversation?.width ?? 0) / 2, (conversation?.y ?? 0) + 100);
    await page.mouse.wheel(0, WHEEL_UP);
    await expect.poll(async () => (await last.boundingBox())?.y ?? 0).toBeGreaterThan(before);
  });
});

describe('the side panel', () => {
  it('leaves the header’s buttons on its row beside a long title, and shows llm-server and the build', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.goto(rig.link);
    await page
      .locator('home-page textarea')
      .fill('Find why the release notes skip the commits of merged branches');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();

    const row = async (selector: string): Promise<number> => {
      const box = await page.locator(`session-page header ${selector}`).boundingBox();
      return Math.round((box?.y ?? 0) + (box?.height ?? 0) / 2);
    };
    expect(await row('.panel-toggle')).toBe(await row('sc-title'));
    expect(await row('.delete')).toBe(await row('sc-title'));
    // The title gives way, cut short: it ends before the path begins.
    const title = await page.locator('session-page header sc-title button').boundingBox();
    const path = await page.locator('session-page header .chip').boundingBox();
    expect((title?.x ?? 0) + (title?.width ?? 0)).toBeLessThanOrEqual(path?.x ?? 0);
    expect(await page.locator('session-page sc-side-panel .version').textContent()).toBe('swcode dev');
    const server = page.locator('session-page sc-side-panel .server');
    await server.locator('.state', { hasText: 'Connected' }).waitFor();
    expect(await server.count()).toBe(1);
  });

  it('hides and shows again from the session’s header, and stays as it was left', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();
    const panel = page.locator('session-page sc-side-panel');
    expect(await panel.isVisible()).toBe(true);

    await page.getByRole('button', { name: 'Hide the side panel' }).click();
    await settle(page);
    expect(await panel.isVisible()).toBe(false);
    await page.reload();
    await page.getByText('Hello there.').waitFor();
    expect(await panel.isVisible()).toBe(false);

    await page.getByRole('button', { name: 'Show the side panel' }).click();
    await settle(page);
    expect(await panel.isVisible()).toBe(true);
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

describe('the navigation', () => {
  it('fills a session’s bubble and its project’s icon while it works, then done until seen, and empties them', async () => {
    const rig = await start({ replies: [{ delayMs: LIGHT_TURN_MS, text: 'Done.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await expect.poll(() => navIconFill(page, 'session:')).toContain('--jl-accent');
    expect(await navIconFill(page, 'project:')).toContain('--jl-accent');

    await settingsLink(page).click();
    await settle(page);
    await expect.poll(() => navIconFill(page, 'session:'), { timeout: LIGHT_TURN_MS * 2 }).toContain('--jl-ok');
    expect(await navIconFill(page, 'project:')).toContain('--jl-ok');

    await page.locator('sc-nav .item[data-id^="session:"]').click();
    await settle(page);
    await page.getByText('Done.').waitFor();
    await expect.poll(() => navIconFill(page, 'session:')).toBeNull();
    expect(await navIconFill(page, 'project:')).toBeNull();
  });
});

/** Chat or Code, at the nav's foot. */
const modeRadio = (page: Page, name: string): Locator =>
  page.locator('sc-nav jl-segmented').getByRole('radio', { name });

describe('chats', () => {
  it('start from Chat at the nav’s foot, listed apart from the projects, the model reading the web alone', async () => {
    const rig = await start({ replies: [{ text: 'Paris.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').waitFor();
    await modeRadio(page, 'Chat').click();
    await page.evaluate('0');
    const composer = page.locator('new-chat-page sc-composer');
    await composer.locator('textarea').waitFor();
    expect(await composer.locator('textarea').getAttribute('placeholder')).toBe('Ask anything');
    await composer.locator('textarea').fill('What is the capital of France?');
    await composer.locator('textarea').press('Enter');
    await page.getByText('Paris.').waitFor();
    const request = JSON.stringify(rig.llama.requests[0]);
    expect(request).toContain('"name":"websearch"');
    expect(request).not.toContain('"name":"bash"');
    expect(await page.locator('session-page header > .chip').isHidden()).toBe(true);
    expect(await page.locator('sc-nav .item[data-id^="session:"]').count()).toBe(1);
    expect(await page.locator('sc-nav .item[data-id^="project:"]').count()).toBe(0);

    // swcode's link opens as the person left it: in Chat, on the new chat page.
    await page.goto(rig.link);
    await page.locator('new-chat-page textarea').waitFor();
    expect(await modeRadio(page, 'Chat').getAttribute('aria-checked')).toBe('true');

    await modeRadio(page, 'Code').click();
    await page.evaluate('0');
    await page.locator('home-page textarea').waitFor();
    expect(await page.locator('sc-nav .item[data-id^="session:"]').count()).toBe(0);
    await page.reload();
    await page.locator('home-page textarea').waitFor();
    expect(await modeRadio(page, 'Code').getAttribute('aria-checked')).toBe('true');
  });
});

describe('the theme', () => {
  it('follows a dark system, the mark’s ink turning light', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ colorScheme: 'dark', viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').waitFor();
    const background = await page.evaluate(BODY_BACKGROUND);
    expect(background).toBe('rgb(15, 17, 19)');
  });
});
