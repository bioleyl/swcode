import { afterEach, describe, expect, it } from 'vitest';

import { DESKTOP, PHONE, settle, start, stopRigs, tab } from './support.js';

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

describe('a tailnet', () => {
  it('gives swcode an HTTPS address with a press, which the QR code then leads to', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(`${rig.link}#/settings/phone`);
    const phone = page.locator('sc-phone');
    await phone.getByText('Tailscale can give swcode an HTTPS address', { exact: false }).waitFor();
    await expect.poll(() => phone.locator('.link').textContent()).toContain('http://127.0.0.1');

    await phone.locator('.serve').click();
    await phone.getByText('swcode is at https://laptop.tail1234.ts.net', { exact: false }).waitFor();
    await expect.poll(() => phone.locator('.link').textContent()).toContain('https://laptop.tail1234.ts.net:');
    expect(await phone.locator('.serve').isHidden()).toBe(true);
    expect(await phone.locator('.tailnet-problem').isHidden()).toBe(true);
  });
});

describe('another computer’s swcode', () => {
  it('is added from its pairing link, shown in this browser, and left again for the one the page came from', async () => {
    const home = await start({ replies: [{ text: 'From home.' }] });
    const other = await start({ replies: [{ text: 'From the other one.' }] });

    // The other computer has a session of its own, and shows the link that pairs with it.
    const there = await other.browser.newPage({ viewport: DESKTOP });
    await there.goto(other.link);
    await there.locator('home-page textarea').fill('Over there');
    await there.locator('home-page textarea').press('Enter');
    await there.getByText('From the other one.').waitFor();
    await there.goto(`${other.link}#/settings/computers`);
    const link = there.locator('sc-computers .pair .link');
    await expect.poll(() => link.textContent()).toContain('pair=');
    const pairing = (await link.textContent()) ?? '';

    // This browser pairs with it from the link alone, and lists it beside the computer its page came from.
    const page = await home.browser.newPage({ viewport: DESKTOP });
    await page.goto(`${home.link}#/settings/computers`);
    const computers = page.locator('sc-computers jl-list-item.computer');
    await computers.first().getByText('Shown here').waitFor();
    await page.locator('#computer-address input').fill(pairing);
    await page.locator('sc-computers .add').click();
    await page.locator('sc-computers jl-alert').getByText('Added', { exact: false }).waitFor();
    await expect.poll(() => computers.count()).toBe(2);
    // The link served once: the same one again is refused, with why.
    await page.locator('#computer-address input').fill(pairing);
    await page.locator('sc-computers .add').click();
    await page.locator('sc-computers jl-alert').getByText('refused the code', { exact: false }).waitFor();

    // Opened, the page shows the other computer's sessions, marked as another computer's.
    const where = page.locator('sc-computer-switch');
    await computers.nth(1).locator('.open').click();
    await where.getByText('Remote', { exact: true }).waitFor();
    await settle(page);
    await page.goto(`${new URL(page.url()).origin}/#/sessions`);
    await page.locator('sessions-page').getByText('Over there').first().click();
    await page.locator('session-page').getByText('From the other one.').waitFor();

    // The pill leads back to the computer the page came from, which has no session.
    await where.locator('jl-select select').selectOption('local');
    await where.getByText('Remote', { exact: true }).waitFor({ state: 'hidden' });
    await settle(page);
    await page.goto(`${new URL(page.url()).origin}/#/sessions`);
    await settle(page);
    await page.locator('sessions-page').waitFor();
    expect(await page.getByText('Over there').count()).toBe(0);
  });
});
