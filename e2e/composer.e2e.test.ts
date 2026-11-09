import { basename } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import {
  DESKTOP,
  EDIT_FLOW,
  GREET,
  LAPTOP,
  navIconFill,
  navItem,
  overflow,
  PHONE,
  projectTab,
  settle,
  start,
  stopRigs,
  tab,
} from './support.js';

import type { Locator, Page } from 'playwright-core';

afterEach(stopRigs);

// A window too short for the help dialog, whose body then scrolls.
const SHORT_DESKTOP = { height: 360, width: 1440 };

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

// Long enough to open the game and start it; the turn then ends while it runs.
const GAME_TURN_MS = 5_000;

/** Thinks a few seconds, then answers: the game pauses as the turn ends. */
const GAME_FLOW = {
  files: { 'src/greet.ts': GREET },
  replies: [{ delayMs: GAME_TURN_MS, text: 'Done.' }],
};

// What the daemon asks to have the conversation summarized.
const SUMMARY_REQUEST = 'Summarize the conversation below';

/**
 * Run in the page before it loads: a speech recognizer that hears "fix the login bug" in two results, the first a
 * guess still, and ends once stopped; or, given a failure, fails with it as it starts. It keeps the language it was
 * asked to listen in.
 */
const fakeRecognizer = (failure = ''): string => `(() => {
  // Chromium has the recognizer under both names now: the page takes the unprefixed one first.
  window.SpeechRecognition = window.webkitSpeechRecognition = class {
    start() {
      window.dictationLanguage = this.lang;
      if ('${failure}') {
        setTimeout(() => {
          this.onerror?.({ error: '${failure}' });
          this.onend?.();
        }, 10);
        return;
      }
      setTimeout(() => this.onresult?.({ results: [[{ transcript: 'fix the' }]] }), 10);
      setTimeout(() => this.onresult?.({ results: [[{ transcript: 'fix the' }], [{ transcript: ' login bug' }]] }), 20);
    }
    stop() {
      setTimeout(() => this.onend?.(), 10);
    }
    abort() {
      setTimeout(() => this.onend?.(), 10);
    }
  };
})();`;

const NO_RECOGNIZER = 'window.webkitSpeechRecognition = undefined; window.SpeechRecognition = undefined;';

/** Run on an element in the page: whether what it holds is taller than it, so that it scrolls. */
const scrolls = (node: { clientHeight: number; scrollHeight: number }): boolean =>
  node.scrollHeight > node.clientHeight;

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

