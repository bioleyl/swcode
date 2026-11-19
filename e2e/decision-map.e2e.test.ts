import { afterEach, describe, expect, it } from 'vitest';

import { DESKTOP, settle, start, stopRigs } from './support.js';

afterEach(stopRigs);

describe('the decision map', () => {
  it('shows the choice the model recorded, with its options and why', async () => {
    const rig = await start({
      replies: [
        {
          toolCalls: [
            {
              arguments: {
                chosen: 'React',
                options: ['React', 'Vue'],
                question: 'Which framework?',
                why: 'The project already uses it.',
              },
              name: 'decision',
            },
          ],
        },
        { text: 'The app is written.' },
        { text: 'Now on Vue.' },
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Make a todo app');
    await page.locator('home-page textarea').press('Enter');
    await page.getByText('The app is written.').waitFor();

    await page.locator('session-page .map-toggle').click();
    await settle(page);
    const map = page.locator('session-page sc-decision-map');
    await expect.poll(() => map.locator('.hint').textContent()).toContain('4 decisions');

    const node = map.locator('jl-flow .node[data-node^="decision:"]');
    expect(await node.locator('.label').textContent()).toBe('Which framework?');
    expect(await node.locator('.description').textContent()).toBe('Chosen · React');
    await node.click();
    await settle(page);
    expect(await map.locator('jl-card').evaluate((card) => card.heading.get())).toBe('Decision');
    const text = await map.locator('jl-card pre.text').textContent();
    expect(text).toContain('Chosen: React');
    expect(text).toContain('Options: React, Vue');
    expect(text).toContain('Why: The project already uses it.');

    const before = page.url();
    await map.locator('.change').click();
    await settle(page);
    const dialog = map.locator('jl-dialog');
    await dialog.locator('jl-select').evaluate((select) => select.value.set('Vue'));
    await dialog.locator('.save').click();
    await page.getByText('Now on Vue.').waitFor();
    expect(page.url()).not.toBe(before);
  });

  it('changes an answer the person gave, and replays from it', async () => {
    const rig = await start({
      replies: [
        {
          toolCalls: [
            {
              arguments: {
                questions: [
                  {
                    header: 'Database',
                    options: [
                      { description: 'Choose Postgres.', label: 'Postgres' },
                      { description: 'Choose SQLite.', label: 'SQLite' },
                    ],
                    question: 'Which database?',
                  },
                ],
              },
              name: 'question',
            },
          ],
        },
        { text: 'Postgres it is.' },
        { text: 'Now on SQLite.' },
      ],
    });
    const page = await rig.browser.newPage({ viewport: DESKTOP });
    await page.goto(rig.link);
    await page.locator('home-page textarea').fill('Make an app');
    await page.locator('home-page textarea').press('Enter');
    const card = page.locator('sc-question');
    await card.getByText('Which database?').waitFor();
    await card.getByText('Postgres', { exact: true }).click();
    await page.getByText('Postgres it is.').waitFor();

    await page.locator('session-page .map-toggle').click();
    await settle(page);
    const map = page.locator('session-page sc-decision-map');
    await expect.poll(() => map.locator('.hint').textContent()).toContain('4 decisions');
    await map.locator('jl-flow .node[data-node^="question:"]').click();
    await settle(page);
    await map.locator('.change').click();
    await settle(page);
    const dialog = map.locator('jl-dialog');
    await dialog.locator('textarea').fill('SQLite');
    await dialog.locator('.save').click();
    await page.getByText('Now on SQLite.').waitFor();
  });
});
