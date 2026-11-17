// Edge cases of the list page (static/js/pages/list.js): the selection is
// always something on screen, keys go where they should whatever was clicked
// last, and what is shown agrees with the server when changes come quickly,
// fail, or cross a refresh.
import { test, expect, openList, row, rowNames, selectedName, saved, toast } from './helpers.js';

const TRIP = [
  ['Car', null],
  ['Blue Backpack', 'Car'],
  ['Hygiene Kit', 'Blue Backpack'],
  ['Comb', 'Hygiene Kit'],
  ['Toothbrush', 'Hygiene Kit'],
  ['Socks', 'Blue Backpack', { quantity: 3, unit: 'pairs' }],
  ['Camp Chair', 'Car', { quantity: 2 }],
  ['Sun Hat', null],
];
const draft = (page) => page.locator('.pack .row.new');

test('Space and Enter belong to the list, not to the title bar button that was clicked last', async ({ page, pack }) => {
  const list = await pack.list('Keys', TRIP);
  await openList(page, list);
  await page.locator('#btn-collapse-all').click();
  await page.keyboard.press('ArrowDown'); // Car
  await page.keyboard.press(' '); // opens the car; it does not press "Collapse all" again
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Camp Chair', 'Sun Hat']);
  await page.keyboard.press(' ');
  expect(await rowNames(page)).toEqual(['Car', 'Sun Hat']);
  await page.keyboard.press(' ');
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Camp Chair', 'Sun Hat']);
  // Enter edits the quantity although a title bar button was the last thing clicked.
  await page.locator('#btn-expand-all').click();
  await page.keyboard.press('Enter');
  await expect(row(page, 'Car').locator('.qty-input')).toBeFocused();
  await page.keyboard.press('Escape');
  // The same after a row's own button (a caret) was clicked.
  await row(page, 'Hygiene Kit').locator('.caret').click();
  expect(await selectedName(page)).toBe('Hygiene Kit');
  await page.keyboard.press(' ');
  await expect(row(page, 'Hygiene Kit')).toHaveAttribute('aria-expanded', 'true');
});

test('the selection is always on screen: a status key never changes a hidden entry', async ({ page, pack }) => {
  const list = await pack.list('Hidden', TRIP);
  await openList(page, list);
  // Selected inside a container, found through the filter, then the filter is cleared with everything closed.
  await page.locator('#btn-collapse-all').click();
  await page.locator('.filter-box').fill('tooth');
  await row(page, 'Toothbrush').locator('.item-name').click();
  await page.locator('.filter-box').fill('');
  await page.locator('.filter-box').blur();
  expect(await rowNames(page)).toEqual(['Car', 'Sun Hat']);
  expect(await selectedName(page)).toBe('Car'); // handed up to what is in view
  const done = saved(page);
  await page.keyboard.press('p');
  await done;
  const now = await pack.fetch(list.id);
  expect(now.entry('Toothbrush').status).toBe('In Storage Location');
  expect(now.entry('Car').status).toBe('Packed');

  // Closing the container the selection is in hands the selection to that container.
  await page.locator('#btn-expand-all').click();
  await row(page, 'Comb').locator('.item-name').click();
  await row(page, 'Blue Backpack').locator('.caret').click();
  expect(await selectedName(page)).toBe('Blue Backpack');
});

test('Escape in the filter box keeps the filter and hands the keys back', async ({ page, pack }) => {
  const list = await pack.list('Filter', TRIP);
  await openList(page, list);
  await page.locator('.filter-box').click();
  await page.keyboard.type('sock');
  await page.keyboard.press('Escape');
  await expect(page.locator('.filter-box')).toHaveValue('sock');
  await expect(page.locator('.filter-box')).not.toBeFocused();
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Socks']);
  await page.keyboard.press('End');
  expect(await selectedName(page)).toBe('Socks');
});

