import { writeFileSync } from 'node:fs';

import { afterEach, describe, expect, it } from 'vitest';

import { startFakeMcp } from './fake-mcp.js';
import { DESKTOP, navItem, settingsLink, settle, start, stopRigs } from './support.js';

import type { Page } from 'playwright-core';
import type { FakeMcp } from './types/fake-mcp.types.js';

const servers: FakeMcp[] = [];

afterEach(async () => {
  await stopRigs();
  await Promise.all(servers.splice(0).map((server) => server.stop()));
});

/** A servers file that lists the fake server as greeter. */
const greeterFile = async (): Promise<Readonly<Record<string, string>>> => {
  const server = await startFakeMcp();
  servers.push(server);
  return { 'mcp.json': JSON.stringify({ mcpServers: { greeter: { url: server.url } } }) };
};

const openServers = async (page: Page): Promise<void> => {
  await settingsLink(page).click();
  await settle(page);
  await page.locator('settings-page jl-tabs').getByRole('tab', { exact: true, name: 'MCP servers' }).click();
  await settle(page);
};

describe('MCP servers', () => {
  it('are listed in the settings with where each stands, the file read again on a press', async () => {
    const dataFiles = await greeterFile();
    const rig = await start({ replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await openServers(page);
    const panel = page.locator('sc-mcp-servers');
    await panel.getByText('No MCP server is listed yet.', { exact: false }).waitFor();
    expect(new URL(page.url()).hash).toBe('#/settings/mcp');

    // The file is written by hand: the daemon reads it again when asked, and reaches the server it now lists.
    const file = String(await panel.locator('.file').textContent());
    expect(file.endsWith('mcp.json')).toBe(true);
    writeFileSync(file, dataFiles['mcp.json'] ?? '');
    await panel.locator('.reload').click();
    await page.evaluate('0');
    await panel.locator('.server .heading').getByText('greeter', { exact: true }).waitFor();
    await panel.getByText('Ready', { exact: true }).waitFor();
    await panel.getByText('1 tool', { exact: false }).waitFor();
    expect(await panel.locator('.example').isHidden()).toBe(true);
  });

  it('give the agent their tools, each call asked about until the server is allowed for the session', async () => {
    const rig = await start({
      dataFiles: await greeterFile(),
      replies: [
        { toolCalls: [{ arguments: { name: 'Ada' }, name: 'mcp_greeter_greet' }] },
        { toolCalls: [{ arguments: { name: 'Grace' }, name: 'mcp_greeter_greet' }] },
        { text: 'Both are greeted.' },
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await openServers(page);
    await page.locator('sc-mcp-servers').getByText('Ready', { exact: true }).waitFor();

    await navItem(page, 'New session').click();
    await settle(page);
    const composer = page.locator('home-page sc-composer');
    await composer.locator('.choices').click();
    await settle(page);
    await composer.locator('sc-session-choices').getByText('Ask before edits', { exact: true }).click();
    await settle(page);
    await page.keyboard.press('Escape');
    await page.locator('home-page textarea').fill('Greet Ada and Grace');
    await page.locator('home-page textarea').press('Enter');

    const card = page.locator('sc-permission');
    await card.getByText('Call this tool of an MCP server?').waitFor();
    expect(await card.locator('.path').textContent()).toBe('greeter · greet {"name":"Ada"}');
    await card.getByText('Always allow greeter’s tools in this session').click();
    await page.evaluate('0');
    await page.getByText('Both are greeted.').waitFor();
    // The second call went through without a card: the server was allowed.
    expect(rig.events().filter((event) => event.kind === 'permissionRequested')).toHaveLength(1);
    expect(
      rig
        .events()
        .filter((event) => event.kind === 'toolFinished')
        .map((event) => (event.kind === 'toolFinished' ? event.output : ''))
    ).toEqual(['Hello, Ada.', 'Hello, Grace.']);
  });
});
