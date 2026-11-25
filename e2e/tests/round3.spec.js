// A list, continued (static/js/list/boxes.js, pages/list.js, css/list.css):
// the statuses of what is inside a container, drag and drop, the details switch,
// boxes sized to their contents and framed by level, Ctrl+F, and printing.
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test, expect, openList, row, box, compartment, rowNames, selectedName, listMenu, saved } from './helpers.js';

// Reading a printed page back needs pdftotext (Debian: poppler-utils); the test that does is skipped without it.
const hasPdfToText = spawnSync('pdftotext', ['-v']).error === undefined;
/** The text of each page of a PDF. */
function pdfPages(pdf) {
  const file = join(mkdtempSync(join(tmpdir(), 'packrat-pdf-')), 'list.pdf');
  writeFileSync(file, pdf);
  const text = execFileSync('pdftotext', ['-layout', file, '-'], { encoding: 'utf8' });
  const pages = text.split('\f');
  if (pages[pages.length - 1].trim() === '') pages.pop();
  return pages;
}

const TRIP = [
  ['Car', null, { status: 'Packed' }],
  ['Blue Backpack', 'Car', { status: 'Packed' }],
  ['Hygiene Kit', 'Blue Backpack', { status: 'Packed' }],
  ['Comb', 'Hygiene Kit', { status: 'Packed' }],
  ['Toothbrush', 'Hygiene Kit', { status: 'Packed' }],
  ['Socks', 'Blue Backpack', { quantity: 3, unit: 'pairs', status: 'Packed' }],
  ['Camp Bin', 'Car', { status: 'Packed' }],
  ['Headlamp', 'Camp Bin', { status: 'Packed' }],
  ['Sun Hat', null, { status: 'Packed' }],
];
/** The chips on a container's heading, as "count name" in the order shown. */
const chips = (page, name) => row(page, name).locator('.inside-chip').evaluateAll((els) => els.map((e) => `${e.querySelector('.inside-count').textContent} ${e.querySelector('.inside-name').textContent}`));

test('a container shows a chip for each status among the entries directly inside it', async ({ page, pack }) => {
  const list = await pack.list('Chips', TRIP);
  const color = Object.fromEntries(pack.ref.statuses.map((s) => [s.name, s.color]));
  const rgb = (hex) => { const n = parseInt(hex.slice(1), 16); return `rgb(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255})`; };
  await openList(page, list);
  // Everything packed: one chip each, counting what is directly inside (the car holds two containers; the
  // seven entries inside those are theirs to report).
  expect(await chips(page, 'Car')).toEqual(['2 Packed']);
  expect(await chips(page, 'Blue Backpack')).toEqual(['2 Packed']);
  expect(await chips(page, 'Hygiene Kit')).toEqual(['2 Packed']);
  expect(await chips(page, 'Camp Bin')).toEqual(['1 Packed']);
  await expect(row(page, 'Car').locator('.inside-chip')).toHaveCSS('background-color', rgb(color.Packed));
  await expect(row(page, 'Car').locator('.inside')).toHaveAttribute('title', /Directly inside Car: 2 Packed\..*5 entries inside the containers in it/);
  // Something that is not a container has nothing to report.
  await expect(row(page, 'Comb').locator('.inside')).toHaveCount(0);

  // One thing in the kit goes back to storage: the kit shows both statuses, the worse first. The backpack
  // and the car, which it is not directly inside, show no change.
  await row(page, 'Comb').locator('.item-name').click();
  let done = saved(page);
  await page.keyboard.press('s');
  await done;
  expect(await chips(page, 'Hygiene Kit')).toEqual(['1 In Storage Location', '1 Packed']);
  expect(await chips(page, 'Blue Backpack')).toEqual(['2 Packed']);
  expect(await chips(page, 'Car')).toEqual(['2 Packed']);
  const kit = row(page, 'Hygiene Kit').locator('.inside-chip');
  await expect(kit.nth(0)).toHaveCSS('background-color', rgb(color['In Storage Location']));
  await expect(kit.nth(1)).toHaveCSS('background-color', rgb(color.Packed));
  // The other entry in the kit goes missing: three statuses would be three chips; here two entries, two chips.
  await row(page, 'Toothbrush').locator('.item-name').click();
  done = saved(page);
  await page.keyboard.press('m');
  await done;
  expect(await chips(page, 'Hygiene Kit')).toEqual(['1 Missing', '1 In Storage Location']);
  // A container's own status is one of its container's chips, and not one of its own.
  await row(page, 'Hygiene Kit').locator('.item-name').click();
  done = saved(page);
  await page.keyboard.press('m');
  await done;
  expect(await chips(page, 'Hygiene Kit')).toEqual(['1 Missing', '1 In Storage Location']);
  expect(await chips(page, 'Blue Backpack')).toEqual(['1 Missing', '1 Packed']); // the kit, and the socks
  expect(await chips(page, 'Car')).toEqual(['2 Packed']);
  // The containers' own statuses have not moved because of what is inside them: on the row and on the server.
  for (const name of ['Car', 'Blue Backpack', 'Camp Bin']) await expect(row(page, name).locator('.status-chip')).toHaveText('Packed');
  const now = await pack.fetch(list.id);
  expect(['Car', 'Blue Backpack', 'Camp Bin'].map((n) => now.entry(n).status)).toEqual(['Packed', 'Packed', 'Packed']);
  // Where a heading has plenty of room (the outermost box here), the statuses are named on the chips;
  // in a narrower one the counts stand alone, on their colours.
  await page.setViewportSize({ width: 1900, height: 900 });
  await expect(row(page, 'Car').locator('.inside-name').first()).toBeVisible();
  await expect(row(page, 'Hygiene Kit').locator('.inside-name').first()).toBeHidden();
  await expect(row(page, 'Hygiene Kit').locator('.inside-count').first()).toBeVisible();
});

