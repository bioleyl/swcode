import { afterEach, describe, expect, it } from 'vitest';

import { PHONE, settle, start, stopRigs, tab } from './support.js';

afterEach(stopRigs);

/** Run in the page: the manifest it names, and how each of its icons is served. */
const MANIFEST_AND_ICONS = `(async () => {
  const manifest = await (await fetch(document.querySelector('link[rel=manifest]').href)).json();
  const icons = await Promise.all(manifest.icons.map(async (icon) => {
    const served = await fetch(icon.src);
    return served.status + ' ' + served.headers.get('content-type');
  }));
  return JSON.stringify({ display: manifest.display, icons, start: manifest.start_url });
})()`;
const WORKER_READY = 'navigator.serviceWorker.ready.then(() => true)';
const WORKER_CONTROLS = 'navigator.serviceWorker.controller !== null';
const PAGE_KEPT = "caches.match('/').then((kept) => kept !== undefined)";

describe('on a phone', () => {
  it('swcode is an app to install: a manifest with its icons, and a worker that keeps the app’s own files', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: PHONE });
    await page.goto(rig.link);
    await settle(page);
    expect(JSON.parse(String(await page.evaluate(MANIFEST_AND_ICONS)))).toEqual({
      display: 'standalone',
      icons: ['200 image/png', '200 image/png', '200 image/png'],
      start: '/',
    });

    // The worker takes the page at its next load, and keeps the page itself for when the daemon is out of reach.
    expect(await page.evaluate(WORKER_READY)).toBe(true);
    await page.reload();
    await settle(page);
    await expect.poll(() => page.evaluate(WORKER_CONTROLS)).toBe(true);
    await expect.poll(() => page.evaluate(PAGE_KEPT)).toBe(true);
    await page.locator('home-page textarea').waitFor();
  });

  it('the settings say how to install it where the browser has no button for it', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: PHONE });
    await page.goto(rig.link);
    await tab(page, 'Settings').click();
    await settle(page);
    await page.locator('settings-page jl-tabs').getByText('Phone', { exact: true }).click();
    await settle(page);
    await page.locator('sc-phone').getByText('Add to Home Screen', { exact: false }).waitFor();
    expect(await page.locator('sc-phone .install-now').isHidden()).toBe(true);
    expect(new URL(page.url()).hash).toBe('#/settings/phone');
    // The topic has its own address, which opens on it.
    await page.reload();
    await page.locator('sc-phone').getByText('Add to Home Screen', { exact: false }).waitFor();
  });
});
