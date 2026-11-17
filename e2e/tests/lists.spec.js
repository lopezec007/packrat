// The Lists page (static/js/pages/lists.js) and the New list dialog
// (static/js/newlist.js): making lists from scratch and from other lists.
import { test, expect, uid, row, rowNames, toast, saved } from './helpers.js';

const listRow = (page, name) => page.locator(`.lists-table .row[data-name="${name}"]`);
const source = (page, name) => page.locator('.source-opt', { hasText: name }).locator('input');

test('the page shows the demo lists, ordinary ones apart from the templates', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/#\/lists$/);
  await expect(page.locator('.nav a[aria-current="page"]')).toHaveText('Lists');
  await expect(page.locator('.lists-table .group-row')).toHaveText([/^Lists/, /^Templates/]);
  const names = await page.locator('.lists-table .row[data-name]').evaluateAll((els) => els.map((e) => e.dataset.name));
  expect(names.indexOf('Paris 2027')).toBeLessThan(names.indexOf('Camping Weekend'));
  await expect(listRow(page, 'Camping Weekend').locator('.num')).toHaveText('25');
  await expect(listRow(page, 'Camping Weekend').locator('input[type="checkbox"]')).toBeChecked();
  await expect(listRow(page, 'Paris 2027').locator('input[type="checkbox"]')).not.toBeChecked();
  await expect(listRow(page, 'Paris 2027').locator('.status-bar')).toHaveAttribute('title', /9 In Storage Location · 7 Packed · 1 Missing/);
  // The filter narrows both groups.
  await page.locator('.filter-box').fill('camp');
  await expect(page.locator('.lists-table .row[data-name]')).toHaveCount(1);
  // A list opens from its name.
  await page.locator('.filter-box').fill('');
  await listRow(page, 'Park Day').locator('.list-link').click();
  await expect(page).toHaveURL(/#\/list\?id=\d+$/);
  await expect(page.locator('.list-switch option:checked')).toHaveText('Park Day');
  await expect(page.locator('.nav a[aria-current="page"]')).toHaveText('Lists');
});

