// The Sort menu of a list (static/js/list/sort.js): several columns, in
// order, each with its direction, as in the planner.
import { test, expect, openList, rowNames } from './helpers.js';

const BIN = [
  ['Camp Bin', null],
  ['Cooler', 'Camp Bin'],
  ['Snacks', 'Cooler'],
  ['Headlamp', 'Camp Bin', { quantity: 5, status: 'Packed' }],
  ['Mess Kit', 'Camp Bin', { quantity: 1, status: 'Missing' }],
  ['Camp Stove', 'Camp Bin', { quantity: 3 }],
  ['Fuel Canister', 'Camp Bin', { quantity: 2, status: 'Packed', notes: 'full' }],
];
/** The items packed straight into the bin, in the order shown. */
const items = async (page) => (await rowNames(page)).filter((n) => !['Camp Bin', 'Cooler', 'Snacks'].includes(n));
const sortKeys = (page) => page.locator('.sort-item').evaluateAll((els) => els.map((e) => `${e.dataset.sortKey} ${e.querySelector('.sort-dir').textContent}`));

test('entries are sorted by several columns in order; directions flip; columns come and go', async ({ page, pack }) => {
  const list = await pack.list('Sort', BIN);
  await openList(page, list);
  // The plain order is by name, the containers first.
  expect(await rowNames(page)).toEqual(['Camp Bin', 'Cooler', 'Snacks', 'Camp Stove', 'Fuel Canister', 'Headlamp', 'Mess Kit']);
  await expect(page.locator('#sort-badge')).toBeHidden();

  await page.locator('#btn-sort').click();
  await expect(page.locator('.sort-panel')).toContainText('Sorted by name');
  await expect(page.locator('.sort-add option')).toHaveText(['+ sort by…', 'Name', 'Quantity', 'Category', 'Stored in', 'Status', 'Notes']);
  // By status (by rank, the worst first): Missing, In Storage Location, Packed; ties by name.
  await page.locator('.sort-add').selectOption('status');
  expect(await items(page)).toEqual(['Mess Kit', 'Camp Stove', 'Fuel Canister', 'Headlamp']);
  await expect(page.locator('#sort-badge')).toHaveText('1');
  // ...then by quantity, the most first: the two packed ones swap. The menu stays open to add to.
  await expect(page.locator('.sort-add option').first()).toHaveText('+ then by…');
  await page.locator('.sort-add').selectOption('quantity');
  expect(await items(page)).toEqual(['Mess Kit', 'Camp Stove', 'Headlamp', 'Fuel Canister']);
  expect(await sortKeys(page)).toEqual(['status ↑', 'quantity ↓']);
  await expect(page.locator('#sort-badge')).toHaveText('2');
  await expect(page.locator('.sort-add option')).toHaveText(['+ then by…', 'Name', 'Category', 'Stored in', 'Notes']); // what is in use is not offered again
  // A column's direction flips on its own.
  await page.locator('.sort-item[data-sort-key="status"] .sort-dir').click();
  expect(await items(page)).toEqual(['Headlamp', 'Fuel Canister', 'Camp Stove', 'Mess Kit']);
  expect(await sortKeys(page)).toEqual(['status ↓', 'quantity ↓']);
  // The containers stay ahead of the items, in their box, whatever the sort.
  expect((await rowNames(page)).slice(0, 3)).toEqual(['Camp Bin', 'Cooler', 'Snacks']);
  // Removing the first column leaves the second in charge.
  await page.locator('.sort-item[data-sort-key="status"] .sort-remove').click();
  expect(await items(page)).toEqual(['Headlamp', 'Camp Stove', 'Fuel Canister', 'Mess Kit']);
  expect(await sortKeys(page)).toEqual(['quantity ↓']);
  await page.keyboard.press('Escape');
  await expect(page.locator('.sort-panel')).toHaveCount(0);

  // The sort is remembered.
  await page.reload();
  await expect(page.locator('#sort-badge')).toHaveText('1');
  expect(await items(page)).toEqual(['Headlamp', 'Camp Stove', 'Fuel Canister', 'Mess Kit']);
  await expect(page.locator('#btn-sort')).toHaveAttribute('title', /Quantity ↓/);

  // Entries without a value sort last in both directions (notes: only the fuel has any).
  await page.locator('#btn-sort').click();
  await page.locator('.sort-item .sort-remove').click();
  await page.locator('.sort-add').selectOption('notes');
  expect((await items(page))[0]).toBe('Fuel Canister');
  await page.locator('.sort-item .sort-dir').click();
  expect((await items(page))[0]).toBe('Fuel Canister');
  // And back to the plain order in one step.
  await page.locator('.sort-clear').click();
  expect(await items(page)).toEqual(['Camp Stove', 'Fuel Canister', 'Headlamp', 'Mess Kit']);
  await expect(page.locator('#sort-badge')).toBeHidden();
  await page.keyboard.press('Escape');

  // Sorting lives in the Sort menu only: the ⋯ menu no longer has it.
  await page.locator('#btn-list-menu').click();
  await expect(page.locator('.menu button', { hasText: 'Rename…' })).toBeVisible();
  await expect(page.locator('.menu button', { hasText: /sort/i })).toHaveCount(0);
});
