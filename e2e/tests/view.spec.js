// A list (static/js/list/boxes.js): containers as boxes within boxes with
// their items listed inside, tinted by status; opening and closing
// containers, the arrow keys, and the status keys.
import { test, expect, openList, row, box, rowNames, selectedName, listMenu, saved, toast } from './helpers.js';

// Car > Blue Backpack > Hygiene Kit > (Comb, Toothbrush); Car > Camp Chair; and a loose Sun Hat.
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

const inside = (inner, outer) => inner.x >= outer.x - 0.5 && inner.y >= outer.y - 0.5
  && inner.x + inner.width <= outer.x + outer.width + 0.5 && inner.y + inner.height <= outer.y + outer.height + 0.5;

test('containers are boxes within boxes, and the items in each are listed inside it with all their fields', async ({ page, pack }) => {
  const list = await pack.list('Boxes', [...TRIP, ['Camp Bin', 'Car'], ['Headlamp', 'Camp Bin']]);
  await openList(page, list);
  // In reading order: a container's heading, the containers inside it, then the items packed straight into it.
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Comb', 'Toothbrush', 'Socks', 'Camp Bin', 'Headlamp', 'Camp Chair', 'Sun Hat']);
  await expect(row(page, 'Car')).toHaveAttribute('aria-level', '1');
  await expect(row(page, 'Blue Backpack')).toHaveAttribute('aria-level', '2');
  await expect(row(page, 'Hygiene Kit')).toHaveAttribute('aria-level', '3');
  await expect(row(page, 'Comb')).toHaveAttribute('aria-level', '4');
  await expect(row(page, 'Sun Hat')).toHaveAttribute('aria-level', '1');

  // Each container's box holds its contents; what is in no container is outside every box.
  const rect = async (loc) => loc.boundingBox();
  const car = await rect(box(page, 'Car')), bag = await rect(box(page, 'Blue Backpack')), kit = await rect(box(page, 'Hygiene Kit')), bin = await rect(box(page, 'Camp Bin'));
  expect(inside(bag, car)).toBe(true);
  expect(inside(bin, car)).toBe(true);
  expect(inside(kit, bag)).toBe(true);
  expect(inside(await rect(row(page, 'Comb')), kit)).toBe(true);
  expect(inside(await rect(row(page, 'Socks')), bag)).toBe(true);
  expect(inside(await rect(row(page, 'Socks')), kit)).toBe(false);
  expect(inside(await rect(row(page, 'Headlamp')), bin)).toBe(true);
  expect(inside(await rect(row(page, 'Camp Chair')), car)).toBe(true);
  expect(inside(await rect(row(page, 'Sun Hat')), car)).toBe(false);
  // The two containers in the car sit side by side; the items in one are a list, one under another.
  expect(Math.abs(bag.y - bin.y)).toBeLessThan(1);
  expect(bin.x).toBeGreaterThanOrEqual(bag.x + bag.width);
  const comb = await rect(row(page, 'Comb')), brush = await rect(row(page, 'Toothbrush'));
  expect(brush.y).toBeGreaterThanOrEqual(comb.y + comb.height - 1);
  expect(Math.abs(brush.x - comb.x)).toBeLessThan(1);

  // A container says how many entries are inside it, at every depth.
  await expect(row(page, 'Car').locator('.inside-count')).toHaveText('8');
  await expect(row(page, 'Hygiene Kit').locator('.inside-count')).toHaveText('2');
  // Every field of an entry is on its row, a container's on its heading.
  const socks = row(page, 'Socks');
  await expect(socks.locator('.qty-input')).toHaveValue('3');
  await expect(socks.locator('.unit-input')).toHaveValue('pairs');
  await expect(socks.locator('.category')).toHaveText('Clothing');
  await expect(socks.locator('.location')).toHaveText('Bedroom Dresser');
  await expect(socks.locator('.status-chip')).toHaveText('In Storage Location');
  await expect(socks.locator('.notes-input')).toBeVisible();
  const head = row(page, 'Blue Backpack');
  await expect(head.locator('.qty-input')).toHaveValue('1');
  await expect(head.locator('.category')).toHaveText('Bags');
  await expect(head.locator('.location')).toHaveText('Hall Closet');
  await expect(head.locator('.status-chip')).toHaveText('In Storage Location');
  await expect(head.locator('.notes-input')).toBeVisible();
  // There is one view: no switch between a list and a treemap.
  await expect(page.locator('#btn-view-map, #btn-view-tree')).toHaveCount(0);
});

