import { readFileSync } from 'node:fs';

import { afterEach, describe, expect, it } from 'vitest';

import { E2E_VERSION } from './e2e.constants.js';
import { programOf } from './fake-releases.js';
import { DESKTOP, overflow, PHONE, settingsLink, settle, start, stopRigs, tab } from './support.js';

import type { Page } from 'playwright-core';

afterEach(stopRigs);

// Whether the page kept a token of its own, which a pairing code is traded for.
const KEPT_TOKEN = "localStorage.getItem('swcode.token') !== null";

const GMKTEC = { address: 'http://gmktec:4500', id: 'g1', name: 'gmktec', reachable: true };
const OFFLINE = { address: 'http://offline:4500', id: 'o1', name: 'offline', reachable: false };

/**
 * Run in the page before it loads: the desktop app around it, as WebView2 lets the page talk to it, showing a
 * computer among those it knows. It keeps what the page asked, sends each message back as WebView2 does, and pairs
 * with gmktec on the code 123456. Opening one that is not reachable answers as the app does: it did not answer.
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
      const opened = request.type === 'openComputer' ? computers.find((computer) => computer.id === request.id) : undefined;
      if (opened && !opened.reachable) {
        answer({ id: opened.id, name: opened.name, type: 'computerUnreachable' });
        return;
      }
      answer({ computers, current: '${current}', type: 'computers' });
    },
  };
})();`;

/** A topic's tab on the settings page, by its label; `selected` the one shown. */
const topicTab = (page: Page, label: string, selected?: boolean) =>
  page
    .locator('settings-page jl-tabs')
    .getByRole('tab', { exact: true, name: label, ...(selected ? { selected } : {}) });

describe('settings', () => {
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

  it('leave Startup out in a browser, and set how the Windows app starts with Windows', async () => {
    const rig = await start({ replies: [] });
    const browser = await rig.browser.newPage({ viewport: DESKTOP });
    await browser.goto(`${rig.link}#/settings/models`);
    await topicTab(browser, 'Models', true).waitFor();
    await topicTab(browser, 'Startup').waitFor({ state: 'detached' });
    await topicTab(browser, 'Computers').waitFor();

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

describe('updates', () => {
  const release = { notes: '## Highlights\n\n- swcode updates itself.', version: '0.2.0' };
  const offer = 'swcode 0.2.0 is out';
  const iphone =
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148';

  it('are offered once on a computer, then installed from the settings in place of the program', async () => {
    const rig = await start({ release, replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.getByText(offer).waitFor();
    await page.reload();
    await settingsLink(page).click();
    await settle(page);
    await topicTab(page, 'Updates').click();
    await settle(page);
    const updates = page.locator('sc-update-settings');
    await updates.getByRole('heading', { exact: true, name: 'swcode 0.2.0' }).waitFor();
    expect(await page.getByText(offer).count()).toBe(0);
    expect(await updates.getByText(E2E_VERSION).isVisible()).toBe(true);
    expect(await updates.getByText('swcode updates itself.').isVisible()).toBe(true);
    await updates.getByRole('button', { name: 'Install 0.2.0' }).click();
    await updates.getByText('swcode 0.2.0 is installed: restart swcode for it to run.').waitFor();
    expect(readFileSync(rig.program, 'utf8')).toBe(programOf(release));
  });

  it('are never offered on a phone, whose settings have no Updates', async () => {
    const rig = await start({ release, replies: [] });
    const page = await rig.browser.newPage({ userAgent: iphone, viewport: PHONE });
    await page.goto(rig.link);
    await tab(page, 'Settings').click();
    await settle(page);
    await topicTab(page, 'Models', true).waitFor();
    expect(await topicTab(page, 'Updates').count()).toBe(0);
    expect(await page.getByText(offer).count()).toBe(0);
  });
});

describe('the computer', () => {
  it('is this one in a browser on it, the pill leading to the settings that add others', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    const where = page.locator('sc-nav sc-computer-switch');
    const computers = where.locator('jl-select select');
    await expect
      .poll(() => computers.locator('option').allTextContents())
      .toEqual(['This computer', 'Manage computers…']);
    expect(await computers.inputValue()).toBe('local');
    expect((await where.getAttribute('class')) ?? '').not.toContain('remote');
    await computers.selectOption('manage');
    await settle(page);
    expect(await computers.inputValue()).toBe('local');
    await page.locator('sc-computers').getByText('Add a computer').waitFor();
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

  it('stays on the one it shows when the computer picked does not answer, and says which', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.addInitScript(fakeDesktopApp('local', [OFFLINE]));
    await page.goto(rig.link);
    const where = page.locator('sc-nav sc-computer-switch');
    const computers = where.locator('jl-select select');
    await where.locator('option[value="o1"]').waitFor({ state: 'attached' });

    await computers.selectOption('o1');
    await page.getByText('offline did not answer').waitFor();
    expect(await computers.inputValue()).toBe('local');
    expect(await page.getByText('Is swcode running there, and listening on the network?').isVisible()).toBe(true);
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
