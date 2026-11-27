import { readFileSync } from 'node:fs';

import { afterEach, describe, expect, it } from 'vitest';

import { E2E_VERSION } from './e2e.constants.js';
import { programOf } from './fake-releases.js';
import {
  DESKTOP,
  overflow,
  PHONE,
  settingsLink,
  settle,
  shownPairNumber,
  start,
  stopRigs,
  tab,
} from './support.js';

import type { Page } from 'playwright-core';

afterEach(stopRigs);

// Whether the page kept a token of its own, which pairing a number gives it.
const KEPT_TOKEN = "localStorage.getItem('swcode.token') !== null";

const GMKTEC = { address: 'http://gmktec:4500', id: 'g1', name: 'gmktec', reachable: true };
const OFFLINE = { address: 'http://offline:4500', id: 'o1', name: 'offline', reachable: false };

/**
 * Run in the page before it loads: the desktop app around it, as WebView2 lets the page talk to it, showing a
 * computer among those it knows. It keeps what the page asked, sends each message back as WebView2 does, and keeps
 * gmktec once its pairing gave the page the token it expects. Opening one that is not reachable answers as the app
 * does: it did not answer.
 */
const fakeDesktopApp = (current = 'local', others: readonly object[] = []): string => `(() => {
  const listeners = [];
  const computers = [{ address: 'http://localhost:4500', id: 'local', name: 'This computer', reachable: true }, ...${JSON.stringify(others)}];
  const send = (text) => setTimeout(() => listeners.forEach((listener) => listener({ data: text })), 10);
  const answer = (message) => send(JSON.stringify(message));
  window.desktopAsked = [];
  let notifications = true;
  window.chrome = window.chrome || {};
  window.chrome.webview = {
    addEventListener: (type, listener) => listeners.push(listener),
    removeEventListener: (type, listener) => listeners.splice(listeners.indexOf(listener), 1),
    postMessage: (text) => {
      window.desktopAsked.push(text);
      send(text);
      const request = JSON.parse(text);
      if (request.type === 'addComputer' && request.token !== 'paired-token') {
        answer({ message: 'That computer refused the token its pairing gave.', type: 'computerRefused' });
        return;
      }
      if (request.type === 'addComputer') {
        computers.push(${JSON.stringify(GMKTEC)});
      }
      // The tray's notifications are the app's own answer, kept between the pages it shows.
      if (request.type === 'setNotifications') {
        notifications = request.on;
      }
      if (request.type === 'notifications' || request.type === 'setNotifications') {
        answer({ on: notifications, type: 'notifications' });
        return;
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
    .locator('settings-page jl-page > jl-tabs')
    .getByRole('tab', { exact: true, name: label, ...(selected ? { selected } : {}) });

/** A tab of a topic's own row, under it: the models' four, or the network's three. */
const subTab = (page: Page, row: '.model-tabs' | '.network-tabs', label: string) =>
  page.locator(`settings-page ${row}`).getByRole('tab', { exact: true, name: label });

const saveButton = (page: Page) => page.locator('settings-page jl-page > jl-button.save');

describe('settings', () => {
  it('open on the models, and keep what the form holds from one topic and tab to the other', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await settingsLink(page).click();
    await settle(page);
    await topicTab(page, 'Models', true).waitFor();
    // The llm-servers save as they change: Save belongs to the settings file, under its own tabs.
    expect(await saveButton(page).isHidden()).toBe(true);
    await subTab(page, '.model-tabs', 'Image server').click();
    await settle(page);
    expect(await saveButton(page).isVisible()).toBe(true);
    await page.locator('#setting-SWCODE_IMAGE_URL input').fill('http://garfield:8090');
    await topicTab(page, 'Network').click();
    await settle(page);
    await subTab(page, '.network-tabs', 'Network').click();
    await settle(page);
    expect(await page.locator('#setting-SWCODE_IMAGE_URL').isHidden()).toBe(true);
    expect(await page.locator('#setting-SWCODE_PORT').isVisible()).toBe(true);
    expect(page.url()).toContain('#/settings/network');
    await topicTab(page, 'Models').click();
    await subTab(page, '.model-tabs', 'Image server').click();
    await settle(page);
    expect(await page.locator('#setting-SWCODE_IMAGE_URL input').inputValue()).toBe('http://garfield:8090');
  });

  it('show how the agent’s browser opens, under a Browser topic', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await settingsLink(page).click();
    await settle(page);
    await topicTab(page, 'Browser').click();
    await settle(page);
    const control = page.locator('#setting-SWCODE_BROWSER_HEADFUL select');
    await control.waitFor();
    await control.selectOption({ label: 'Visible window' });
    expect(await control.inputValue()).toBe('on');
    expect(await saveButton(page).isVisible()).toBe(true);
  });

  it('list the SSH hosts, one added by name, changed and removed from its dialog', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await settingsLink(page).click();
    await settle(page);
    await topicTab(page, 'Network').click();
    await settle(page);
    await subTab(page, '.network-tabs', 'SSH').click();
    await settle(page);
    const hosts = page.locator('sc-ssh-hosts');
    await hosts.getByText('No SSH host yet', { exact: false }).waitFor();
    expect(await saveButton(page).isHidden()).toBe(true);

    await hosts.locator('.add').click();
    await settle(page);
    await page.locator('sc-ssh-host-dialog .host input').fill('me@build box');
    await page.locator('sc-ssh-host-dialog .save').click();
    await page.locator('sc-ssh-host-dialog').getByText('One host, without spaces or commas.').waitFor();
    await page.locator('sc-ssh-host-dialog .host input').fill('me@build-box');
    await page.locator('sc-ssh-host-dialog .name input').fill('build box');
    await page.locator('sc-ssh-host-dialog .save').click();
    await page.locator('sc-ssh-host-dialog').getByText('A name without spaces, commas or =.').waitFor();
    await page.locator('sc-ssh-host-dialog .name input').fill('buildbox');
    await page.locator('sc-ssh-host-dialog .save').click();
    await hosts.locator('.hosts jl-list-item', { hasText: 'buildbox' }).waitFor();
    await hosts.locator('jl-alert').getByText('Saved', { exact: false }).waitFor();

    // A host is changed from its row: the name stays, the endpoint it reaches changes.
    await hosts
      .locator('.hosts jl-list-item', { hasText: 'buildbox' })
      .getByRole('button', { name: 'Change' })
      .click();
    await settle(page);
    await page.locator('sc-ssh-host-dialog .host input').fill('me@other-box');
    await page.locator('sc-ssh-host-dialog .save').click();
    await hosts.locator('.hosts').getByText('me@other-box').waitFor();

    await hosts
      .locator('.hosts jl-list-item', { hasText: 'buildbox' })
      .getByRole('button', { name: 'Remove' })
      .click();
    await settle(page);
    await page.locator('jl-confirm').getByRole('button', { name: 'Remove' }).click();
    await hosts.getByText('No SSH host yet', { exact: false }).waitFor();
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

    await subTab(page, '.model-tabs', 'Presets').click();
    await settle(page);
    await editor.locator('.presets jl-list-item').first().waitFor();
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

  it('offer no install in the desktop app, and its own notifications turned off and on there', async () => {
    const rig = await start({ reachable: true, replies: [] });
    const app = await rig.browser.newPage({ viewport: DESKTOP });
    await app.addInitScript(fakeDesktopApp());
    await app.goto(`${rig.link}#/settings/computers`);
    const phone = app.locator('sc-computers');
    await phone.getByText('Open it on a phone', { exact: true }).waitFor();
    expect(await phone.locator('.install').isHidden()).toBe(true);
    // The tray notifies through Windows: the switch is the app's own answer, which it keeps.
    await phone.getByRole('button', { name: 'Stop notifying this device' }).click();
    await app.evaluate('0');
    await phone.getByRole('button', { name: 'Notify me on this device' }).waitFor();

    // A browser has both: there, swcode is a page until it is installed.
    const browser = await rig.browser.newPage({ viewport: DESKTOP });
    await browser.goto(rig.link);
    await browser.goto(`${rig.link.split('?')[0]}#/settings/computers`);
    // The page before stays until the settings have their data: one page, then its sections.
    await settle(browser);
    await browser.locator('sc-computers').getByText('Install on this device', { exact: true }).waitFor();
    await browser.locator('sc-computers').getByText('Notifications on this device', { exact: true }).waitFor();
  });

  it('list the desktop app’s computers, pair another one with its number, and open it', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.addInitScript(fakeDesktopApp());
    await page.goto(`${rig.link}#/settings/computers`);
    // The devices paired with this swcode come first; the computers this app may show are the second tab.
    await page.locator('sc-computers jl-tabs').getByRole('tab', { name: 'Connect a device' }).click();
    await settle(page);
    const computers = page.locator('sc-computers jl-list-item.computer');
    await computers.first().getByText('This computer', { exact: true }).filter({ visible: true }).waitFor();
    await computers.first().getByText('Shown here').waitFor();

    // That computer answers the pairing route as swcode does: first refusing the number, then giving a token.
    let refuse = true;
    // That computer answers: asked before a number is shown for it.
    await page.route('http://gmktec:4500/api/health', (route) => route.fulfill({ body: '{"ok":true}' }));
    await page.route('http://gmktec:4500/api/pairing', (route) =>
      route.fulfill({
        body: JSON.stringify(
          refuse ? { message: 'This pairing number does not wait here.' } : { token: 'paired-token' }
        ),
        contentType: 'application/json',
        status: refuse ? 400 : 200,
      })
    );

    await page.locator('#computer-name input').fill('gmktec');
    await page.locator('#computer-address input').fill('gmktec');
    await page.locator('sc-computers .add').click();
    await page.locator('sc-computers jl-alert').getByText('does not wait here', { exact: false }).waitFor();

    refuse = false;
    await page.locator('sc-computers .add').click();
    await page.locator('sc-computers jl-alert').getByText('Added', { exact: false }).waitFor();
    // Paired, it is named and coloured in its own dialog, which opens on it.
    await page.locator('sc-computers .edit').getByText('Edit gmktec', { exact: true }).waitFor();
    await page.locator('sc-computers .edit-cancel').click();
    await settle(page);
    expect(await computers.count()).toBe(2);
    expect(await page.locator('#computer-address input').inputValue()).toBe('');

    await computers.nth(1).locator('.open').click();
    await settle(page);
    const asked = await page.evaluate('window.desktopAsked');
    expect(asked).toContainEqual('{"id":"g1","type":"openComputer"}');
    expect(asked).toContainEqual(
      '{"address":"http://gmktec:4500","name":"gmktec","token":"paired-token","type":"addComputer"}'
    );
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

  it('are the Android app’s own on a phone, which takes its release from GitHub', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: PHONE });
    // What Capacitor's bridge sets in the Android app, which its core reads as the platform.
    await page.addInitScript('window.androidBridge = {};');
    await page.goto(`${rig.link}#/settings/updates`);
    await topicTab(page, 'Updates', true).waitFor();
    const app = page.locator('sc-app-update-settings');
    await app.getByText('Look now', { exact: true }).waitFor();
    expect(await page.locator('sc-update-settings').count()).toBe(0);
    expect(await overflow(page)).toBe(0);
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
    await page.locator('sc-computers jl-tabs').getByRole('tab', { name: 'Connect a device' }).click();
    await settle(page);
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

  it('works only with other computers in the Android app, which has no swcode of its own', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: PHONE });
    // What Capacitor's bridge sets in the Android app, which its core reads as the platform.
    await page.addInitScript('window.androidBridge = {};');
    await page.goto(rig.link);
    const where = page.locator('sc-nav sc-computer-switch');
    await expect
      .poll(() => where.locator('jl-select select option').allTextContents())
      .toEqual(['Manage computers…']);
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
  it('says so where swcode listens on its computer alone, with no code to scan and no number to type', async () => {
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(`${rig.link}#/settings/computers`);
    const computers = page.locator('sc-computers');
    await computers.getByText('Other devices cannot reach this swcode', { exact: true }).waitFor();
    expect(await computers.locator('.reach').isHidden()).toBe(true);
    expect(await computers.locator('.pair-input input').isDisabled()).toBe(true);
    expect(await computers.getByRole('button', { name: 'Pair the device' }).isDisabled()).toBe(true);

    await computers.getByRole('button', { name: 'Network settings' }).click();
    await settle(page);
    expect(page.url()).toContain('#/settings/network');
  });

  it('shows a number the phone types here: the phone gets in, and the number works once', async () => {
    const rig = await start({ reachable: true, replies: [] });
    const computer = await rig.browser.newPage({ viewport: DESKTOP });
    await computer.goto(rig.link);
    await settingsLink(computer).click();
    await settle(computer);
    // Startup goes once the daemon says it cannot: the tabs shift when it does.
    await topicTab(computer, 'Startup').waitFor({ state: 'detached' });
    await topicTab(computer, 'Computers').click();
    await settle(computer);
    expect(computer.url()).toContain('#/settings/computers');
    // The address a phone on the network reaches: the one the test's daemon was given.
    const address = await computer.locator('sc-computers .reach .link').textContent();
    const phone = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await phone.goto(address ?? '');
    const number = await shownPairNumber(phone);
    // The status is asked again every fifteen seconds: the number the person reads stays the one they type.
    await new Promise((resolve) => setTimeout(resolve, 17_000));
    expect(await shownPairNumber(phone)).toBe(number);
    // The number is typed in the computer's own page: Settings → Computers, the first tab.
    await topicTab(computer, 'Computers').click();
    await settle(computer);
    await computer.locator('sc-computers .pair-input input').fill(number);
    await computer.getByRole('button', { name: 'Pair the device' }).click();
    await computer.locator('sc-computers .pair-line').getByText('waits', { exact: false }).waitFor();
    await expect.poll(() => phone.evaluate(KEPT_TOKEN)).toBe(true);
    await phone.locator('home-page textarea').waitFor();
  });
});