test('entries and containers are tinted by their own status', async ({ page, pack }) => {
  const list = await pack.list('Tint', [
    ['Car', null], ['Blue Backpack', 'Car', { status: 'Packed' }], ['Comb', 'Blue Backpack'], ['Socks', 'Blue Backpack', { status: 'Missing' }], ['Sun Hat', null, { status: 'Packed' }],
  ]);
  await openList(page, list);
  const color = Object.fromEntries(pack.ref.statuses.map((s) => [s.name, s.color]));
  const tint = (loc) => loc.evaluate((e) => getComputedStyle(e).getPropertyValue('--c').trim());
  const rgb = (hex) => { const n = parseInt(hex.slice(1), 16); return `rgb(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255})`; };
  expect(await tint(row(page, 'Socks'))).toBe(color.Missing);
  expect(await tint(row(page, 'Comb'))).toBe(color['In Storage Location']);
  expect(await tint(row(page, 'Sun Hat'))).toBe(color.Packed);
  await expect(row(page, 'Socks').locator('.status-chip')).toHaveCSS('background-color', rgb(color.Missing));
  // The backpack is Packed although what is in it is not: its box and heading take its own colour.
  expect(await tint(box(page, 'Blue Backpack'))).toBe(color.Packed);
  expect(await tint(row(page, 'Blue Backpack'))).toBe(color.Packed);
  expect(await tint(box(page, 'Car'))).toBe(color['In Storage Location']);
  // A row's tint is its colour mixed into the page, not the page colour itself.
  const bg = await row(page, 'Socks').evaluate((e) => getComputedStyle(e).backgroundColor);
  expect(bg).not.toBe(await page.evaluate(() => getComputedStyle(document.body).backgroundColor));

  // A status key retints the selected entry, and nothing else.
  await row(page, 'Comb').locator('.item-name').click();
  const done = saved(page);
  await page.keyboard.press('m');
  await done;
  expect(await tint(row(page, 'Comb'))).toBe(color.Missing);
  expect(await tint(box(page, 'Blue Backpack'))).toBe(color.Packed);
  expect(await tint(box(page, 'Car'))).toBe(color['In Storage Location']);
});

test('a caret closes and opens one container; the title bar closes and opens them all; the state survives a reload', async ({ page, pack }) => {
  const list = await pack.list('Tree', TRIP);
  await openList(page, list);
  await row(page, 'Hygiene Kit').locator('.caret').click();
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Socks', 'Camp Chair', 'Sun Hat']);
  await expect(row(page, 'Hygiene Kit')).toHaveAttribute('aria-expanded', 'false');

  await page.reload();
  await expect(row(page, 'Car')).toBeVisible();
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Socks', 'Camp Chair', 'Sun Hat']);

  await page.locator('#btn-collapse-all').click();
  expect(await rowNames(page)).toEqual(['Car', 'Sun Hat']);
  await page.locator('#btn-expand-all').click();
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Comb', 'Toothbrush', 'Socks', 'Camp Chair', 'Sun Hat']);

  // Closing a container with the selection inside it selects the container.
  await row(page, 'Comb').locator('.item-name').click();
  await row(page, 'Blue Backpack').locator('.caret').click();
  expect(await selectedName(page)).toBe('Blue Backpack');
});

