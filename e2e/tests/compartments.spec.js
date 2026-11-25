// Compartments (static/js/compartments.js, list/boxes.js): a container's
// compartments are made in the Library, and on a list what is inside the
// container can be put in one of them.
import { test, expect, uid, openList, row, box, compartment, rowNames, saved, toast } from './helpers.js';

const refRow = (page, name) => page.locator(`.ref-table .row[data-name="${name}"]`);
const draft = (page) => page.locator('.pack .row.new');
const option = (page, name) => page.locator('.combo-opt', { hasText: name }).first();
const refSaved = (page, method = 'POST') => page.waitForResponse((r) => r.url().includes('/api/ref/compartments') && r.request().method() === method);
const inside = (inner, outer) => inner.x >= outer.x - 0.5 && inner.y >= outer.y - 0.5
  && inner.x + inner.width <= outer.x + outer.width + 0.5 && inner.y + inner.height <= outer.y + outer.height + 0.5;

// The demo Daypack has the compartments Main, Front Pocket and Side Pocket.
const DAY = [
  ['Daypack', null],
  ['Camera', 'Daypack', { compartment: 'Main' }],
  ['Passport', 'Daypack', { compartment: 'Front Pocket' }],
  ['Socks', 'Daypack'],
];

test('a container is given compartments in the Library', async ({ page, pack }) => {
  const name = `Duffel ${uid()}`;
  const item = await pack.createRef('items', { name, item_category_id: pack.ref.item_categories[0].id, storage_location_id: pack.ref.storage_locations[0].id, is_container: true });
  await page.goto('/#/library');
  await page.locator('.filter-box').fill(name);
  // Only a container has them.
  await expect(refRow(page, 'Comb').locator('[data-col="compartments"]')).toHaveCount(1);
  await page.locator('.filter-box').fill('');
  await expect(refRow(page, 'Comb').locator('[data-col="compartments"]')).toHaveText('—');
  await expect(refRow(page, 'Daypack').locator('.compartments-btn')).toHaveText('Main, Front Pocket, Side Pocket');

  await page.locator('.filter-box').fill(name);
  await refRow(page, name).locator('.compartments-btn').click();
  const dialog = page.locator('.modal.compartments');
  await expect(dialog).toContainText('No compartments yet');
  // Enter adds one and waits for the next; the button does the same.
  let done = refSaved(page);
  await dialog.locator('.comp-new').fill('Top');
  await dialog.locator('.comp-new').press('Enter');
  await done;
  await expect(dialog.locator('.comp-new')).toBeFocused();
  await expect(dialog.locator('.comp-new')).toHaveValue('');
  done = refSaved(page);
  await dialog.locator('.comp-new').fill('Bottom');
  await dialog.locator('.comp-add').click();
  await done;
  await expect(dialog.locator('.comp-name')).toHaveCount(2);
  // A name it already has is refused.
  await dialog.locator('.comp-new').fill('Top');
  await dialog.locator('.comp-new').press('Enter');
  await expect(toast(page)).toContainText('already has a compartment with that name');
  await dialog.locator('.comp-new').fill('');
  // Renamed in place.
  done = refSaved(page, 'PATCH');
  await dialog.locator('.comp-row[data-name="Top"] .comp-name').fill('Lid');
  await dialog.locator('.comp-row[data-name="Top"] .comp-name').press('Enter');
  await done;
  await dialog.locator('.modal-foot button', { hasText: 'Done' }).click();
  await expect(refRow(page, name).locator('.compartments-btn')).toHaveText('Lid, Bottom');
  const made = (await (await page.request.get('/api/ref/compartments')).json()).filter((c) => c.item_id === item.id);
  expect(made.map((c) => c.name)).toEqual(['Lid', 'Bottom']);

  // Deleted from the same dialog.
  await refRow(page, name).locator('.compartments-btn').click();
  done = refSaved(page, 'DELETE');
  await dialog.locator('.comp-row[data-name="Bottom"] .comp-delete').click();
  await done;
  await expect(dialog.locator('.comp-name')).toHaveCount(1);
  await page.keyboard.press('Escape');
  await expect(refRow(page, name).locator('.compartments-btn')).toHaveText('Lid');
});

