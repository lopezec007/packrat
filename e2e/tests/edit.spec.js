// Changing what is on a list (static/js/pages/list.js, list/boxes.js,
// list/additem.js): adding entries, creating library items on the way,
// moving entries between containers and removing them.
import { test, expect, uid, openList, row, rowNames, selectedName, listMenu, saved, toast } from './helpers.js';

const draft = (page) => page.locator('.pack .row.new');
const option = (page, name) => page.locator('.combo-opt', { hasText: name }).first();

test('an empty list offers the first item; entries are added one after another', async ({ page, pack }) => {
  const list = await pack.list('Add');
  await openList(page, list);
  await expect(page.locator('.list-empty')).toContainText('This list is empty');
  await page.locator('.list-empty button').click();
  await expect(draft(page)).toBeVisible();

  // Type to search the library, pick, Enter: the row is added and the next one is ready.
  await page.keyboard.type('blue');
  await expect(option(page, 'Blue Backpack')).toBeVisible();
  await page.keyboard.press('Enter'); // picks the match
  let done = saved(page, 'POST');
  await page.keyboard.press('Enter'); // adds
  await done;
  await expect(row(page, 'Blue Backpack')).toBeVisible();
  await expect(draft(page)).toBeVisible();
  await expect(draft(page).locator('.combo-input')).toBeFocused();

  // With quantity, unit, status and notes.
  await page.keyboard.type('sock');
  await option(page, 'Socks').click();
  await draft(page).locator('.qty-input').fill('4');
  await draft(page).locator('.unit-input').fill('pairs');
  await draft(page).locator('select').selectOption({ label: 'Packed' });
  await draft(page).locator('.notes-input').fill('the thick ones');
  done = saved(page, 'POST');
  await draft(page).locator('button.save').click();
  await done;
  const e = (await pack.fetch(list.id)).entry('Socks');
  expect(e).toMatchObject({ container: null, quantity: 4, unit: 'pairs', status: 'Packed', notes: 'the thick ones' });
  // The next entry starts with the status just used; Escape closes the row.
  await expect(draft(page).locator('select')).toHaveValue(String(pack.statusId('Packed')));
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await expect(draft(page)).toHaveCount(0);
  expect(await rowNames(page)).toEqual(['Blue Backpack', 'Socks']);
});

test('items are added inside a container, and a container is only offered while it is not on the list', async ({ page, pack }) => {
  const list = await pack.list('Add', [['Car', null], ['Blue Backpack', 'Car'], ['Sun Hat', null]]);
  await openList(page, list);
  await row(page, 'Blue Backpack').hover();
  await row(page, 'Blue Backpack').locator('button.add-inside').click();
  await page.keyboard.type('comb');
  await option(page, 'Comb').click();
  let done = saved(page, 'POST');
  await page.keyboard.press('Enter');
  await done;
  expect((await pack.fetch(list.id)).entry('Comb').container).toBe('Blue Backpack');
  await expect(row(page, 'Comb')).toHaveAttribute('aria-level', '3');

  // The backpack is on the list already (a container has one place); the hat is at the top level, so it can go in here too.
  await draft(page).locator('.combo-input').fill('b');
  await expect(option(page, 'Camp Bin')).toBeVisible();
  await expect(page.locator('.combo-opt:not(.create)', { hasText: 'Blue Backpack' })).toHaveCount(0);
  await draft(page).locator('.combo-input').fill('sun hat');
  await expect(option(page, 'Sun Hat')).toBeVisible();
  // What is already in this container is not offered again, and its name does not create a second item.
  await draft(page).locator('.combo-input').fill('comb');
  await expect(page.locator('.combo-opt:not(.create)', { hasText: 'Comb' })).toHaveCount(0);
  await page.locator('.combo-opt.create').click();
  await expect(toast(page)).toContainText('already in this place');
  await expect(page.locator('.modal')).toHaveCount(0);
  await draft(page).locator('.combo-input').fill('blue backpack');
  await page.locator('.combo-opt.create').click();
  await expect(toast(page)).toContainText('already on this list');
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');

  // "Add item" in the title bar adds beside the selected entry, or inside the selected container.
  await row(page, 'Comb').locator('.item-name').click();
  await page.locator('#btn-add-item').click();
  await page.keyboard.type('toothbrush');
  await option(page, 'Toothbrush').click();
  done = saved(page, 'POST');
  await page.keyboard.press('Enter');
  await done;
  expect((await pack.fetch(list.id)).entry('Toothbrush').container).toBe('Blue Backpack');
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await row(page, 'Car').locator('.item-name').click();
  await page.locator('#btn-add-item').click();
  await page.keyboard.type('camp chair');
  await option(page, 'Camp Chair').click();
  done = saved(page, 'POST');
  await page.keyboard.press('Enter');
  await done;
  expect((await pack.fetch(list.id)).entry('Camp Chair').container).toBe('Car');
});