test('working down a status filter: the selection moves on when an entry leaves it', async ({ page, pack }) => {
  const list = await pack.list('Work', TRIP);
  await openList(page, list);
  await page.locator('.multi-btn', { hasText: 'Status' }).click();
  await page.locator('.multi-opt', { hasText: 'In Storage Location' }).click();
  await page.keyboard.press('Escape');
  await page.locator('.multi-btn', { hasText: 'Status' }).blur();
  await row(page, 'Comb').locator('.item-name').click();
  for (const next of ['Toothbrush', 'Socks', 'Camp Chair']) {
    const done = saved(page);
    await page.keyboard.press('p');
    await done;
    expect(await selectedName(page)).toBe(next);
  }
  const now = await pack.fetch(list.id);
  expect(['Comb', 'Toothbrush', 'Socks'].map((n) => now.entry(n).status)).toEqual(['Packed', 'Packed', 'Packed']);
  expect(now.entry('Camp Chair').status).toBe('In Storage Location');
});

test('two status keys in quick succession: the last one wins, on screen and on the server', async ({ page, pack }) => {
  const list = await pack.list('Race', TRIP);
  await openList(page, list);
  const patches = [];
  // Hold the first answer back so the second key is pressed while it is on its way.
  await page.route('**/api/contents/*', async (route) => {
    if (route.request().method() !== 'PATCH') return route.continue();
    patches.push(route.request().postDataJSON());
    if (patches.length === 1) await new Promise((r) => setTimeout(r, 400));
    return route.continue();
  });
  await row(page, 'Sun Hat').locator('.item-name').click();
  await page.keyboard.press('p');
  await page.keyboard.press('m');
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveText('Missing'); // at once
  await expect.poll(async () => (await pack.fetch(list.id)).entry('Sun Hat').status).toBe('Missing');
  expect(patches.map((b) => b.status_id)).toEqual([pack.statusId('Packed'), pack.statusId('Missing')]); // sent in order
  await page.locator('#btn-collapse-all').click(); // any redraw
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveText('Missing');
  await expect(page.locator('.legend-row .status-chip', { hasText: 'Packed' }).locator('.count')).toHaveText('0');

  // A change the server refuses is taken back, with the reason.
  await page.unroute('**/api/contents/*');
  await page.route('**/api/contents/*', (route) => (route.request().method() === 'PATCH'
    ? route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal error"}' }) : route.continue()));
  await page.keyboard.press('p');
  await expect(toast(page)).toContainText('Internal error');
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveText('Missing');
});

test('a shortcut that needs Shift to type does not move the selection on', async ({ page, pack }) => {
  const status = await pack.createRef('statuses', { name: `Quoted ${Math.random().toString(36).slice(2, 7)}`, color: '#59a14f', shortcut: '"' });
  const list = await pack.list('Shift', TRIP);
  await openList(page, list);
  await row(page, 'Comb').locator('.item-name').click();
  const done = saved(page);
  await page.keyboard.press('Shift+"');
  await done;
  expect(await selectedName(page)).toBe('Comb');
  expect((await pack.fetch(list.id)).entry('Comb').status).toBe(status.name);
});

test('what was typed into the add-entry row survives a redraw; a name typed in full counts as picked', async ({ page, pack }) => {
  const list = await pack.list('Draft', TRIP);
  await openList(page, list);
  await page.locator('#btn-add-item').click();
  await page.keyboard.type('gloves'); // in full, no pick from the list
  await draft(page).locator('.qty-input').fill('3');
  await draft(page).locator('.notes-input').fill('the warm ones');
  await row(page, 'Hygiene Kit').locator('.caret').click(); // redraws the list
  await expect(draft(page).locator('.combo-input')).toHaveValue('Gloves');
  await expect(draft(page).locator('.qty-input')).toHaveValue('3');
  await expect(draft(page).locator('.notes-input')).toHaveValue('the warm ones');
  const done = saved(page, 'POST');
  await draft(page).locator('button.save').click();
  await done;
  expect((await pack.fetch(list.id)).entry('Gloves')).toMatchObject({ container: null, quantity: 3, notes: 'the warm ones' });
  // The row is emptied for the next entry.
  await expect(draft(page).locator('.combo-input')).toHaveValue('');
  await expect(draft(page).locator('.qty-input')).toHaveValue('1');
  await expect(draft(page).locator('.notes-input')).toHaveValue('');
});

test('an edit made after the tab regains the focus stays as typed', async ({ page, pack }) => {
  const list = await pack.list('Focus', TRIP);
  await openList(page, list);
  // Coming back to the tab re-reads the list.
  const reread = page.waitForResponse((r) => r.url().endsWith(`/api/lists/${list.id}`) && r.request().method() === 'GET');
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await reread;
  await page.waitForTimeout(100);
  const socks = row(page, 'Socks');
  let done = saved(page);
  await socks.locator('.qty-input').fill('7');
  await socks.locator('.qty-input').press('Enter');
  await done;
  await expect(socks.locator('.qty-input')).toHaveValue('7');
  done = saved(page);
  await socks.locator('.notes-input').fill('hello');
  await socks.locator('.notes-input').press('Enter');
  await done;
  await expect(socks.locator('.notes-input')).toHaveValue('hello');
  expect((await pack.fetch(list.id)).entry('Socks')).toMatchObject({ quantity: 7, notes: 'hello' });

  // A change made elsewhere shows after the next return to the tab, statuses included.
  await page.request.patch(`/api/contents/${list.rows.find((r) => r.item === 'Sun Hat').id}`, { data: { status_id: pack.statusId('Missing') } });
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveText('Missing');
  // An edit still being saved shows as typed if the list is redrawn meanwhile.
  await page.route('**/api/contents/*', async (route) => { await new Promise((r) => setTimeout(r, 300)); return route.continue(); });
  done = saved(page);
  await socks.locator('.notes-input').fill('in the safe');
  await socks.locator('.notes-input').press('Enter');
  await row(page, 'Hygiene Kit').locator('.caret').click();
  await expect(row(page, 'Socks').locator('.notes-input')).toHaveValue('in the safe');
  await done;
});

test('leaving a page closes its dialogs, and a page still loading does not leak into the next', async ({ page, pack }) => {
  const list = await pack.list('Leave', TRIP);
  await openList(page, list);
  await row(page, 'Car').hover();
  await row(page, 'Car').locator('button.delete').click();
  await expect(page.locator('.modal')).toBeVisible();
  await expect(page.locator('.modal-foot button').first()).toBeFocused(); // Cancel, not the button behind the dialog
  await page.keyboard.press('Enter');
  await expect(page.locator('.modal')).toHaveCount(0); // Enter pressed Cancel; it did not open a second dialog
  await row(page, 'Car').locator('button.delete').click();
  await page.goBack();
  await expect(page.locator('.modal')).toHaveCount(0);
  expect((await pack.fetch(list.id)).entries).toHaveLength(TRIP.length);

  // Open the list slowly and go to the Library before it arrives.
  await page.goto('/#/lists');
  await expect(page.locator('.lists-table')).toBeVisible();
  await page.route(`**/api/lists/${list.id}`, async (route) => { await new Promise((r) => setTimeout(r, 500)); return route.continue(); });
  await page.evaluate((id) => { location.hash = `#/list?id=${id}`; }, list.id);
  await page.locator('.nav a', { hasText: 'Library' }).click();
  await expect(page.locator('.ref-table')).toBeVisible();
  await page.waitForTimeout(800);
  await expect(page.locator('.list-switch')).toHaveCount(0);
  await expect(page.locator('.legend-row')).toHaveCount(0);
  await expect(page.locator('.pack')).toHaveCount(0);
  await expect(page).toHaveTitle(/Library/);
});