test('the arrow keys move the selection to the nearest entry in that direction', async ({ page, pack }) => {
  const entries = [...TRIP, ['Camp Bin', 'Car'], ['Headlamp', 'Camp Bin'], ['Mess Kit', 'Camp Bin']];
  const list = await pack.list('Arrows', entries);
  await openList(page, list);
  expect(await selectedName(page)).toBe('');
  await page.keyboard.press('ArrowDown'); // nothing selected yet: the first entry
  expect(await selectedName(page)).toBe('Car');
  await page.keyboard.press('ArrowDown'); // into the car: the first box inside it
  expect(await selectedName(page)).toBe('Blue Backpack');
  await page.keyboard.press('ArrowRight'); // across to the box beside it
  expect(await selectedName(page)).toBe('Camp Bin');
  await page.keyboard.press('ArrowDown'); // down its list
  expect(await selectedName(page)).toBe('Headlamp');
  await page.keyboard.press('ArrowDown');
  expect(await selectedName(page)).toBe('Mess Kit');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('ArrowUp');
  expect(await selectedName(page)).toBe('Camp Bin');
  await page.keyboard.press('ArrowLeft');
  expect(await selectedName(page)).toBe('Blue Backpack');
  await page.keyboard.press('ArrowUp');
  expect(await selectedName(page)).toBe('Car');

  await page.keyboard.press('End');
  expect(await selectedName(page)).toBe('Sun Hat');
  await page.keyboard.press('Home');
  expect(await selectedName(page)).toBe('Car');
  await page.keyboard.press('ArrowUp'); // nothing above: stays
  expect(await selectedName(page)).toBe('Car');
  // Space closes and opens the selected container.
  await page.keyboard.press(' ');
  expect(await rowNames(page)).toEqual(['Car', 'Sun Hat']);
  await page.keyboard.press(' ');
  expect((await rowNames(page)).length).toBe(entries.length);
  // Escape selects the container the selection is in.
  await row(page, 'Comb').locator('.item-name').click();
  await page.keyboard.press('Escape');
  expect(await selectedName(page)).toBe('Hygiene Kit');
  await page.keyboard.press('Escape');
  expect(await selectedName(page)).toBe('Blue Backpack');

  // From every entry, each arrow either stays put (nothing that way) or lands on an entry that really lies that way...
  const names = await rowNames(page);
  const rect = (name) => row(page, name).boundingBox();
  for (const name of names) {
    for (const [key, lies] of [
      ['ArrowRight', (a, b) => b.x >= a.x + a.width - 1],
      ['ArrowLeft', (a, b) => b.x + b.width <= a.x + 1],
      ['ArrowDown', (a, b) => b.y >= a.y + a.height - 1],
      ['ArrowUp', (a, b) => b.y + b.height <= a.y + 1],
    ]) {
      await row(page, name).locator('.item-name').click();
      const from = await rect(name);
      await page.keyboard.press(key);
      const to = await selectedName(page);
      if (to !== name) expect(lies(from, await rect(to)), `${key} from ${name} went to ${to}`).toBe(true);
    }
  }
  // ...and Down alone, from the top, passes through a whole column of them; every entry can be reached with the arrows.
  const reached = new Set();
  const walk = async (depth) => {
    const here = await selectedName(page);
    if (reached.has(here) || depth > 60) return;
    reached.add(here);
    for (const key of ['ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp']) {
      await page.keyboard.press(key);
      await walk(depth + 1);
      await row(page, here).locator('.item-name').click();
    }
  };
  await row(page, 'Car').locator('.item-name').click();
  await walk(0);
  expect([...reached].sort()).toEqual([...names].sort());
});

