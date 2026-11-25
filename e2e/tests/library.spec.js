// The Library and Settings pages (static/js/pages/library.js, settings.js,
// refedit.js): the items, their categories and storage locations, and the
// statuses with their colours and keys.
import { test, expect, uid, openList, row, saved, toast } from './helpers.js';

const refRow = (page, name) => page.locator(`.ref-table .row[data-name="${name}"]`);
const field = (rowLocator, col) => rowLocator.locator(`[data-col="${col}"]`).locator('input, select').first();
const draft = (page) => page.locator('.ref-table .row.new');
const refSaved = (page, method = 'PATCH') => page.waitForResponse((r) => r.url().includes('/api/ref/') && r.request().method() === method);

test('the library lists the items; sections are deep-linkable', async ({ page }) => {
  await page.goto('/#/library');
  await expect(page.locator('.nav a[aria-current="page"]')).toHaveText('Library');
  await expect(page.locator('.settings-nav a[aria-current="page"]')).toContainText('Items');
  // The first section's button is whole: nothing lies over its top edge.
  const top = await page.locator('.settings-nav a').first().evaluate((a) => {
    const r = a.getBoundingClientRect();
    return document.elementFromPoint(r.left + r.width / 2, r.top + 1) === a || a.contains(document.elementFromPoint(r.left + r.width / 2, r.top + 1));
  });
  expect(top).toBe(true);
  await expect(refRow(page, 'Blue Backpack')).toBeVisible();
  await expect(field(refRow(page, 'Blue Backpack'), 'is_container')).toBeChecked();
  await expect(field(refRow(page, 'Blue Backpack'), 'item_category_id').locator('option:checked')).toHaveText('Bags');
  await expect(refRow(page, 'Blue Backpack').locator('[data-col="list_count"]')).toContainText(/\d+ lists?/);
  // The filter looks at the category and the storage location too.
  await page.locator('.filter-box').fill('bathroom');
  await expect(refRow(page, 'Toothbrush')).toBeVisible();
  await expect(refRow(page, 'Blue Backpack')).toBeHidden();
  await page.goto('/#/library?section=storage-locations');
  await expect(page.locator('.settings-body h2')).toHaveText('Storage Locations');
  await expect(refRow(page, 'Hall Closet')).toBeVisible();
  await page.locator('.settings-nav a', { hasText: 'Categories' }).click();
  await expect(refRow(page, 'Clothing').locator('[data-col="item_count"]')).toContainText('items');
});

test('an item is added, edited in place and deleted; names are unique', async ({ page, pack }) => {
  const name = `Kite ${uid()}`;
  const category = `Toys ${uid()}`;
  await page.goto('/#/library');
  await page.locator('#btn-add').click();
  await field(draft(page), 'name').fill(name);
  // A category made on the spot from the end of the list.
  await field(draft(page), 'item_category_id').selectOption({ label: 'New category…' });
  await page.locator('.modal input[type="text"]').fill(category);
  await page.locator('.modal button.primary').click();
  await expect(field(draft(page), 'item_category_id').locator('option:checked')).toHaveText(category);
  // Saving without a storage location is refused.
  await draft(page).locator('button.save').click();
  await expect(toast(page)).toContainText('Stored in is required');
  await field(draft(page), 'storage_location_id').selectOption({ label: 'Hall Closet' });
  let done = refSaved(page, 'POST');
  await field(draft(page), 'name').press('Enter');
  await done;
  const item = await pack.track('items', name);
  await pack.track('item_categories', category);
  expect(item).toMatchObject({ name, is_container: false, list_count: 0 });
  await expect(refRow(page, name)).toBeVisible();

  // Every field is a live control.
  done = refSaved(page);
  await field(refRow(page, name), 'is_container').check();
  await done;
  done = refSaved(page);
  await field(refRow(page, name), 'storage_location_id').selectOption({ label: 'Garage Shelf A' });
  await done;
  const renamed = `${name} II`;
  done = refSaved(page);
  await field(refRow(page, name), 'name').fill(renamed);
  await field(refRow(page, name), 'name').press('Enter');
  await done;
  const now = (await (await page.request.get('/api/ref/items')).json()).find((i) => i.id === item.id);
  expect(now).toMatchObject({ name: renamed, is_container: true });

  // A name another item has is refused and put back.
  await field(refRow(page, renamed), 'name').fill('Comb');
  await field(refRow(page, renamed), 'name').press('Enter');
  await expect(toast(page)).toContainText('already an item with that name');
  await expect(field(refRow(page, renamed), 'name')).toHaveValue(renamed);

  page.once('dialog', (d) => d.accept());
  done = refSaved(page, 'DELETE');
  await refRow(page, renamed).locator('button.delete').click();
  await done;
  await expect(refRow(page, renamed)).toHaveCount(0);
});

test('an item on a list cannot be deleted, and a container holding things cannot stop being one', async ({ page, pack }) => {
  const list = await pack.list('Guard', [['Camp Bin', null], ['Headlamp', 'Camp Bin']]);
  await page.goto('/#/library');
  page.once('dialog', (d) => d.accept());
  await refRow(page, 'Headlamp').locator('button.delete').click();
  await expect(toast(page)).toContainText('is on a list');
  await expect(refRow(page, 'Headlamp')).toBeVisible();

  await field(refRow(page, 'Camp Bin'), 'is_container').uncheck();
  await expect(toast(page)).toContainText('holds other items');
  await expect(field(refRow(page, 'Camp Bin'), 'is_container')).toBeChecked();
  expect(list.id).toBeGreaterThan(0);
});