test('on a list, what is in a container is shown compartment by compartment', async ({ page, pack }) => {
  const list = await pack.list('Comp', [...DAY, ['Hygiene Kit', 'Daypack', { compartment: 'Main' }], ['Comb', 'Hygiene Kit']]);
  await openList(page, list);
  // Compartment by compartment, in the container's own order of them, then what is in none; containers first within each.
  expect(await rowNames(page)).toEqual(['Daypack', 'Hygiene Kit', 'Comb', 'Camera', 'Passport', 'Socks']);
  const pack_ = await box(page, 'Daypack').boundingBox();
  const main = compartment(page, 'Daypack', 'Main'), front = compartment(page, 'Daypack', 'Front Pocket'), side = compartment(page, 'Daypack', 'Side Pocket');
  await expect(main.locator('.pk-comp-name')).toHaveText('Main');
  for (const section of [main, front, side]) expect(inside(await section.boundingBox(), pack_)).toBe(true);
  // Each entry is drawn in its compartment; a container in a compartment brings its box along.
  expect(inside(await row(page, 'Camera').boundingBox(), await main.boundingBox())).toBe(true);
  expect(inside(await box(page, 'Hygiene Kit').boundingBox(), await main.boundingBox())).toBe(true);
  expect(inside(await row(page, 'Comb').boundingBox(), await box(page, 'Hygiene Kit').boundingBox())).toBe(true);
  expect(inside(await row(page, 'Passport').boundingBox(), await front.boundingBox())).toBe(true);
  expect(inside(await row(page, 'Passport').boundingBox(), await main.boundingBox())).toBe(false);
  // What is in no compartment is in the container all the same, outside them.
  expect(inside(await row(page, 'Socks').boundingBox(), pack_)).toBe(true);
  for (const section of [main, front, side]) expect(inside(await row(page, 'Socks').boundingBox(), await section.boundingBox())).toBe(false);
  // A compartment with nothing in it is there to be filled.
  await expect(side.locator('.pk-comp-empty')).toHaveText('empty');
  await expect(side.locator('.row.entry')).toHaveCount(0);
  // A container without compartments looks as it always did.
  await expect(box(page, 'Hygiene Kit').locator('.pk-comp')).toHaveCount(0);
  // While a filter is on, only the compartments with a match are shown.
  await page.locator('.filter-box').fill('passport');
  await expect(front).toBeVisible();
  await expect(main).toHaveCount(0);
  await expect(side).toHaveCount(0);
});