test('the statuses are ranked in Settings, and the rank is the order they are shown in', async ({ page, pack }) => {
  const list = await pack.list('Rank', [['Camp Bin', null], ['Headlamp', 'Camp Bin', { status: 'Missing' }], ['Mess Kit', 'Camp Bin', { status: 'Packed' }]]);
  const names = () => page.locator('.ref-table .row[data-name]').evaluateAll((els) => els.map((e) => e.dataset.name));
  const order = async () => (await (await page.request.get('/api/ref/statuses')).json()).sort((a, b) => a.rank - b.rank).map((s) => s.name);
  const before = await order();
  expect(before).toEqual(['Missing', 'In Storage Location', 'Packed']);
  try {
    await page.goto('/#/settings');
    await expect.poll(names).toEqual(before); // the worst at the top
    const r = (name) => page.locator(`.ref-table .row[data-name="${name}"]`);
    await expect(r('Missing').locator('.rank-up')).toBeDisabled();
    await expect(r('Packed').locator('.rank-down')).toBeDisabled();
    // Packed is moved up to be the worst of all.
    let done = page.waitForResponse((x) => x.url().endsWith('/api/statuses/order'));
    await r('Packed').locator('.rank-up').click();
    await done;
    await expect.poll(names).toEqual(['Missing', 'Packed', 'In Storage Location']);
    await expect(r('Packed').locator('.rank-up')).toBeFocused(); // so it can be pressed again
    done = page.waitForResponse((x) => x.url().endsWith('/api/statuses/order'));
    await page.keyboard.press('Enter');
    await done;
    await expect.poll(names).toEqual(['Packed', 'Missing', 'In Storage Location']);
    expect(await order()).toEqual(['Packed', 'Missing', 'In Storage Location']);
    // The default status is still the one made first, wherever it is ranked.
    await expect(r('In Storage Location').locator('.chip')).toHaveText('default');

    // On the list, the bin's chips and the legend follow the new order.
    await openList(page, list);
    expect(await chips(page, 'Camp Bin')).toEqual(['1 Packed', '1 Missing']);
    await expect(page.locator('.legend-row .status-chip .name')).toHaveText(['Packed', 'Missing', 'In Storage Location']);
  } finally {
    await page.request.post('/api/statuses/order', { data: { ids: before.map((n) => pack.statusId(n)) } });
  }
  expect(await order()).toEqual(before);
});

