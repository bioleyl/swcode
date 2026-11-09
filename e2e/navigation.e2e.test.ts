import { basename } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { DESKTOP, navIconFill, navItem, settingsLink, settle, start, stopRigs } from './support.js';

import type { Locator, Page } from 'playwright-core';

afterEach(stopRigs);

// Long enough to see a session at work, and to leave it for another page before its turn ends.
const LIGHT_TURN_MS = 3_000;

/** Run on an element in the page: its opacity, as drawn. */
const opacityOf = (node: {
  ownerDocument: { defaultView: { getComputedStyle: (node: unknown) => { opacity: string } } | null };
}): string => node.ownerDocument.defaultView?.getComputedStyle(node).opacity ?? '';

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

  it('finds a session by what its conversation says, the words around shown under its title', async () => {
    const rig = await start({ replies: [{ text: 'The badge now reads green on main.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Tidy the README');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('The badge now reads green').waitFor();
    await navItem(page, 'New session').click();
    await settle(page);

    await page.keyboard.press('Control+k');
    const palette = page.locator('jl-command-palette');
    await palette.getByRole('combobox').fill('badge green');
    const found = palette.getByRole('option', { name: /Tidy the README/ });
    await found.waitFor();
    expect(await found.textContent()).toMatch(/swcode-e2e-workspace-\w+ · The badge now reads green on main\./);
    await page.keyboard.press('Enter');
    await settle(page);
    await page.locator('session-page').getByText('The badge now reads green').waitFor();
  });
});

describe('the navigation', () => {
  it('starts a session in a project from the + of its row, shown while the row is pointed at', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();

    const row = page.locator('sc-nav .item[data-depth="0"][data-id^="project:"]');
    const plus = page.locator('sc-nav .sc-new-session');
    const projectId = ((await row.getAttribute('data-id')) ?? '').replace('project:', '');
    expect(await plus.getAttribute('aria-label')).toBe(`New session in ${basename(rig.workspace)}`);
    expect(await plus.evaluate(opacityOf)).toBe('0');
    await row.hover();
    await expect.poll(() => plus.evaluate(opacityOf)).toBe('1');

    await plus.click();
    await settle(page);
    expect(new URL(page.url()).hash).toBe(`#/new/${projectId}`);
    await page.locator('home-page textarea').waitFor();
    expect(await page.locator('sc-nav .sc-new-session').count()).toBe(1);
  });

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

  it('empties on every device once the session is seen on one', async () => {
    const rig = await start({ replies: [{ delayMs: LIGHT_TURN_MS, text: 'Done.' }] });
    const computer = await rig.browser.newPage({ viewport: DESKTOP });
    await computer.goto(rig.link);
    await computer.locator('home-page textarea').fill('Hi');
    await computer.locator('home-page textarea').press('Enter');
    await expect.poll(() => navIconFill(computer, 'session:')).toContain('--jl-accent');
    await settingsLink(computer).click();
    await settle(computer);
    // A phone: a browser of its own, which never opened the session.
    const phone = await rig.browser.newPage({ viewport: DESKTOP });
    await phone.goto(rig.link);
    await settle(phone);
    await expect.poll(() => navIconFill(phone, 'session:'), { timeout: LIGHT_TURN_MS * 2 }).toContain('--jl-ok');

    await computer.locator('sc-nav .item[data-id^="session:"]').click();
    await settle(computer);
    await computer.getByText('Done.').waitFor();
    await expect.poll(() => navIconFill(phone, 'session:')).toBeNull();
    expect(await navIconFill(phone, 'project:')).toBeNull();
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