test('entries are added to a compartment and moved between compartments', async ({ page, pack }) => {
  const list = await pack.list('Comp', DAY);
  await openList(page, list);
  const side = compartment(page, 'Daypack', 'Side Pocket');
  // The + beside a compartment's name adds into it.
  await side.locator('.add-to-compartment').click();
  await expect(side.locator('.row.new')).toBeVisible();
  await page.keyboard.type('sun hat');
  await option(page, 'Sun Hat').click();
  let done = saved(page, 'POST');
  await page.keyboard.press('Enter');
  await done;
  expect((await pack.fetch(list.id)).entry('Sun Hat')).toMatchObject({ container: 'Daypack', compartment: 'Side Pocket' });
  await expect(side.locator('.row.entry[data-name="Sun Hat"]')).toBeVisible();
  // The row stays open in the same compartment for the next one. An item can be in a container only once, whatever
  // the compartment: the camera is in Main, so it is not offered for the side pocket.
  await expect(side.locator('.row.new')).toBeVisible();
  await draft(page).locator('.combo-input').fill('camera');
  await expect(page.locator('.combo-opt:not(.create)', { hasText: 'Camera' })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  const refused = await page.request.post(`/api/lists/${list.id}/contents`, { data: { item_id: pack.itemId('Camera'), container_id: pack.itemId('Daypack'), compartment_id: pack.compartmentId('Daypack', 'Side Pocket') } });
  expect(refused.status()).toBe(409);
  // The box button on the container's heading still adds to the container itself, in no compartment.
  await row(page, 'Daypack').hover();
  await row(page, 'Daypack').locator('button.add-inside').click();
  await page.keyboard.type('comb');
  await option(page, 'Comb').click();
  done = saved(page, 'POST');
  await page.keyboard.press('Enter');
  await done;
  expect((await pack.fetch(list.id)).entry('Comb')).toMatchObject({ container: 'Daypack', compartment: null });
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');

  // Moving: the menu offers the entry's other compartments, and the container itself to take it out of its compartment.
  const menu = async (name) => { await row(page, name).hover(); await row(page, name).locator('button.move').click(); };
  await menu('Passport');
  await expect(page.locator('.move-menu button')).toHaveText(['Top level', 'Daypack', 'Main', 'Side Pocket']);
  done = saved(page);
  await page.locator('.move-menu button.to-compartment', { hasText: 'Main' }).click();
  await done;
  expect((await pack.fetch(list.id)).entry('Passport').compartment).toBe('Main');
  expect(inside(await row(page, 'Passport').boundingBox(), await compartment(page, 'Daypack', 'Main').boundingBox())).toBe(true);
  await menu('Passport');
  done = saved(page);
  await page.locator('.move-menu button:not(.to-compartment)', { hasText: 'Daypack' }).click();
  await done;
  expect((await pack.fetch(list.id)).entry('Passport')).toMatchObject({ container: 'Daypack', compartment: null });
  // In no compartment: the container itself is not somewhere to go, its compartments are.
  await menu('Socks');
  await expect(page.locator('.move-menu button')).toHaveText(['Top level', 'Daypack', 'Main', 'Front Pocket', 'Side Pocket']);
  await expect(page.locator('.move-menu button:not(.to-compartment)', { hasText: 'Daypack' })).toBeDisabled();
  // Out of the container altogether leaves the compartment behind.
  done = saved(page);
  await page.locator('.move-menu button', { hasText: 'Top level' }).click();
  await done;
  expect((await pack.fetch(list.id)).entry('Socks')).toMatchObject({ container: null, compartment: null });
});

test('deleting a compartment leaves what was in it in the container', async ({ page, pack }) => {
  const name = `Pannier ${uid()}`;
  const item = await pack.createRef('items', { name, item_category_id: pack.ref.item_categories[0].id, storage_location_id: pack.ref.storage_locations[0].id, is_container: true });
  const pocket = await pack.createRef('compartments', { item_id: item.id, name: 'Pocket' });
  const list = await pack.list('Comp');
  for (const data of [{ item_id: item.id }, { item_id: pack.itemId('Comb'), container_id: item.id, compartment_id: pocket.id }]) {
    expect((await page.request.post(`/api/lists/${list.id}/contents`, { data })).status()).toBe(201);
  }
  await page.goto('/#/library');
  await page.locator('.filter-box').fill(name);
  await refRow(page, name).locator('.compartments-btn').click();
  const dialog = page.locator('.modal.compartments');
  await expect(dialog.locator('.comp-used')).toHaveText('1 entry');
  page.once('dialog', (d) => { expect(d.message()).toContain('1 entry'); d.accept(); });
  const done = refSaved(page, 'DELETE');
  await dialog.locator('.comp-delete').click();
  await done;
  expect((await pack.fetch(list.id)).entry('Comb')).toMatchObject({ container: name, compartment: null });
  await dialog.locator('.modal-foot button').click();
  await openList(page, list);
  await expect(box(page, name).locator('.pk-comp')).toHaveCount(0);
  await expect(row(page, 'Comb')).toBeVisible();
});