test('a status key gives the selected entry that status, and only that entry', async ({ page, pack }) => {
  const list = await pack.list('Keys', TRIP);
  await openList(page, list);
  await row(page, 'Comb').locator('.item-name').click();
  expect(await selectedName(page)).toBe('Comb');

  let done = saved(page);
  await page.keyboard.press('p');
  await done;
  await expect(row(page, 'Comb').locator('.status-chip')).toHaveText('Packed');
  expect((await pack.fetch(list.id)).entry('Comb').status).toBe('Packed');
  expect(await selectedName(page)).toBe('Comb'); // the selection stays put

  // The key works whatever the state of Shift; with Shift the selection moves on.
  done = saved(page);
  await page.keyboard.press('Shift+M');
  await done;
  await expect(row(page, 'Comb').locator('.status-chip')).toHaveText('Missing');
  expect(await selectedName(page)).toBe('Toothbrush');

  // Packing everything inside a container leaves the container's own status alone.
  done = saved(page);
  await page.keyboard.press('p');
  await done;
  await row(page, 'Comb').locator('.item-name').click();
  done = saved(page);
  await page.keyboard.press('p');
  await done;
  const now = await pack.fetch(list.id);
  expect(now.entry('Comb').status).toBe('Packed');
  expect(now.entry('Toothbrush').status).toBe('Packed');
  expect(now.entry('Hygiene Kit').status).toBe('In Storage Location');
  expect(now.entry('Blue Backpack').status).toBe('In Storage Location');
  await expect(row(page, 'Hygiene Kit').locator('.status-chip')).toHaveText('In Storage Location');

  // A container takes a status of its own like any other entry.
  await row(page, 'Hygiene Kit').locator('.item-name').click();
  done = saved(page);
  await page.keyboard.press('m');
  await done;
  expect((await pack.fetch(list.id)).entry('Hygiene Kit').status).toBe('Missing');
  expect((await pack.fetch(list.id)).entry('Comb').status).toBe('Packed');

  // The legend counts follow.
  await expect(page.locator('.legend-row .status-chip', { hasText: 'Packed' }).locator('.count')).toHaveText('2');
  await expect(page.locator('.legend-row .status-chip', { hasText: 'Missing' }).locator('.count')).toHaveText('1');
});

test('keys typed into a text box are text, not commands', async ({ page, pack }) => {
  const list = await pack.list('Keys', TRIP);
  await openList(page, list);
  await row(page, 'Socks').locator('.item-name').click();
  const notes = row(page, 'Socks').locator('.notes-input');
  await notes.click();
  await page.keyboard.type('pm spares');
  const done = saved(page);
  await page.keyboard.press('Enter'); // saves and hands the keys back to the list
  await done;
  const now = await pack.fetch(list.id);
  expect(now.entry('Socks').notes).toBe('pm spares');
  expect(now.entry('Socks').status).toBe('In Storage Location');
  // ...and now the keys are commands again.
  const again = saved(page);
  await page.keyboard.press('p');
  await again;
  expect((await pack.fetch(list.id)).entry('Socks').status).toBe('Packed');
  // The filter box keeps its keys too.
  await page.locator('.filter-box').click();
  await page.keyboard.type('m');
  await expect(page.locator('.filter-box')).toHaveValue('m');
  expect((await pack.fetch(list.id)).entry('Socks').status).toBe('Packed');
});

test('quantity, unit and notes are edited in place', async ({ page, pack }) => {
  const list = await pack.list('Edit', TRIP);
  await openList(page, list);
  const socks = row(page, 'Socks');
  let done = saved(page);
  await socks.locator('.qty-input').fill('5');
  await socks.locator('.qty-input').press('Enter');
  await done;
  done = saved(page);
  await socks.locator('.unit-input').fill('');
  await socks.locator('.unit-input').press('Enter');
  await done;
  let e = (await pack.fetch(list.id)).entry('Socks');
  expect(e.quantity).toBe(5);
  expect(e.unit).toBeNull();

  // Not a whole number from 1 up: refused, and the old value comes back.
  await socks.locator('.qty-input').fill('0');
  await socks.locator('.qty-input').press('Enter');
  await expect(toast(page)).toContainText('whole number');
  await expect(socks.locator('.qty-input')).toHaveValue('5');
  // Escape drops what was typed.
  await socks.locator('.qty-input').fill('99');
  await socks.locator('.qty-input').press('Escape');
  await expect(socks.locator('.qty-input')).toHaveValue('5');
  expect((await pack.fetch(list.id)).entry('Socks').quantity).toBe(5);

  // Enter on a selected row starts editing its quantity.
  await row(page, 'Camp Chair').locator('.item-name').click();
  await page.keyboard.press('Enter');
  await expect(row(page, 'Camp Chair').locator('.qty-input')).toBeFocused();
});