test('entries are dragged between containers, into compartments and out to the top level', async ({ page, pack }) => {
  const list = await pack.list('Drag', [
    ['Car', null], ['Blue Backpack', 'Car'], ['Comb', 'Blue Backpack'], ['Socks', 'Blue Backpack'],
    ['Camp Bin', 'Car'], ['Socks', 'Camp Bin'], ['Headlamp', 'Camp Bin'],
    ['Daypack', null], ['Camera', 'Daypack', { compartment: 'Main' }], ['Sun Hat', null],
  ]);
  await openList(page, list);
  const drag = async (source, target, method = 'PATCH') => {
    const done = method ? saved(page, method) : null;
    await source.dragTo(target);
    if (done) await done;
  };
  const name = (n, within) => (within ? within.locator(`.row.entry[data-name="${n}"]`) : row(page, n)).locator('.item-name');

  // An item onto another container's box (its heading will do).
  await drag(name('Comb'), row(page, 'Camp Bin').locator('.item-name'));
  expect((await pack.fetch(list.id)).entry('Comb')).toMatchObject({ container: 'Camp Bin', compartment: null });
  expect(await selectedName(page)).toBe('Comb');
  await expect(box(page, 'Camp Bin').locator('.row.entry[data-name="Comb"]')).toBeVisible();
  // Onto a compartment: into that compartment.
  await drag(name('Comb'), compartment(page, 'Daypack', 'Front Pocket').locator('.pk-comp-name'));
  expect((await pack.fetch(list.id)).entry('Comb')).toMatchObject({ container: 'Daypack', compartment: 'Front Pocket' });
  // From one compartment to another of the same container, and onto the container itself to leave the compartment.
  await drag(name('Comb'), compartment(page, 'Daypack', 'Side Pocket').locator('.pk-comp-name'));
  expect((await pack.fetch(list.id)).entry('Comb').compartment).toBe('Side Pocket');
  await drag(name('Comb'), row(page, 'Daypack').locator('.item-name'));
  expect((await pack.fetch(list.id)).entry('Comb')).toMatchObject({ container: 'Daypack', compartment: null });
  // A container is dragged by its heading and takes what is in it along.
  await drag(name('Blue Backpack'), compartment(page, 'Daypack', 'Main').locator('.pk-comp-name'));
  let now = await pack.fetch(list.id);
  expect(now.entry('Blue Backpack')).toMatchObject({ container: 'Daypack', compartment: 'Main' });
  expect(now.entry('Socks', 'Blue Backpack')).toBeTruthy();
  // Out to the top level: onto what is in no container.
  await drag(name('Blue Backpack'), row(page, 'Sun Hat').locator('.item-name'));
  expect((await pack.fetch(list.id)).entry('Blue Backpack')).toMatchObject({ container: null, compartment: null });

  // Places that do not take the drop: where the same item already is; the entry's own place; inside itself.
  const before = JSON.stringify((await pack.fetch(list.id)).contents);
  await drag(name('Socks', box(page, 'Blue Backpack')), row(page, 'Camp Bin').locator('.item-name'), null); // the bin has socks
  await drag(name('Headlamp'), row(page, 'Camp Bin').locator('.item-name'), null); // it is there already
  await drag(name('Car'), row(page, 'Camp Bin').locator('.item-name'), null); // the bin is inside the car
  await page.waitForTimeout(200);
  expect(JSON.stringify((await pack.fetch(list.id)).contents)).toBe(before);
  // The place a drop would go is outlined while the entry is over it, and not when it cannot go there.
  await name('Headlamp').hover();
  await page.mouse.down();
  const to = await row(page, 'Daypack').locator('.item-name').boundingBox();
  await page.mouse.move(to.x + 5, to.y + 5, { steps: 8 });
  await expect(box(page, 'Daypack')).toHaveClass(/drop-target/);
  await expect(page.locator('.pk-top-drop')).toBeVisible();
  const own = await row(page, 'Camp Bin').locator('.item-name').boundingBox();
  await page.mouse.move(own.x + 5, own.y + 5, { steps: 8 });
  await expect(page.locator('.drop-target')).toHaveCount(0);
  // Dropped on the strip that appears for it: the top level.
  const strip = await page.locator('.pk-top-drop').boundingBox();
  await page.mouse.move(strip.x + strip.width / 2, strip.y + strip.height / 2, { steps: 8 });
  await expect(page.locator('.pack')).toHaveClass(/drop-target/);
  const done = saved(page);
  await page.mouse.up();
  await done;
  expect((await pack.fetch(list.id)).entry('Headlamp').container).toBeNull();
  await expect(page.locator('.pk-top-drop')).toBeHidden();
  await expect(page.locator('.drop-target, .dragging')).toHaveCount(0);
  // Text in a row's boxes is still selected with the mouse, not dragged.
  await row(page, 'Sun Hat').locator('.qty-input').click();
  await expect(row(page, 'Sun Hat').locator('.qty-input')).toBeFocused();
  expect(await row(page, 'Sun Hat').evaluate((e) => e.draggable)).toBe(false);
});

