import { readdirSync, readFileSync } from 'node:fs';
import { basename, join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { DESKTOP, settle, start, stopRigs } from './support.js';

import type { Page } from 'playwright-core';
import type { E2eRig } from './types/e2e.types.js';

afterEach(stopRigs);

/** A project to run a workflow in: a first session in the daemon's folder makes one. */
async function projectSession(page: Page, rig: E2eRig): Promise<void> {
  await page.goto(rig.link);
  await page.locator('home-page textarea').fill('Hi');
  await page.locator('home-page textarea').press('Enter');
  await page.getByText('Ready.').waitFor();
}

async function editStep(page: Page, name: string, role: string, prompt: string): Promise<void> {
  await page.locator('workflows-page .step-name input').fill(name);
  await page.locator('workflows-page .step-role textarea').fill(role);
  await page.locator('workflows-page .step-prompt textarea').fill(prompt);
  await page.locator('workflows-page .step-apply').click();
  await settle(page);
}

describe('workflows', () => {
  it('are written in the editor, confirmed, and run as one session per step with handoff files', async () => {
    const rig = await start({
      replies: [{ text: 'Ready.' }, { text: 'Tickets: build it.' }, { text: 'Implemented.' }],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await projectSession(page, rig);

    await page.goto(`${rig.link}#/workflows/new`);
    await settle(page);
    await page.locator('workflows-page .name input').fill('Team');

    await page.locator('workflows-page .add-step').click();
    await settle(page);
    await editStep(page, 'PO', 'You write the tickets.', 'Write the tickets for what the person wants.');
    await page.locator('workflows-page .add-step').click();
    await settle(page);
    await editStep(page, 'Dev', 'You implement one ticket.', 'Implement the tickets you are handed.');

    await page.locator('workflows-page .save').click();
    await settle(page);
    await expect.poll(() => page.locator('workflows-page .confirm').isVisible()).toBe(true);
    await page.locator('workflows-page .confirm').click();
    await expect.poll(() => page.locator('workflows-page .draft-badge').isHidden()).toBe(true);

    // The session's composer offers the workflow: its first message is the brief.
    await page.goto(rig.link);
    await settle(page);
    await page.locator('home-page .where jl-select select').selectOption({ label: basename(rig.workspace) });
    await settle(page);
    const composer = page.locator('home-page sc-composer');
    await composer.locator('.choices').click();
    await settle(page);
    await composer.locator('sc-session-choices .workflow select').selectOption({ label: 'Team' });
    await page.keyboard.press('Escape');
    await composer.locator('textarea').fill('Build the team feature');
    await composer.locator('textarea').press('Enter');

    // The run session shows the graph and the steps, and the run ends with both done.
    const run = page.locator('session-page sc-workflow-run');
    await run.getByRole('heading', { name: 'Workflow · Team' }).waitFor();
    await expect
      .poll(async () => (await run.locator('.run-status').textContent()) ?? '', { timeout: 30_000 })
      .toContain('2 of 2 steps done');
    expect(await run.locator('.steps jl-list-item').count()).toBe(2);

    // Each step left its report in the run's folder, in the project.
    const folders = readdirSync(join(rig.workspace, '.swcode', 'workflows'));
    expect(folders).toHaveLength(1);
    const folder = join(rig.workspace, '.swcode', 'workflows', folders[0] ?? '');
    const files = readdirSync(folder).sort();
    expect(files).toHaveLength(2);
    const reports = files.map((file) => readFileSync(join(folder, file), 'utf8')).join('\n');
    expect(reports).toContain('Tickets: build it.');
    expect(reports).toContain('Implemented.');
    expect(await page.evaluate('document.documentElement.scrollWidth - window.innerWidth')).toBe(0);
  });
});

/** A reply that calls a tool, and one that answers, as the fake llm-server takes them. */
const toolCall = (name: string, args: Record<string, unknown>) => ({ toolCalls: [{ arguments: args, name }] });
const answer = (text: string) => ({ text });
/** A workflow step's reply: its handoff, then the answer that ends its turn. */
const handoff = (exit: string, report: string) => [toolCall('handoff', { exit, report }), answer('Handed off.')];

describe('a workflow that loops', () => {
  it('goes back to dev when QA asks for a fix, and finishes when the PO says so', async () => {
    const team = {
      description: 'A PO, a Dev and a QA working through tickets.',
      edges: [
        { from: 'po', to: 'dev' },
        { from: 'dev', to: 'qa' },
        { from: 'qa', port: 'fix', to: 'dev' },
        { from: 'qa', port: 'pass', to: 'check' },
        { from: 'check', port: 'more', to: 'dev' },
      ],
      name: 'Team loop',
      steps: [
        {
          id: 'po',
          name: 'PO',
          prompt: 'Write the tickets and hand them to Dev.',
          role: 'You write the tickets.',
        },
        { id: 'dev', name: 'Dev', prompt: 'Implement the ticket you are handed.', role: 'You implement tickets.' },
        {
          exits: [
            { id: 'fix', label: 'Fix needed', when: 'the tests fail or the review finds a problem' },
            { id: 'pass', label: 'Passed', when: 'it tests and reviews cleanly' },
          ],
          id: 'qa',
          name: 'QA',
          prompt: 'Test the work. Hand it back to Dev for a fix, or to Check when it passes.',
          role: 'You test.',
        },
        {
          exits: [{ id: 'more', label: 'Another ticket', when: 'another ticket is left' }],
          id: 'check',
          name: 'PO check',
          prompt: 'Validate the work. Hand another ticket to Dev, or finish.',
          role: 'You validate.',
        },
      ],
    };
    const rig = await start({
      replies: [
        toolCall('workflow', team),
        answer('I proposed it.'),
        ...handoff('', 'Ticket 1: build the thing.'),
        ...handoff('', 'Built the thing.'),
        ...handoff('fix', 'Fix the null check.'),
        ...handoff('', 'Fixed the null check.'),
        ...handoff('pass', 'The work passes.'),
        ...handoff('', 'Validated: all tickets are done.'),
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Make me a team of three');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('I proposed it.').waitFor();

    // The agent's proposal waits in Workflows as a draft; its person confirms it.
    await page.goto(`${rig.link}#/workflows`);
    await settle(page);
    await page.locator('workflows-page .workflows').getByRole('link', { name: 'Team loop' }).click();
    await settle(page);
    await expect.poll(() => page.locator('workflows-page .confirm').isVisible()).toBe(true);
    await page.locator('workflows-page .confirm').click();
    await expect.poll(() => page.locator('workflows-page .draft-badge').isHidden()).toBe(true);

    await page.goto(rig.link);
    await settle(page);
    await page.locator('home-page .where jl-select select').selectOption({ label: basename(rig.workspace) });
    await settle(page);
    const composer = page.locator('home-page sc-composer');
    await composer.locator('.choices').click();
    await settle(page);
    await composer.locator('sc-session-choices .workflow select').selectOption({ label: 'Team loop' });
    await page.keyboard.press('Escape');
    await composer.locator('textarea').fill('Build the feature in two tickets');
    await composer.locator('textarea').press('Enter');

    const run = page.locator('session-page sc-workflow-run');
    await run.getByRole('heading', { name: 'Workflow · Team loop' }).waitFor();
    await expect
      .poll(async () => (await run.locator('.run-status').textContent()) ?? '', { timeout: 30_000 })
      .toContain('6 of 6 steps done');
    expect(await run.locator('.steps jl-list-item').count()).toBe(6);

    const folders = readdirSync(join(rig.workspace, '.swcode', 'workflows'));
    const folder = join(rig.workspace, '.swcode', 'workflows', folders[0] ?? '');
    const reports = readdirSync(folder)
      .map((file) => readFileSync(join(folder, file), 'utf8'))
      .join('\n');
    expect(reports).toContain('Fix the null check');
    expect(reports).toContain('Validated: all tickets are done.');
  });
});