test('a name that is not in the library creates the item on the way, with a new category', async ({ page, pack }) => {
  const list = await pack.list('Add', [['Car', null]]);
  const name = `Kite ${uid()}`;
  const category = `Toys ${uid()}`;
  await openList(page, list);
  await page.locator('#btn-add-item').click();
  await page.keyboard.type(name);
  await page.locator('.combo-opt.create').click();
  const dialog = page.locator('.modal.new-item');
  await expect(dialog.locator('#new-item-name')).toHaveValue(name);
  // Create is refused until a category and a storage location are chosen.
  await dialog.locator('button.primary').click();
  await expect(toast(page)).toContainText('category');
  // A new category from the end of the list.
  await dialog.locator('select').first().selectOption({ label: 'New category…' });
  await page.locator('.modal').last().locator('input[type="text"]').fill(category);
  await page.locator('.modal').last().locator('button.primary').click();
  await expect(dialog.locator('select').first().locator('option:checked')).toHaveText(category);
  await dialog.locator('select').nth(1).selectOption({ label: 'Hall Closet' });
  await dialog.locator('button.primary').click();
  await expect(dialog).toHaveCount(0);
  const done = saved(page, 'POST');
  await page.keyboard.press('Enter');
  await done;
  await pack.track('items', name);
  await pack.track('item_categories', category);
  await expect(row(page, name)).toBeVisible();
  await expect(row(page, name).locator('.category')).toHaveText(category);
  expect((await pack.fetch(list.id)).entry(name)).toBeTruthy();
});

test('an entry moves into another container and takes its contents along', async ({ page, pack }) => {
  const list = await pack.list('Move', [
    ['Car', null], ['Blue Backpack', 'Car'], ['Hygiene Kit', 'Blue Backpack'], ['Comb', 'Hygiene Kit'],
    ['Camp Bin', 'Car'], ['Socks', 'Blue Backpack'], ['Socks', 'Camp Bin'],
  ]);
  await openList(page, list);
  const moveTo = async (name, target) => {
    await row(page, name).hover();
    await row(page, name).locator('button.move').click();
    const done = saved(page);
    await page.locator('.move-menu button', { hasText: target }).click();
    await done;
  };
  // The kit's menu offers every container but the one it is in and itself.
  await row(page, 'Hygiene Kit').hover();
  await row(page, 'Hygiene Kit').locator('button.move').click();
  await expect(page.locator('.move-menu button')).toHaveText(['Top level', 'Car', 'Camp Bin']);
  await page.keyboard.press('Escape');
  // A container is not offered a place inside itself or its own contents.
  await row(page, 'Blue Backpack').hover();
  await row(page, 'Blue Backpack').locator('button.move').click();
  await expect(page.locator('.move-menu button')).toHaveText(['Top level', 'Camp Bin']);
  await page.keyboard.press('Escape');
  // The socks in the backpack cannot go where there are socks already (the camp bin).
  await row(page, 'Socks').hover();
  await row(page, 'Socks').locator('button.move').click();
  await expect(page.locator('.move-menu button')).toHaveText(['Top level', 'Car', 'Hygiene Kit']);
  await page.keyboard.press('Escape');

  await moveTo('Hygiene Kit', 'Camp Bin');
  let now = await pack.fetch(list.id);
  expect(now.entry('Hygiene Kit').container).toBe('Camp Bin');
  expect(now.entry('Comb').container).toBe('Hygiene Kit'); // it went along
  expect(await selectedName(page)).toBe('Hygiene Kit');
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Socks', 'Camp Bin', 'Hygiene Kit', 'Comb', 'Socks']);

  await moveTo('Camp Bin', 'Top level');
  now = await pack.fetch(list.id);
  expect(now.entry('Camp Bin').container).toBeNull();
  expect(await rowNames(page)).toEqual(['Camp Bin', 'Hygiene Kit', 'Comb', 'Socks', 'Car', 'Blue Backpack', 'Socks']);
});