test('the category and storage location can be switched off', async ({ page, pack }) => {
  const list = await pack.list('Meta', [['Camp Bin', null], ['Headlamp', 'Camp Bin', { notes: 'spare batteries' }], ['Mess Kit', 'Camp Bin'], ['Sun Hat', null]]);
  await openList(page, list);
  await expect(row(page, 'Headlamp').locator('.category')).toBeVisible();
  await expect(row(page, 'Headlamp').locator('.location')).toBeVisible();
  await listMenu(page, 'Hide category and storage location');
  for (const name of ['Camp Bin', 'Headlamp', 'Sun Hat']) {
    await expect(row(page, name).locator('.category')).toBeHidden();
    await expect(row(page, name).locator('.location')).toBeHidden();
  }
  // The other fields are all still there.
  await expect(row(page, 'Headlamp').locator('.qty-input')).toBeVisible();
  await expect(row(page, 'Headlamp').locator('.status-chip')).toBeVisible();
  await expect(row(page, 'Headlamp').locator('.notes-input')).toHaveValue('spare batteries');
  // It is remembered, and the menu offers the way back.
  await page.reload();
  await expect(row(page, 'Headlamp')).toBeVisible();
  await expect(row(page, 'Headlamp').locator('.category')).toBeHidden();
  // In a narrow box an entry is then a single line, unless it has notes (or is selected, to write some).
  await page.setViewportSize({ width: 520, height: 900 });
  const h = async (name) => (await row(page, name).boundingBox()).height;
  expect(await h('Mess Kit')).toBeLessThan((await h('Headlamp')) - 10);
  await expect(row(page, 'Mess Kit').locator('.notes-input')).toBeHidden();
  await row(page, 'Mess Kit').locator('.item-name').click();
  await expect(row(page, 'Mess Kit').locator('.notes-input')).toBeVisible();
  await page.setViewportSize({ width: 1500, height: 900 });
  await listMenu(page, 'Show category and storage location');
  await expect(row(page, 'Headlamp').locator('.category')).toHaveText('Camp');
  await expect(row(page, 'Headlamp').locator('.location')).toBeVisible();
});

