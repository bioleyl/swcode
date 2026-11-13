import { existsSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { DESKTOP, GREET, navItem, overflow, PHONE, projectTab, settle, start, stopRigs, tab } from './support.js';

import type { Page } from 'playwright-core';
import type { E2eRig } from './types/e2e.types.js';

afterEach(stopRigs);

describe('projects', () => {
  it('come from a folder picked in the dialog, and a session starts in one', async () => {
    const rig = await start({
      files: { 'src/greet.ts': GREET },
      replies: [{ toolCalls: [{ arguments: { command: 'pwd' }, name: 'bash' }] }, { text: 'Listed.' }],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page .new-project').click();
    await settle(page);
    await page.locator('sc-folder-browser li button', { hasText: 'src' }).click();
    await settle(page);
    await page.locator('sc-project-dialog .confirm button', { hasText: 'Use src' }).click();
    await settle(page);
    // A folder that was already there: the person says whether they trust what it says of itself.
    await page.locator('jl-confirm').getByText('Trust src?').waitFor();
    await page.locator('jl-confirm').getByRole('button', { name: 'Trust this folder' }).click();
    await settle(page);
    await page.locator('home-page select').filter({ hasText: 'src' }).waitFor();

    await page.locator('home-page textarea').fill('Where am I?');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Listed.').waitFor();
    await navItem(page, 'src').click();
    await settle(page);
    await page.locator('project-page sc-session-row').getByText('Where am I?').first().waitFor();
    expect(await page.getByText(join(rig.workspace, 'src')).count()).toBeGreaterThan(0);

    await page.locator('project-page .remove').click();
    await settle(page);
    await page.getByText('Its session goes with it, their conversations for good.', { exact: false }).waitFor();
    await page.locator('jl-confirm').getByRole('button', { name: 'Remove' }).click();
    await settle(page);
    await page.locator('home-page textarea').waitFor();
    await navItem(page, 'src').waitFor({ state: 'detached' });
    expect(readFileSync(join(rig.workspace, 'src/greet.ts'), 'utf8')).toBe(GREET);
  });
});

describe('a folder not trusted yet', () => {
  it('keeps its notes from the agent until its page says the person trusts it', async () => {
    const rig = await start({
      files: { 'src/AGENTS.md': 'Run the tests with npm test.\n', 'src/greet.ts': GREET },
      replies: [{ text: 'First.' }, { text: 'Second.' }],
    });
    const notesRead = (): number => rig.events().filter((event) => event.kind === 'projectNotesRead').length;
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page .new-project').click();
    await settle(page);
    await page.locator('sc-folder-browser li button', { hasText: 'src' }).click();
    await settle(page);
    await page.locator('sc-project-dialog .confirm button', { hasText: 'Use src' }).click();
    await settle(page);
    await page.locator('jl-confirm').getByRole('button', { name: 'Not now' }).click();
    await settle(page);
    await page.locator('home-page textarea').fill('One');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('First.').waitFor();
    expect(notesRead()).toBe(0);

    // The project's page says so on every tab, and trusts the folder with a press.
    await navItem(page, 'src').click();
    await settle(page);
    const banner = page.locator('project-page .untrusted');
    await banner.getByText('You have not trusted this folder').waitFor();
    await banner.locator('.trust').click();
    await page.evaluate('0');
    await banner.waitFor({ state: 'hidden' });

    await page.locator('project-page .new-session').click();
    await settle(page);
    await page.locator('home-page textarea').fill('Two');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Second.').waitFor();
    expect(notesRead()).toBe(1);
    expect(await overflow(page)).toBe(0);
  });
});

describe('a project’s check after each edit', () => {
  it('is kept with the project, and tells the agent what it found in the file it just edited', async () => {
    const rig = await start({
      files: { 'src/greet.ts': GREET },
      replies: [
        { text: 'Hello there.' },
        { toolCalls: [{ arguments: { path: 'src/greet.ts' }, name: 'read' }] },
        {
          toolCalls: [{ arguments: { newString: 'Hi', oldString: 'Hello', path: 'src/greet.ts' }, name: 'edit' }],
        },
        { text: 'Done.' },
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();
    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await projectTab(page, 'Config').click();
    await settle(page);

    // The command is typed once, and stays through the page's own redraws and a reload.
    const field = page.locator('project-page .after-edit-input input');
    await field.fill('grep -c Hello {file}');
    await page.locator('project-page .after-edit').getByRole('button', { name: 'Save' }).click();
    await page.getByText('The check after each edit is saved').waitFor();
    await page.reload();
    await expect.poll(() => field.inputValue()).toBe('grep -c Hello {file}');

    // The next edit runs it: grep finds no Hello left and fails, which the agent reads with its edit.
    await projectTab(page, 'Sessions').click();
    await settle(page);
    await page.locator('project-page sc-session-row').getByText('Hi', { exact: true }).first().click();
    await settle(page);
    await page.locator('session-page sc-composer textarea').fill('Make greet say Hi');
    await page.locator('session-page sc-composer textarea').press('Enter');
    await page.getByText('Done.').waitFor();
    const edited = rig.events().findLast((event) => event.kind === 'toolFinished');
    expect(edited?.kind === 'toolFinished' ? edited.output : '').toContain(
      'check after an edit (grep -c Hello {file}) failed'
    );
    expect(await overflow(page)).toBe(0);
  });
});

describe('a project’s rules', () => {
  it('take a program allowed “in this project” from the card, list it in its Config, and give it back', async () => {
    const rig = await start({
      replies: [{ toolCalls: [{ arguments: { command: 'echo one' }, name: 'bash' }] }, { text: 'Said one.' }],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    const composer = page.locator('home-page sc-composer');
    await composer.locator('.choices').click();
    await settle(page);
    await composer.locator('sc-session-choices').getByText('Ask before commands', { exact: true }).click();
    await settle(page);
    await page.keyboard.press('Escape');
    await page.locator('home-page textarea').fill('Say one');
    await page.locator('home-page textarea').press('Enter');

    const card = page.locator('sc-permission');
    await card.getByText('Run this command?').waitFor();
    await card.getByText('Always allow echo in this project').click();
    await page.evaluate('0');
    await page.getByText('Said one.').waitFor();

    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await projectTab(page, 'Config').click();
    await settle(page);
    const rules = page.locator('sc-project-allowed');
    await rules.locator('.rule .heading').getByText('echo', { exact: true }).waitFor();
    await rules.locator('.rule .remove').click();
    await page.evaluate('0');
    await rules.getByText('Nothing yet.').waitFor();
    expect(await rules.locator('.rule').count()).toBe(0);
    expect(await overflow(page)).toBe(0);
  });
});

describe('a project’s sandbox', () => {
  it('holds its sessions’ commands unless turned off in its Config, and says why where none can be made', async () => {
    const outside = join(tmpdir(), `swcode-e2e-outside-${process.pid}`);
    const rig = await start({
      replies: [
        { toolCalls: [{ arguments: { command: `echo inside > made.txt; echo no > ${outside}` }, name: 'bash' }] },
        { text: 'Tried.' },
      ],
      sandbox: true,
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Write the files');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Tried.').waitFor();
    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await projectTab(page, 'Config').click();
    await settle(page);

    const card = page.locator('sc-project-sandbox');
    const toggle = card.locator('.sandboxed').getByRole('switch');
    const problem = card.locator('jl-alert');
    await expect.poll(async () => (await toggle.isEnabled()) || (await problem.isVisible())).toBe(true);
    expect(readFileSync(join(rig.workspace, 'made.txt'), 'utf8')).toBe('inside\n');
    if (await problem.isVisible()) {
      // This machine makes no sandbox (no bubblewrap, or a system that refuses it): the card says so.
      expect(await toggle.isChecked()).toBe(false);
      expect(await problem.textContent()).toContain('bubblewrap');
      rmSync(outside, { force: true });
      return;
    }

    // The command wrote its workspace and nothing beside it: its /tmp is the session's own.
    expect(existsSync(outside)).toBe(false);
    expect(await toggle.isChecked()).toBe(true);
    await toggle.click();
    await page.evaluate('0');
    await expect.poll(() => toggle.isChecked()).toBe(false);
    await page.reload();
    await expect.poll(() => card.locator('.sandboxed').getByRole('switch').isChecked()).toBe(false);
    expect(await overflow(page)).toBe(0);
  });
});

describe('a project the daemon refuses', () => {
  it('says why in the dialog, which can be tried again', async () => {
    const rig = await start({ files: { 'src/greet.ts': GREET }, replies: [] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page .new-project').click();
    await settle(page);
    const dialog = page.locator('sc-project-dialog');
    await dialog.getByText('New folder', { exact: true }).click();
    await settle(page);
    // A folder that is already there cannot be made again.
    await dialog.locator('.name input').fill('src');
    await dialog.locator('.confirm button').click();
    await dialog.locator('jl-alert').getByText('already exists', { exact: false }).waitFor();
    expect(await dialog.locator('.confirm button').isDisabled()).toBe(false);

    await dialog.locator('.name input').fill('docs');
    await dialog.locator('.confirm button').click();
    await settle(page);
    await page.locator('home-page select').filter({ hasText: 'docs' }).waitFor();
  });
});

describe('a project’s page', () => {
  async function openProject(rig: E2eRig): Promise<Page> {
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();
    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await page.locator('project-page sc-session-row').getByText('Hi', { exact: true }).first().waitFor();
    return page;
  }

  it('deletes a session from its row, once confirmed', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }] });
    const page = await openProject(rig);

    await page.locator('project-page sc-session-row').getByRole('button', { name: 'Delete this session' }).click();
    await settle(page);
    await page.locator('jl-confirm').getByRole('button', { name: 'Delete' }).click();
    await settle(page);
    await page.locator('project-page jl-empty').getByText('No sessions yet').waitFor();
    expect(await page.locator('project-page sc-session-row').count()).toBe(0);
    expect(new URL(page.url()).hash).toContain('/projects/');
  });

  it('keeps what its new sessions start with, until it goes back to the app’s defaults', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }, { text: 'Going.' }] });
    const page = await openProject(rig);
    await projectTab(page, 'Config').click();
    await settle(page);
    const card = page.locator('project-page .defaults');
    const reset = page.locator('project-page .reset');
    await card.getByText('until you pick otherwise here', { exact: false }).waitFor();
    expect(await reset.isHidden()).toBe(true);

    await card.locator('sc-session-choices .thinking').getByText('Low', { exact: true }).click();
    await settle(page);
    await card.locator('sc-session-choices jl-switch').getByRole('switch').click();
    await settle(page);
    await card.getByText('Set for this project', { exact: false }).waitFor();
    expect(await reset.isVisible()).toBe(true);

    await page.locator('project-page .new-session').click();
    await settle(page);
    const summary = page.locator('home-page sc-composer .summary');
    await expect.poll(() => summary.textContent()).toContain('Think briefly');
    expect(await summary.textContent()).not.toContain('Token economy');
    await page.locator('home-page textarea').fill('Go');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Going.').waitFor();
    expect(await page.locator('session-page sc-composer .summary').textContent()).toContain('Think briefly');
    // Token economy, by message: the first session's by the app's defaults, the next by the project's.
    const concise = rig
      .events()
      .flatMap((event) => (event.kind === 'userMessage' ? [`${event.text}: ${event.concise === true}`] : []));
    expect(concise.sort()).toEqual(['Go: false', 'Hi: true']);

    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await projectTab(page, 'Config').click();
    await settle(page);
    await reset.click();
    await settle(page);
    await card.getByText('until you pick otherwise here', { exact: false }).waitFor();
    expect(await reset.isHidden()).toBe(true);
  });

  it('keeps its todos, which a new session pins to its first message', async () => {
    const rig = await start({ replies: [{ text: 'Hello there.' }, { text: 'On it.' }] });
    const page = await openProject(rig);
    await projectTab(page, 'Todos').click();
    await settle(page);
    const card = page.locator('project-page .todos');
    await card.locator('.todo-input input').fill('Write the tests');
    await card.getByRole('button', { name: 'Add' }).click();
    await settle(page);
    const row = card.locator('sc-todo-row').first();
    await row.getByText('Write the tests').waitFor();
    expect(await card.getByText('No todos yet').isHidden()).toBe(true);

    await row.getByRole('button', { name: 'Edit “Write the tests”' }).click();
    await row.locator('.editor input').fill('Write better tests');
    await row.getByRole('button', { name: 'Save' }).click();
    await row.getByText('Write better tests').waitFor();

    await row.getByRole('checkbox').click();
    await page.evaluate('0');
    await expect.poll(() => row.getByRole('checkbox').isChecked()).toBe(true);
    await page.reload();
    await row.waitFor();
    expect(await row.getByRole('checkbox').isChecked()).toBe(true);
    await row.getByRole('checkbox').click();
    await settle(page);

    // A todo being edited keeps what was typed while the page hears of another change: a todo added elsewhere.
    await row.getByRole('button', { name: 'Edit “Write better tests”' }).click();
    await row.locator('.editor input').fill('Write the best tests');
    const other = await rig.browser.newPage({ viewport: DESKTOP });
    // A new page is a new browser profile: the link brings its token first.
    await other.goto(rig.link);
    await other.goto(page.url());
    await other.locator('project-page .todos .todo-input input').fill('Ship it');
    await other.locator('project-page .todos').getByRole('button', { name: 'Add' }).click();
    await card.locator('sc-todo-row').nth(1).getByText('Ship it').waitFor();
    expect(await row.locator('.editor input').inputValue()).toBe('Write the best tests');
    await row.getByRole('button', { name: 'Cancel' }).click();
    await other.close();
    await card.getByRole('button', { name: 'Delete “Ship it”' }).click();
    await page.locator('jl-confirm').getByRole('button', { name: 'Delete' }).click();
    await card.getByText('Ship it').waitFor({ state: 'detached' });

    await page.locator('project-page .new-session').click();
    await settle(page);
    const todos = page.locator('home-page .todos');
    await todos.getByText('Write better tests').waitFor();
    await todos.getByRole('checkbox').click();
    await page.evaluate('0');
    await page.locator('home-page textarea').fill('Fix the bug');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('On it.').waitFor();
    await expect.poll(() => page.locator('session-page header sc-title button').textContent()).toBe('Fix the bug');
    const linked = page.locator('session-page sc-session-todos');
    await linked.getByText('Linked todos').waitFor();
    await linked.getByText('Write better tests').waitFor();
    const request = JSON.stringify(rig.llama.requests.at(-1));
    expect(request).toContain('Todos pinned to this session');
    expect(request).toContain('Write better tests');

    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await projectTab(page, 'Todos').click();
    await settle(page);
    const link = page.locator('project-page sc-todo-row jl-link');
    await link.waitFor();
    expect(await link.locator('a').getAttribute('href')).toContain('/sessions/');
  });
});

describe('a project’s skills', () => {
  const ReleaseNotes = {
    '.agents/skills/release-notes/SKILL.md':
      '---\nname: release-notes\ndescription: Writes release notes from the commits.\n---\nList the commits.',
  };
  const Deploy = {
    '.agents/skills/deploy/SKILL.md': '---\nname: deploy\ndescription: Deploys the app.\n---\nDeploy.',
  };

  it('show on its page, each turned off with its switch for its sessions and their / commands', async () => {
    const rig = await start({ files: ReleaseNotes, homeFiles: Deploy, replies: [{ text: 'Hello there.' }] });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();
    await navItem(page, basename(rig.workspace)).click();
    await settle(page);
    await projectTab(page, 'Skills').click();
    await settle(page);
    const card = page.locator('project-page .skills');
    const deploy = card.locator('jl-switch[data-skill="deploy"]');
    await deploy.waitFor();
    await card.locator('jl-switch[data-skill="release-notes"]').getByText('Project ·', { exact: false }).waitFor();
    await deploy.getByText('Yours ·', { exact: false }).waitFor();
    expect(await overflow(page)).toBe(0);

    await deploy.getByRole('switch').click();
    await page.evaluate('0');
    await expect.poll(() => deploy.getByRole('switch').isChecked()).toBe(false);
    await page.reload();
    await deploy.waitFor();
    expect(await deploy.getByRole('switch').isChecked()).toBe(false);

    await page.locator('project-page .new-session').click();
    await settle(page);
    const composer = page.locator('home-page sc-composer');
    await composer.locator('textarea').fill('/');
    await composer.locator('.command-menu [role="option"]').first().waitFor();
    expect(await composer.locator('.command-menu [role="option"] .name').allTextContents()).toEqual([
      '/release-notes',
    ]);
  });

  it('run from the composer’s / menu, the session showing the skill it ran', async () => {
    const rig = await start({
      homeFiles: ReleaseNotes,
      replies: [{ text: 'Hello there.' }, { text: 'Here are the notes.' }],
    });
    const page = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Hi');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Hello there.').waitFor();

    const composer = page.locator('session-page sc-composer');
    const field = composer.locator('textarea');
    await field.pressSequentially('/rel');
    await composer.locator('.command-menu [role="option"]', { hasText: '/release-notes' }).waitFor();
    await field.press('Enter');
    await page.evaluate('0');
    await page.getByText('Here are the notes.').waitFor();
    await page.locator('session-page sc-conversation').getByText('Ran the skill release-notes').waitFor();
    const loaded = rig.events().flatMap((event) => (event.kind === 'skillLoaded' ? [event.name] : []));
    expect(loaded).toEqual(['release-notes']);
    expect(await overflow(page)).toBe(0);
  });
});

describe('skills', () => {
  it('are read from the home’s .agents folder, and the session shows what the agent was told of them', async () => {
    const rig = await start({
      homeFiles: {
        '.agents/skills/release-notes/SKILL.md':
          '---\nname: release-notes\ndescription: Writes release notes from the commits.\n---\nList the commits.',
      },
      replies: [{ text: 'Here are the notes.' }],
    });
    const page = await rig.browser.newPage({ deviceScaleFactor: 2, viewport: PHONE });
    await page.goto(rig.link);
    await tab(page, 'New').click();
    await settle(page);
    await page.locator('home-page textarea').fill('Write the release notes');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('Here are the notes.').waitFor();
    await page.getByText('Skills the agent was told of: release-notes').waitFor();
    expect(await overflow(page)).toBe(0);
  });
});