test('removing an entry can be undone; removing a container asks about its contents', async ({ page, pack }) => {
  const list = await pack.list('Remove', [
    ['Car', null], ['Blue Backpack', 'Car'], ['Hygiene Kit', 'Blue Backpack'], ['Comb', 'Hygiene Kit'], ['Toothbrush', 'Hygiene Kit'],
    ['Socks', 'Blue Backpack', { quantity: 3, unit: 'pairs', notes: 'thick', status: 'Packed' }], ['Sun Hat', null],
  ]);
  await openList(page, list);
  const remove = async (name) => { await row(page, name).hover(); await row(page, name).locator('button.delete').click(); };

  // A plain entry goes at once, with an Undo that puts it back as it was.
  let done = saved(page, 'DELETE');
  await remove('Socks');
  await done;
  await expect(row(page, 'Socks')).toHaveCount(0);
  expect((await pack.fetch(list.id)).entry('Socks')).toBeUndefined();
  done = saved(page, 'POST');
  await toast(page).locator('button', { hasText: 'Undo' }).click();
  await done;
  expect((await pack.fetch(list.id)).entry('Socks')).toMatchObject({ container: 'Blue Backpack', quantity: 3, unit: 'pairs', notes: 'thick', status: 'Packed' });
  await expect(row(page, 'Socks')).toBeVisible();

  // A container with contents: keep them (they move up one level)...
  await remove('Hygiene Kit');
  const dialog = page.locator('.modal');
  await expect(dialog).toContainText('2 entries inside');
  await dialog.locator('button', { hasText: 'Cancel' }).click();
  expect((await pack.fetch(list.id)).entry('Hygiene Kit')).toBeTruthy();
  await remove('Hygiene Kit');
  done = saved(page, 'DELETE');
  await dialog.locator('button', { hasText: 'Keep the contents' }).click();
  await done;
  await expect(row(page, 'Hygiene Kit')).toHaveCount(0);
  let now = await pack.fetch(list.id);
  expect(now.entry('Hygiene Kit')).toBeUndefined();
  expect(now.entry('Comb').container).toBe('Blue Backpack');
  expect(now.entry('Toothbrush').container).toBe('Blue Backpack');

  // ...or remove everything inside, at every depth.
  await remove('Car');
  await expect(dialog).toContainText('4 entries inside');
  done = saved(page, 'DELETE');
  await dialog.locator('button', { hasText: 'Remove all 5' }).click();
  await done;
  now = await pack.fetch(list.id);
  expect(now.entries.map((e) => e.item)).toEqual(['Sun Hat']);
  expect(await rowNames(page)).toEqual(['Sun Hat']);
});

test('the list menu renames the list, flags it as a template and gives every entry a status', async ({ page, pack }) => {
  const list = await pack.list('Menu', [['Car', null, { status: 'Packed' }], ['Sun Hat', 'Car', { status: 'Missing' }]]);
  await openList(page, list);
  const menu = (label) => listMenu(page, label);

  const renamed = `Renamed ${uid()}`;
  await menu('Rename');
  await page.locator('.modal input[type="text"]').fill(renamed);
  let done = saved(page);
  await page.locator('.modal button.primary').click();
  await done;
  await expect(page.locator('.list-switch option:checked')).toHaveText(renamed);
  await expect(page).toHaveTitle(new RegExp(renamed));
  expect((await pack.fetch(list.id)).name).toBe(renamed);

  await expect(page.locator('.template-chip')).toBeHidden();
  done = saved(page);
  await menu('Make it a template');
  await done;
  await expect(page.locator('.template-chip')).toBeVisible();
  expect((await pack.fetch(list.id)).is_template).toBe(true);
  done = saved(page);
  await menu('Stop being a template');
  await done;
  await expect(page.locator('.template-chip')).toBeHidden();

  await menu('Give every entry a status');
  done = saved(page, 'POST');
  await page.locator('.status-pick .status-chip', { hasText: 'In Storage Location' }).click();
  await done;
  expect((await pack.fetch(list.id)).entries.map((e) => e.status)).toEqual(['In Storage Location', 'In Storage Location']);
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveText('In Storage Location');
});