test('boxes are as tall as their contents, and each level of containers is framed more lightly', async ({ page, pack }) => {
  const list = await pack.list('Shape', [
    ['Car', null],
    ['Blue Backpack', 'Car'], ['Hygiene Kit', 'Blue Backpack'], ['Comb', 'Hygiene Kit'], ['Toothbrush', 'Hygiene Kit'], ['Socks', 'Blue Backpack'], ['T-Shirt', 'Blue Backpack'],
    ['Camp Bin', 'Car'], ['Headlamp', 'Camp Bin'],
    ['Daypack', null], ['Camera', 'Daypack'],
  ]);
  await openList(page, list);
  const rect = (name) => box(page, name).boundingBox();
  // Side by side in the car: the backpack holds much more than the bin, and the bin does not stretch to match.
  const bag = await rect('Blue Backpack'), bin = await rect('Camp Bin');
  expect(Math.abs(bag.y - bin.y)).toBeLessThan(1);
  expect(bin.x).toBeGreaterThanOrEqual(bag.x + bag.width);
  expect(bin.height).toBeLessThan(bag.height - 60);
  // The bin's box ends just under its one entry.
  const lamp = await row(page, 'Headlamp').boundingBox();
  expect(bin.y + bin.height - (lamp.y + lamp.height)).toBeLessThan(24);
  // Levels: the box says how deep it is, and each level is drawn differently from the one around it.
  await expect(box(page, 'Car')).toHaveAttribute('data-level', '0');
  await expect(box(page, 'Blue Backpack')).toHaveAttribute('data-level', '1');
  await expect(box(page, 'Hygiene Kit')).toHaveAttribute('data-level', '2');
  const look = (name) => box(page, name).evaluate((e) => {
    const s = getComputedStyle(e), h = getComputedStyle(e.querySelector(':scope > .row.entry.container')), n = getComputedStyle(e.querySelector(':scope > .row.entry .item-name'));
    return { frame: parseFloat(s.borderTopWidth), spine: parseFloat(s.borderLeftWidth), body: s.backgroundColor, band: h.backgroundColor, size: parseFloat(n.fontSize), shadow: s.boxShadow };
  });
  const [l0, l1, l2] = [await look('Car'), await look('Blue Backpack'), await look('Hygiene Kit')];
  expect(l0.frame).toBeGreaterThan(l1.frame);
  expect(l1.frame).toBeGreaterThan(l2.frame);
  expect(l0.size).toBeGreaterThan(l1.size);
  expect(l1.size).toBeGreaterThan(l2.size);
  expect(l0.shadow).not.toBe('none');
  expect(l1.shadow).toBe('none');
  expect(l1.spine).toBeGreaterThan(l1.frame); // a container inside another carries a spine
  expect(l2.spine).toBeGreaterThan(l2.frame);
  expect(new Set([l0.band, l1.band, l2.band]).size).toBe(3);
  expect(l0.body).not.toBe(l1.body);
  expect(l1.body).not.toBe(l2.body);
});

test('Ctrl+F goes to the filter box, on a list and in the Library', async ({ page, pack }) => {
  const list = await pack.list('Find', [['Camp Bin', null], ['Headlamp', 'Camp Bin'], ['Mess Kit', 'Camp Bin']]);
  await openList(page, list);
  await row(page, 'Headlamp').locator('.item-name').click();
  await page.keyboard.press('Control+f');
  await expect(page.locator('.filter-box')).toBeFocused();
  await page.keyboard.type('mess');
  expect(await rowNames(page)).toEqual(['Camp Bin', 'Mess Kit']);
  // Typed again over what is there (the text is selected); Escape hands the keys back to the list.
  await page.keyboard.press('Escape');
  await page.keyboard.press('Control+f');
  await page.keyboard.type('lamp');
  await expect(page.locator('.filter-box')).toHaveValue('lamp');
  expect(await rowNames(page)).toEqual(['Camp Bin', 'Headlamp']);
  // With the cursor already there, the key is the browser's own again.
  const second = await page.evaluate(() => new Promise((resolve) => {
    document.addEventListener('keydown', (e) => resolve(e.defaultPrevented), { once: true });
    document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true, cancelable: true }));
  }));
  expect(second).toBe(false);

  await page.goto('/#/library');
  await expect(page.locator('.ref-table .row[data-name="Comb"]')).toBeVisible();
  await page.keyboard.press('Control+f');
  await expect(page.locator('.filter-box')).toBeFocused();
  await page.keyboard.type('bathroom');
  await expect(page.locator('.ref-table .row[data-name="Toothbrush"]')).toBeVisible();
  await expect(page.locator('.ref-table .row[data-name="Blue Backpack"]')).toBeHidden();
  // A page without a filter box leaves the key alone.
  await page.goto('/#/settings');
  await expect(page.locator('.ref-table')).toBeVisible();
  const settings = await page.evaluate(() => new Promise((resolve) => {
    document.addEventListener('keydown', (e) => resolve(e.defaultPrevented), { once: true });
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true, cancelable: true }));
  }));
  expect(settings).toBe(false);
});