describe('dictation', () => {
  it('writes what is said after the words typed, in the browser’s language, until the microphone is pressed again', async () => {
    const rig = await start(EDIT_FLOW);
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.addInitScript(fakeRecognizer());
    await page.goto(rig.link);
    const composer = page.locator('home-page sc-composer');
    await composer.locator('textarea').fill('Please');

    await composer.getByRole('button', { name: 'Dictate' }).click();
    await page.evaluate('0');
    await expect.poll(() => composer.locator('textarea').inputValue()).toBe('Please fix the login bug');
    await composer.getByRole('button', { name: 'Stop dictating' }).click();
    await page.evaluate('0');
    await composer.getByRole('button', { name: 'Dictate' }).waitFor();
    expect(await page.evaluate('window.dictationLanguage')).toBe(await page.evaluate('navigator.language'));
  });

  it('says why it stopped when the microphone is refused', async () => {
    const rig = await start(EDIT_FLOW);
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.addInitScript(fakeRecognizer('not-allowed'));
    await page.goto(rig.link);
    const composer = page.locator('home-page sc-composer');

    await composer.getByRole('button', { name: 'Dictate' }).click();
    await page.evaluate('0');
    await page.getByText('The microphone is not allowed', { exact: false }).waitFor();
    await composer.getByRole('button', { name: 'Dictate' }).waitFor();
  });

  it('has no microphone where the browser recognizes no speech', async () => {
    const rig = await start(EDIT_FLOW);
    const page = await rig.browser.newPage({ viewport: LAPTOP });
    await page.addInitScript(NO_RECOGNIZER);
    await page.goto(rig.link);
    const composer = page.locator('home-page sc-composer');
    // The chip's words are written as the composer connects, when it also decides on the microphone.
    await composer.locator('.summary', { hasText: /\S/ }).waitFor();

    expect(await composer.locator('.dictate').isHidden()).toBe(true);
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

describe('the commands', () => {
  it('list as a slash is typed, the arrows choosing, and /help shows those of the session', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('/');
    await settle(page);
    expect(await page.locator('home-page sc-composer .command-menu').isHidden()).toBe(true);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();

    const composer = page.locator('session-page sc-composer');
    const field = composer.locator('textarea');
    await field.pressSequentially('/');
    const options = composer.locator('.command-menu [role="option"]');
    await options.first().waitFor();
    expect(await options.locator('.name').allTextContents()).toEqual([
      '/new',
      '/compact',
      '/todo ‹todo›',
      '/peek ‹question›',
      '/help',
    ]);
    expect(await field.getAttribute('aria-activedescendant')).toBe('command-new');
    await field.press('ArrowUp');
    expect(await field.getAttribute('aria-activedescendant')).toBe('command-help');
    expect(await composer.locator('[aria-selected="true"] .name').textContent()).toBe('/help');
    await field.press('Escape');
    expect(await composer.locator('.command-menu').isHidden()).toBe(true);
    await field.pressSequentially('h');
    expect(await options.count()).toBe(1);
    await field.press('Enter');
    const help = composer.locator('sc-command-help jl-dialog');
    await help.getByText('/compact', { exact: true }).waitFor();
    expect(await help.locator('.commands code').allTextContents()).toEqual([
      '/new',
      '/compact',
      '/todo ‹todo›',
      '/peek ‹question›',
      '/help',
    ]);
    // In a short window only the dialog's body scrolls: the keycaps' spoken words, in absolute spans, stay inside it
    // rather than giving the dialog a second scrollbar.
    await page.setViewportSize(SHORT_DESKTOP);
    await page.evaluate('0');
    expect(await help.locator('.body').evaluate(scrolls)).toBe(true);
    expect(await help.locator('dialog').evaluate(scrolls)).toBe(false);
    expect(await field.inputValue()).toBe('');
    expect(rig.llama.requests.length).toBe(1);
  });

  it('/compact summarizes the conversation now, and Tab completes /peek for its question', async () => {
    const rig = await start({
      replies: [{ text: 'Hello there.' }],
      scripts: { [SUMMARY_REQUEST]: [{ text: 'The user greeted the agent.' }] },
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();

    const composer = page.locator('session-page sc-composer');
    const field = composer.locator('textarea');
    await field.pressSequentially('/pe');
    await field.press('Tab');
    expect(await field.inputValue()).toBe('/peek ');
    expect(await composer.locator('.command-menu').isHidden()).toBe(true);

    await field.fill('');
    await field.pressSequentially('/co');
    await composer.locator('.command-menu [role="option"]').first().waitFor();
    await field.press('Enter');
    const conversation = page.locator('session-page sc-conversation');
    await conversation.getByText('Earlier messages were summarized to save context').waitFor();
    expect(JSON.stringify(rig.llama.requests.at(-1))).toContain(SUMMARY_REQUEST);
    expect(rig.events().filter((event) => event.kind === 'contextCompacted')).toHaveLength(1);
  });

  it('/new opens a new session with the same settings', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();
    const first = page.url();
    const summary = page.locator('session-page sc-composer .summary');
    const settings = await summary.textContent();

    const field = page.locator('session-page sc-composer textarea');
    await field.fill('/new');
    await field.press('Enter');
    await expect.poll(() => page.url()).not.toBe(first);
    await settle(page);
    expect(await page.locator('session-page sc-conversation').getByText('Hello there.').count()).toBe(0);
    expect(await summary.textContent()).toBe(settings);
  });

  it('/todo adds the words to the project of the folder the session works in', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();

    const field = page.locator('session-page sc-composer textarea');
    await field.fill('/todo Write the tests');
    await field.press('Enter');
    await page.getByText('Todo added to', { exact: false }).waitFor();
    expect(rig.events().some((event) => event.kind === 'userMessage' && event.text.includes('/todo'))).toBe(false);

    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await projectTab(page, 'Todos').click();
    await settle(page);
    await page.locator('project-page sc-todo-row').getByText('Write the tests').waitFor();
  });
});
