import { afterEach, describe, expect, it } from 'vitest';

import { DESKTOP, overflow, settle, start, stopRigs } from './support.js';

afterEach(stopRigs);

// What the terminal shows once the shell ran what was typed in it.
const ROWS = 'session-page sc-terminal .xterm-rows';
// xterm's ink, read through the app shell's and the page's shadow roots.
const INK = `(() => {
  const page = document.querySelector('sc-app-shell').shadowRoot.querySelector('session-page');
  const terminal = page.shadowRoot.querySelector('sc-terminal');
  return getComputedStyle(terminal.shadowRoot.querySelector('.xterm')).color;
})()`;

describe('a terminal of the project', () => {
  it('opens in a dialog titled by its name, keeps each terminal’s own screen, and takes a new name', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();
    await settle(page);

    const panel = page.locator('session-page sc-side-panel');
    const dialog = page.locator('session-page sc-terminal jl-dialog');
    await panel.getByRole('button', { name: 'New terminal' }).click();
    await page.evaluate('0');
    // The dialog shows the terminal's name; the side panel renames it in place.
    await expect.poll(async () => dialog.isVisible()).toBe(true);
    expect(await dialog.getByRole('heading', { level: 2 }).textContent()).toBe('Terminal');
    await dialog.getByRole('button', { exact: true, name: 'Close' }).click();
    await page.evaluate('0');
    await panel.locator('.terminals .terminal .rename').click();
    await panel.locator('.terminals .terminal .name').fill('dev server');
    await page.keyboard.press('Enter');
    await expect.poll(async () => panel.getByRole('button', { exact: true, name: 'dev server' }).count()).toBe(1);
    await panel.getByRole('button', { exact: true, name: 'dev server' }).click();
    await page.evaluate('0');
    await expect.poll(async () => dialog.getByRole('heading', { level: 2 }).textContent()).toBe('dev server');

    await page.locator('session-page sc-terminal .xterm').click();
    await page.keyboard.type('echo first-terminal');
    await page.keyboard.press('Enter');
    await expect.poll(async () => page.locator(ROWS).textContent()).toContain('first-terminal');

    // The ink follows the page's theme while the terminal stays open.
    const light = await page.evaluate(INK);
    await page.evaluate(`document.documentElement.setAttribute('data-theme', 'dark')`);
    await page.evaluate('0');
    expect(await page.evaluate(INK)).not.toBe(light);
    await page.evaluate(`document.documentElement.removeAttribute('data-theme')`);

    // A second terminal starts on a screen of its own, none of the first one's output in it.
    await dialog.getByRole('button', { exact: true, name: 'Close' }).click();
    await page.evaluate('0');
    await panel.getByRole('button', { name: 'New terminal' }).click();
    await page.evaluate('0');
    await expect.poll(async () => dialog.getByRole('heading', { level: 2 }).textContent()).toBe('Terminal');
    expect(await page.locator(ROWS).textContent()).not.toContain('first-terminal');
    await page.keyboard.type('echo second-terminal');
    await page.keyboard.press('Enter');
    await expect.poll(async () => page.locator(ROWS).textContent()).toContain('second-terminal');

    // Back on the first, its own screen comes back, without the second one's lines.
    await dialog.getByRole('button', { exact: true, name: 'Close' }).click();
    await page.evaluate('0');
    await panel.getByRole('button', { exact: true, name: 'dev server' }).click();
    await page.evaluate('0');
    await expect.poll(async () => page.locator(ROWS).textContent()).toContain('first-terminal');
    expect(await page.locator(ROWS).textContent()).not.toContain('second-terminal');

    // A page reloaded asks for the terminal again: what it printed so far comes with it.
    await page.reload();
    await settle(page);
    await panel.getByRole('button', { exact: true, name: 'dev server' }).click();
    await page.evaluate('0');
    await expect.poll(async () => page.locator(ROWS).textContent()).toContain('first-terminal');

    // The dialog's own close button leaves the view, the shell staying there for the next visit: Escape is the
    // terminal's, and goes to the shell.
    await dialog.getByRole('button', { exact: true, name: 'Close' }).click();
    await page.evaluate('0');
    expect(await dialog.isVisible()).toBe(false);
    expect(await page.locator('session-page sc-conversation').isVisible()).toBe(true);

    await panel.locator('.terminals .terminal .close').first().click();
    await page.evaluate('0');
    await expect.poll(async () => panel.locator('.terminals .terminal').count()).toBe(1);
  });
});

describe('a command that asks for a password', () => {
  it('runs in a terminal of the app, where the person types it, and the agent goes on once it ends', async () => {
    const rig = await start({
      replies: [
        {
          toolCalls: [
            {
              arguments: {
                command: 'read -r -s -p "Password: " secret; echo; echo "it was $secret"',
                title: 'setup',
                wait: true,
              },
              name: 'terminal',
            },
          ],
        },
        { text: 'Installed.' },
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Install it');
    await page.locator('home-page textarea').press('Enter');

    // The call says where its command runs, and the session that it needs its person once the command asks.
    const card = page.locator('session-page sc-terminal-wait');
    await card.getByText('Runs in the terminal “setup”').waitFor();
    await page.locator('session-page header').getByText('Needs you').waitFor();
    expect(await overflow(page)).toBe(0);

    // The password is typed in the terminal itself, which shows none of it.
    await card.getByRole('button', { name: 'Open the terminal' }).click();
    const dialog = page.locator('session-page sc-terminal jl-dialog');
    await expect.poll(async () => dialog.isVisible()).toBe(true);
    await expect.poll(async () => page.locator(ROWS).textContent()).toContain('Password:');
    await page.locator('session-page sc-terminal .xterm').click();
    await page.keyboard.type('hunter2');
    expect(await page.locator(ROWS).textContent()).not.toContain('hunter2');
    await page.keyboard.press('Enter');
    await expect.poll(async () => page.locator(ROWS).textContent()).toContain('it was hunter2');
    // The terminal shows no trace of how the shell told swcode the command ended.
    expect(await page.locator(ROWS).textContent()).not.toContain('777;swcode');
    await dialog.getByRole('button', { exact: true, name: 'Close' }).click();

    // The agent went on by itself, and the card left with the call's result.
    await page.getByText('Installed.').waitFor();
    expect(await card.count()).toBe(0);
    expect(await page.locator('session-page sc-tool-row').getAttribute('state')).toBe('done');
  });
});