test('a new list from scratch starts empty and opens', async ({ page, pack }) => {
  const name = `Scratch ${uid()}`;
  await page.goto('/#/lists');
  await page.locator('#btn-new-list').click();
  const dialog = page.locator('.modal.new-list');
  await expect(dialog.locator('.new-list-summary')).toContainText('starts empty');
  // A name is required.
  await dialog.locator('button.primary').click();
  await expect(dialog.locator('#new-list-name')).toHaveClass(/invalid/);
  await dialog.locator('#new-list-name').fill(name);
  await dialog.locator('#new-list-name').press('Enter');
  await expect(page).toHaveURL(/#\/list\?id=\d+$/);
  const made = await pack.trackListByName(name);
  expect(made).toMatchObject({ name, is_template: false, rows: 0 });
  await expect(page.locator('.list-empty')).toBeVisible();
  // A second list of the same name is refused, and the dialog stays open.
  await page.goto('/#/lists');
  await page.locator('#btn-new-list').click();
  await dialog.locator('#new-list-name').fill(name);
  await dialog.locator('button.primary').click();
  await expect(toast(page)).toContainText('already a list with that name');
  await expect(dialog).toBeVisible();
});

test('a new list from several lists is everything on any of them, with the count shown first', async ({ page, pack, request }) => {
  // A template and an ordinary list that share some items.
  const a = await pack.list('Tpl A', [
    ['Car', null], ['Blue Backpack', 'Car'], ['Socks', 'Blue Backpack', { quantity: 3, unit: 'pairs', status: 'Packed' }],
    ['Comb', 'Blue Backpack'], ['Camp Chair', 'Car'],
  ], { template: true });
  const b = await pack.list('Plain B', [
    ['Daypack', null], ['Socks', 'Daypack', { quantity: 5, unit: 'pairs', status: 'Missing' }], ['Comb', 'Daypack'], ['Sun Hat', null],
    ['Blue Backpack', null], ['Socks', 'Blue Backpack', { quantity: 4, unit: 'pairs', status: 'Missing' }],
  ]);
  await page.goto('/#/lists');
  await page.locator('#btn-new-list').click();
  const dialog = page.locator('.modal.new-list');
  await source(page, a.name).check();
  await expect(dialog.locator('.new-list-summary')).toContainText('A copy of');
  await expect(dialog.locator('.new-list-summary')).toContainText('5 entries');
  await source(page, b.name).check();
  // 5 + 6 placements, two of them shared (the backpack itself, and the socks in it): 9.
  await expect(dialog.locator('.new-list-summary')).toContainText('Everything that is on any of the 2 lists: 9 entries');
  // There is nothing to choose: lists are always combined this way.
  await expect(dialog.locator('input[type="radio"]')).toHaveCount(0);
  await expect(dialog).not.toContainText(/intersection/i);

  const name = `Either ${uid()}`;
  await dialog.locator('#new-list-name').fill(name);
  await dialog.locator('#new-list-template').check();
  await dialog.locator('button.primary').click();
  await expect(page).toHaveURL(/#\/list\?id=\d+$/);
  const made = await pack.trackListByName(name);
  expect(made.is_template).toBe(true);
  const got = await pack.fetch(made.id);
  expect(got.entries).toHaveLength(9);
  // The backpack stays where the list ticked first has it (in the car), and what the other list packs into it follows.
  expect(got.entry('Blue Backpack')).toMatchObject({ container: 'Car' });
  expect(got.entries.filter((e) => e.item === 'Blue Backpack')).toHaveLength(1);
  // The same placement on both lists is one entry: the larger quantity, and the first list's status (a template's is kept).
  expect(got.entry('Socks', 'Blue Backpack')).toMatchObject({ quantity: 4, status: 'Packed' });
  // Entries that come from the ordinary list start at the default status, not the one they had there.
  expect(got.entry('Socks', 'Daypack')).toMatchObject({ quantity: 5, status: 'In Storage Location' });
  await expect(page.locator('.template-chip')).toBeVisible();
  await expect(row(page, 'Daypack')).toBeVisible();
  expect(await rowNames(page)).toContain('Sun Hat');

  // The API has no way of asking for anything else.
  const refused = await request.post('/api/lists', { data: { name: `Mode ${uid()}`, sources: [a.id, b.id], mode: 'intersection' } });
  expect(refused.status()).toBe(400);
  expect((await request.post('/api/lists/preview', { data: { sources: [a.id, b.id] } }).then((r) => r.json()))).toEqual({ rows: 9 });
});

test('a list is renamed, flagged as a template, copied and deleted from its row', async ({ page, pack }) => {
  const list = await pack.list('Row', [['Car', null], ['Sun Hat', 'Car']]);
  await page.goto('/#/lists');
  const renamed = `Row renamed ${uid()}`;
  await listRow(page, list.name).locator('button.rename').click();
  await listRow(page, list.name).locator('.cell-input').fill(renamed);
  const named = saved(page);
  await listRow(page, list.name).locator('.cell-input').press('Enter');
  await named;
  await expect(listRow(page, renamed)).toBeVisible();
  expect((await pack.fetch(list.id)).name).toBe(renamed);
  // Escape leaves the name alone.
  await listRow(page, renamed).locator('button.rename').click();
  await listRow(page, renamed).locator('.cell-input').fill('something else');
  await listRow(page, renamed).locator('.cell-input').press('Escape');
  await expect(listRow(page, renamed).locator('.list-link')).toHaveText(renamed);

  // The Template box moves it to the other group.
  const flagged = saved(page);
  await listRow(page, renamed).locator('input[type="checkbox"]').check();
  await flagged;
  expect((await pack.fetch(list.id)).is_template).toBe(true);
  await expect.poll(async () => {
    const names = await page.locator('.lists-table .row').evaluateAll((els) => els.map((e) => e.dataset.name || e.textContent.trim()));
    return names.indexOf(renamed) > names.findIndex((n) => n.startsWith('Templates'));
  }).toBe(true);

  // Copy opens the New list dialog with this list ticked.
  await listRow(page, renamed).locator('button.copy').click();
  const dialog = page.locator('.modal.new-list');
  await expect(source(page, renamed)).toBeChecked();
  const copy = `Row copy ${uid()}`;
  await dialog.locator('#new-list-name').fill(copy);
  await dialog.locator('button.primary').click();
  await expect(page).toHaveURL(/#\/list\?id=\d+$/);
  const made = await pack.trackListByName(copy);
  expect((await pack.fetch(made.id)).entries.map((e) => [e.item, e.container])).toEqual([['Car', null], ['Sun Hat', 'Car']]);

  // Delete asks first.
  await page.goto('/#/lists');
  page.once('dialog', (d) => { expect(d.message()).toContain('2 entries'); d.accept(); });
  await listRow(page, copy).locator('button.delete').click();
  await expect(listRow(page, copy)).toHaveCount(0);
  expect((await page.request.get(`/api/lists/${made.id}`)).status()).toBe(404);
  // The items are still in the library.
  expect(pack.itemId('Sun Hat')).toBeGreaterThan(0);
  expect((await (await page.request.get('/api/ref/items')).json()).some((i) => i.name === 'Sun Hat')).toBe(true);
});