test('printing: what is on screen, with a box to tick in place of each status', async ({ page, pack }) => {
  const long = 'Charge it the night before and pack the spare battery, the card reader and both of the lens cloths beside it';
  const list = await pack.list('Print', [
    ['Daypack', null, { status: 'Packed' }], ['Camera', 'Daypack', { compartment: 'Main', status: 'Missing', notes: long }],
    ['Passport', 'Daypack', { compartment: 'Front Pocket' }], ['Hygiene Kit', 'Daypack'], ['Comb', 'Hygiene Kit'],
    ['Socks', 'Daypack', { quantity: 3, unit: 'pairs' }], ['Sun Hat', null, { quantity: 2 }],
  ]);
  await page.emulateMedia({ colorScheme: 'dark' });
  await openList(page, list);
  // Closed on screen, closed on paper.
  await row(page, 'Hygiene Kit').locator('.caret').click();
  // The ⋯ menu prints.
  await page.evaluate(() => { window.printed = 0; window.print = () => { window.printed++; }; });
  await listMenu(page, 'Print');
  expect(await page.evaluate(() => window.printed)).toBe(1);
  // On screen there are chips and text boxes, and none of what is only for paper.
  await expect(row(page, 'Camera').locator('.status-chip')).toBeVisible();
  await expect(row(page, 'Camera').locator('.print-check')).toBeHidden();
  await expect(row(page, 'Camera').locator('.print-notes')).toBeHidden();
  await expect(page.locator('.print-title')).toBeHidden();
  const onScreen = (await row(page, 'Passport').boundingBox()).height;

  await page.setViewportSize({ width: 720, height: 1000 }); // about the width of a page
  await page.emulateMedia({ media: 'print', colorScheme: 'dark' });
  // The title bar and every control are gone; the list's name heads the page.
  await expect(page.locator('.topbar')).toBeHidden();
  await expect(page.locator('.print-title h1')).toHaveText(list.name);
  await expect(page.locator('.print-title')).toContainText('7 entries');
  await expect(page.locator('.pack .actions button').first()).toBeHidden();
  await expect(page.locator('.pack .caret').first()).toBeHidden();
  await expect(page.locator('.add-to-compartment').first()).toBeHidden();
  await expect(page.locator('.pack input:visible, .pack select:visible')).toHaveCount(0);
  // The same entries in the same boxes, with their fields, as text.
  expect(await rowNames(page)).toEqual(['Daypack', 'Camera', 'Passport', 'Hygiene Kit', 'Socks', 'Sun Hat']);
  await expect(compartment(page, 'Daypack', 'Main').locator('.pk-comp-name')).toBeVisible();
  await expect(compartment(page, 'Daypack', 'Main').locator('.row.entry[data-name="Camera"]')).toBeVisible();
  await expect(row(page, 'Camera').locator('.category')).toBeVisible();
  await expect(row(page, 'Camera').locator('.location')).toBeVisible();
  await expect(row(page, 'Sun Hat').locator('.print-qty')).toHaveText('2');
  await expect(row(page, 'Socks').locator('.print-qty')).toHaveText('3 pairs');
  await expect(row(page, 'Comb')).toHaveCount(0);
  await expect(row(page, 'Hygiene Kit').locator('.inside-total')).toBeVisible(); // how many are inside the closed kit
  await expect(row(page, 'Hygiene Kit').locator('.inside-total')).toHaveText('1');
  await expect(row(page, 'Daypack').locator('.inside-chip').first()).toBeHidden(); // the statuses go with the colours
  // A long note is all there: it wraps over as many lines as it needs, inside its row, and nothing clips it.
  const note = row(page, 'Camera').locator('.print-notes');
  await expect(note).toHaveText(long);
  const fits = await note.evaluate((e) => {
    const row = e.closest('.row.entry').getBoundingClientRect();
    const rects = [...e.getClientRects()];
    const clipped = (() => { for (let p = e.parentElement; p && !p.classList.contains('pack'); p = p.parentElement) { const s = getComputedStyle(p); if (s.overflow !== 'visible' && (p.scrollWidth > p.clientWidth + 1 || p.scrollHeight > p.clientHeight + 1)) return p.className; } return ''; })();
    return { lines: rects.length, inside: rects.every((r) => r.left >= row.left - 1 && r.right <= row.right + 1 && r.bottom <= row.bottom + 1), clipped, ws: getComputedStyle(e.parentElement).whiteSpace };
  });
  expect(fits.lines).toBeGreaterThan(1);
  expect(fits.inside).toBe(true);
  expect(fits.clipped).toBe('');
  // An entry without notes has no line for them; names are whole too (no ellipsis).
  await expect(row(page, 'Passport').locator('.notes')).toBeHidden();
  await expect(row(page, 'Passport').locator('.item-name')).toHaveCSS('white-space', 'normal');
  // Every entry, containers too, has an empty box to tick where its status chip was, and no chip.
  for (const name of ['Daypack', 'Camera', 'Passport', 'Hygiene Kit', 'Socks', 'Sun Hat']) {
    await expect(row(page, name).locator('.status-chip')).toBeHidden();
    const check = row(page, name).locator('.print-check');
    await expect(check).toBeVisible();
    const b = await check.boundingBox();
    expect(Math.abs(b.width - b.height)).toBeLessThan(1);
    expect(b.width).toBeGreaterThan(7);
    await expect(check).toHaveCSS('border-top-style', 'solid');
    await expect(check).toHaveText('');
  }
  // On white, whatever the screen's theme, and without the statuses' colours.
  await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(row(page, 'Camera')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
  // Set close: an entry takes less than half the height it has on screen.
  expect((await row(page, 'Passport').boundingBox()).height).toBeLessThan(onScreen / 2);
  // And the filter applies to paper as to the screen.
  await page.emulateMedia({ media: 'screen' });
  await page.locator('.filter-box').fill('passport');
  await page.emulateMedia({ media: 'print' });
  expect(await rowNames(page)).toEqual(['Daypack', 'Passport']);
  await expect(page.locator('.print-title')).toContainText('1 of 7 entries (filtered)');
});

test('printing: a long list is dense, and each top-level container is whole on one page', async ({ page, pack }) => {
  test.skip(!hasPdfToText, 'pdftotext (poppler-utils) is not installed');
  // Three top-level containers of 36 entries each, every entry marked in its notes with its container and number.
  const loose = pack.ref.items.filter((i) => !i.is_container).map((i) => i.name).slice(0, 36);
  const tops = ['Car', 'Suitcase, Large', 'Daypack'];
  const list = await pack.list('Pages', tops.flatMap((top, t) => [[top, null], ...loose.map((n, i) => [n, top, { notes: `mark-${t}-${i}` }])]));
  await openList(page, list);
  const pdf = await page.pdf({ format: 'A4', preferCSSPageSize: true });
  expect(pdf.subarray(0, 5).toString()).toBe('%PDF-');
  const pages = pdfPages(pdf);
  // Dense: 111 entries with their notes, in three pages at the most (as they were set before, they took eight).
  expect(pages.length).toBeGreaterThan(1);
  expect(pages.length).toBeLessThanOrEqual(3);
  // Whole: all the marks of one container are on one page.
  for (const t of [0, 1, 2]) {
    const on = new Set();
    for (let i = 0; i < loose.length; i++) {
      const at = pages.findIndex((text) => new RegExp(`mark-${t}-${i}(?!\\d)`).test(text));
      expect(at, `mark-${t}-${i} is printed`).toBeGreaterThanOrEqual(0);
      on.add(at);
    }
    expect([...on], `the entries of ${tops[t]} are on one page`).toHaveLength(1);
  }
});