test('the status chip offers the statuses to the mouse', async ({ page, pack }) => {
  const list = await pack.list('Chip', TRIP);
  await openList(page, list);
  await row(page, 'Sun Hat').locator('.status-chip').click();
  const done = saved(page);
  await page.locator('.status-menu button', { hasText: 'Missing' }).click();
  await done;
  await expect(row(page, 'Sun Hat').locator('.status-chip')).toHaveText('Missing');
  // ...and so does the legend, for the selected entry.
  await row(page, 'Camp Chair').locator('.item-name').click();
  const again = saved(page);
  await page.locator('.legend-row .status-chip', { hasText: 'Packed' }).click();
  await again;
  expect((await pack.fetch(list.id)).entry('Camp Chair').status).toBe('Packed');
});

test('the filter keeps matches and the containers they are in; sorting reorders within a container', async ({ page, pack }) => {
  const list = await pack.list('Filter', [...TRIP, ['Camera', 'Blue Backpack', { status: 'Packed' }]]);
  await openList(page, list);
  await page.locator('.filter-box').fill('tooth');
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Toothbrush']);
  await expect(row(page, 'Car')).toHaveClass(/dim/);
  await expect(row(page, 'Toothbrush')).not.toHaveClass(/dim/);
  await expect(page.locator('.legend-total')).toContainText('1 of 9');
  await page.locator('.filter-box').fill('');

  // By status: only the packed camera, with its containers.
  await page.locator('.multi-btn', { hasText: 'Status' }).click();
  await page.locator('.multi-opt', { hasText: 'Packed' }).click();
  await page.keyboard.press('Escape');
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Camera']);
  await page.locator('.multi-btn', { hasText: 'Status' }).click();
  await page.locator('.multi-actions button', { hasText: 'None' }).click();
  await page.keyboard.press('Escape');

  // Sorted by category, the backpack's loose items go Clothing (Socks) then Electronics (Camera); by name the other way.
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Comb', 'Toothbrush', 'Camera', 'Socks', 'Camp Chair', 'Sun Hat']);
  await listMenu(page, 'Sort by category');
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Comb', 'Toothbrush', 'Socks', 'Camera', 'Camp Chair', 'Sun Hat']);
  await listMenu(page, 'Sort by name');
  expect(await rowNames(page)).toEqual(['Car', 'Blue Backpack', 'Hygiene Kit', 'Comb', 'Toothbrush', 'Camera', 'Socks', 'Camp Chair', 'Sun Hat']);
});

test('the title bar stays on one line, legend included, however many statuses there are', async ({ page, pack }) => {
  const list = await pack.list('One line with a fairly long name', TRIP, { template: true });
  await openList(page, list);
  const oneLine = async () => {
    const bar = await page.locator('.topbar').boundingBox();
    expect(bar.height).toBeLessThan(60);
    // Nothing is pushed out of the bar or under its neighbour.
    const menu = await page.locator('#btn-list-menu').boundingBox();
    const version = await page.locator('#app-version').boundingBox();
    expect(menu.x + menu.width).toBeLessThanOrEqual(version.x + 1);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0);
  };
  await oneLine();
  await expect(page.locator('.legend-row .status-chip')).toHaveCount(pack.ref.statuses.length);
  await expect(page.locator('.legend-row .status-chip .name').first()).toBeVisible();
  // More statuses than there is room to name: the chips keep their keys and counts and drop their names.
  for (const [i, key] of ['1', '2', '3', '4'].entries()) {
    await pack.createRef('statuses', { name: `A rather long status name ${i} ${Math.random().toString(36).slice(2, 6)}`, color: '#59a14f', shortcut: key });
  }
  await page.reload();
  await expect(page.locator('.legend-row .status-chip')).toHaveCount(pack.ref.statuses.length + 4);
  await oneLine();
  await expect(page.locator('.legend-row')).toHaveClass(/no-names/);
  await expect(page.locator('.legend-row .status-chip .key-cap').last()).toBeVisible();
  // A narrower desktop window keeps to one line too.
  await page.setViewportSize({ width: 1200, height: 800 });
  await oneLine();
});
