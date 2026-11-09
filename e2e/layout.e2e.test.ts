import { afterEach, describe, expect, it } from 'vitest';

import { E2E_VERSION } from './e2e.constants.js';
import {
  askForTheEdit,
  DESKTOP,
  EDIT_FLOW,
  LAPTOP,
  overflow,
  PHONE,
  settle,
  start,
  stopRigs,
  tab,
} from './support.js';

afterEach(stopRigs);

// Far more than a window's height of paragraphs, and a turn of the wheel up through them.
const PARAGRAPHS = 80;
const LONG_REPLY = Array.from({ length: PARAGRAPHS }, (_, index) => `Paragraph ${index + 1}.`).join('\n\n');
const WHEEL_UP = -400;

const BODY_BACKGROUND = 'getComputedStyle(document.body).backgroundColor';

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
    // The sheet ends where the tab bar begins, none of it under the bar.
    const sheet = await page.locator('sc-permission .card').boundingBox();
    const tabs = await page.locator('jl-tab-bar').boundingBox();
    expect(Math.round((sheet?.y ?? 0) + (sheet?.height ?? 0))).toBe(Math.round(tabs?.y ?? 0));
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
    expect(await page.locator('session-page sc-side-panel .version').textContent()).toBe(`swcode ${E2E_VERSION}`);
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
