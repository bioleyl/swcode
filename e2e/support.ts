import { expect } from 'vitest';

import { startRig } from './rig.js';

import type { Page } from 'playwright-core';
import type { E2eRig } from './types/e2e.types.js';

export const DESKTOP = { height: 900, width: 1440 };
// Wide enough for the side panel, which leaves the composer 650 pixels.
export const LAPTOP = { height: 900, width: 1280 };
export const PHONE = { height: 844, width: 390 };
export const GREET = 'export const greet = (name: string) => "Hello, " + name;\n';

/** Reads greet.ts, asks to edit it, then answers: the flow the session tests start from. */
export const EDIT_FLOW = {
  files: { 'src/greet.ts': GREET },
  replies: [
    { toolCalls: [{ arguments: { path: 'src/greet.ts' }, name: 'read' }] },
    { toolCalls: [{ arguments: { newString: 'Hi', oldString: 'Hello', path: 'src/greet.ts' }, name: 'edit' }] },
    { text: 'Done: it says **Hi** now.' },
  ],
};

const rigs: E2eRig[] = [];

export const start = async (options: Parameters<typeof startRig>[0]): Promise<E2eRig> => {
  const rig = await startRig(options);
  rigs.push(rig);
  return rig;
};

/** Stops every rig the file started: each test file registers it with `afterEach`. */
export const stopRigs = async (): Promise<void> => {
  await Promise.all(rigs.splice(0).map((rig) => rig.stop()));
};

// Run in the page: the tests are typed for Node, and these few expressions need no more.
const PAGE_OVERFLOW = 'document.documentElement.scrollWidth - window.innerWidth';

/** How much wider than the screen the page is: nothing should ever scroll sideways. */
export const overflow = (page: Page) => page.evaluate(PAGE_OVERFLOW);

/** An item of the navigation down the side, by its label. */
export const navItem = (page: Page, label: string) =>
  page.locator('sc-nav jl-side-nav').getByText(label, { exact: true });

/** The settings' cog, at the foot of the navigation. */
export const settingsLink = (page: Page) => page.locator('sc-nav .settings');

/** The style of the first shape of a navigation item's icon, by the start of its id: its light's fill, or none. */
export const navIconFill = (page: Page, id: string) =>
  page.locator(`sc-nav .item[data-id^="${id}"] svg > *`).first().getAttribute('style');

/** A tab of the project page, by its label. */
export const projectTab = (page: Page, label: string) =>
  page.locator('project-page jl-tabs').getByRole('tab', { exact: true, name: label });

/** A tab of a phone's bar under the page, by its label. */
export const tab = (page: Page, label: string) => page.locator('jl-tab-bar').getByText(label, { exact: true });

// Two frames: the router has the next page, and the outlet decided when it shows.
const NEXT_FRAMES = 'new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))';
// The pages the router shows.
const PAGES = 'home-page, session-page, sessions-page, project-page, settings-page';

/**
 * A click is delivered on the next command in headless Chromium; a page it opens shows once it has its data, the
 * one before staying meanwhile (jlib's page outlet): both are in the document until then.
 */
export const settle = async (page: Page) => {
  await page.evaluate('0');
  await page.evaluate(NEXT_FRAMES);
  await expect.poll(() => page.locator(PAGES).count()).toBe(1);
};

/** In a session that asks before edits, picked in the composer's panel: new ones run everything. */
export async function askForTheEdit(page: Page): Promise<void> {
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