test('a status is added with a colour and a key, and the key then works on a list', async ({ page, pack }) => {
  const name = `Staged ${uid()}`;
  const list = await pack.list('Status', [['Sun Hat', null]]);
  await page.goto('/#/settings');
  await expect(refRow(page, 'In Storage Location').locator('.chip')).toHaveText('default');
  await page.locator('#btn-add').click();
  await field(draft(page), 'name').fill(name);
  // A key another status has is refused.
  await field(draft(page), 'shortcut').fill('P');
  await draft(page).locator('button.save').click();
  await expect(toast(page)).toContainText('already uses that shortcut key');
  await field(draft(page), 'shortcut').fill('G');
  await draft(page).locator('[data-col="color"] .swatch').click();
  await page.locator('.swatch-pop .swatch-opt[title="#f28e2b"]').click();
  let done = refSaved(page, 'POST');
  await draft(page).locator('button.save').click();
  await done;
  const status = await pack.track('statuses', name);
  expect(status).toMatchObject({ name, color: '#f28e2b', shortcut: 'g' }); // stored in lower case

  // On a list: the legend shows it with its key, and the key assigns it.
  await openList(page, list);
  await expect(page.locator('.legend-row .status-chip', { hasText: name }).locator('.key-cap')).toHaveText('g');
  await row(page, 'Sun Hat').locator('.item-name').click();
  done = saved(page);
  await page.keyboard.press('g');
  await done;
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveText(name);
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveCSS('background-color', 'rgb(242, 142, 43)');

  // In use, it cannot be deleted; its key can be changed.
  await page.goto('/#/settings');
  page.once('dialog', (d) => d.accept());
  await refRow(page, name).locator('button.delete').click();
  await expect(toast(page)).toContainText('in use on a list');
  done = refSaved(page);
  await field(refRow(page, name), 'shortcut').fill('7');
  await field(refRow(page, name), 'shortcut').press('Enter');
  await done;
  await openList(page, list);
  await row(page, 'Sun Hat').locator('.item-name').click();
  done = saved(page);
  await page.keyboard.press('p');
  await done;
  done = saved(page);
  await page.keyboard.press('7');
  await done;
  expect((await pack.fetch(list.id)).entry('Sun Hat').status).toBe(name);
});

test('Enter saves a new item and opens the next one, with the same category and place', async ({ page, pack }) => {
  const a = `Tarp ${uid()}`, b = `Tarp pegs ${uid()}`, c = `Tarp line ${uid()}`;
  await page.goto('/#/library');
  await page.locator('#btn-add').click();
  await field(draft(page), 'name').fill(a);
  await field(draft(page), 'item_category_id').selectOption({ label: 'Camp' });
  await field(draft(page), 'storage_location_id').selectOption({ label: 'Garage Shelf B' });
  let done = refSaved(page, 'POST');
  await field(draft(page), 'name').press('Enter');
  await done;
  await pack.track('items', a);
  // The item is in the table, and a new row is waiting with the cursor in its name.
  await expect(refRow(page, a)).toBeVisible();
  await expect(draft(page)).toHaveCount(1);
  await expect(field(draft(page), 'name')).toBeFocused();
  await expect(field(draft(page), 'name')).toHaveValue('');
  await expect(field(draft(page), 'item_category_id').locator('option:checked')).toHaveText('Camp');
  await expect(field(draft(page), 'storage_location_id').locator('option:checked')).toHaveText('Garage Shelf B');
  await expect(field(draft(page), 'is_container')).not.toBeChecked();
  // So the next one is just its name and Enter.
  done = refSaved(page, 'POST');
  await page.keyboard.type(b);
  await page.keyboard.press('Enter');
  await done;
  await pack.track('items', b);
  await expect(draft(page)).toHaveCount(1);
  await expect(field(draft(page), 'name')).toBeFocused();
  // Escape on the waiting row ends the run.
  await page.keyboard.press('Escape');
  await expect(draft(page)).toHaveCount(0);
  const items = await (await page.request.get('/api/ref/items')).json();
  const camp = pack.ref.item_categories.find((x) => x.name === 'Camp').id, shelf = pack.ref.storage_locations.find((x) => x.name === 'Garage Shelf B').id;
  for (const name of [a, b]) expect(items.find((i) => i.name === name)).toMatchObject({ item_category_id: camp, storage_location_id: shelf });
  // The tick button saves just the one.
  await page.locator('#btn-add').click();
  await field(draft(page), 'name').fill(c);
  await field(draft(page), 'item_category_id').selectOption({ label: 'Camp' });
  await field(draft(page), 'storage_location_id').selectOption({ label: 'Hall Closet' });
  done = refSaved(page, 'POST');
  await draft(page).locator('button.save').click();
  await done;
  await pack.track('items', c);
  await expect(refRow(page, c)).toBeVisible();
  await expect(draft(page)).toHaveCount(0);
});
