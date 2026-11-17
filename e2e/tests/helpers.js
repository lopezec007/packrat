// Shared by the specs: making lists through the API, finding things on the
// page, and cleaning up afterwards. Every test builds the lists it works on
// (with a random suffix in the name) from the demo library, so the tests do
// not depend on one another or on the demo lists staying as they are.
import { test as base, expect } from '@playwright/test';

export { expect };
export const uid = () => Math.random().toString(36).slice(2, 8);

/**
 * `test` with two additions: page errors fail the test, and `pack` (below)
 * removes what it made when the test ends.
 */
export const test = base.extend({
  page: async ({ page }, use) => {
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await use(page);
    expect(errors, 'uncaught errors on the page').toEqual([]);
  },
  pack: async ({ request }, use) => {
    const made = { lists: [], rows: [] }; // rows: [table, id] of library/settings rows
    const ref = await (await request.get('/api/ref')).json();
    const itemId = (name) => {
      const it = ref.items.find((i) => i.name === name);
      if (!it) throw new Error(`no demo item named ${name}`);
      return it.id;
    };
    const statusId = (name) => {
      const s = ref.statuses.find((x) => x.name === name);
      if (!s) throw new Error(`no status named ${name}`);
      return s.id;
    };
    const api = {
      ref, itemId, statusId,
      /**
       * Make a list. entries: [[item, container | null, {quantity, unit, status, notes}], ...],
       * containers before what goes inside them. Returns {id, name, row(item, container)}.
       */
      async list(prefix, entries = [], { template = false } = {}) {
        const name = `${prefix} ${uid()}`;
        const res = await request.post('/api/lists', { data: { name, is_template: template } });
        expect(res.status(), await res.text()).toBe(201);
        const list = await res.json();
        made.lists.push(list.id);
        const rows = [];
        for (const [item, container, opts = {}] of entries) {
          const r = await request.post(`/api/lists/${list.id}/contents`, { data: {
            item_id: itemId(item), container_id: container ? itemId(container) : null,
            quantity: opts.quantity ?? 1, unit: opts.unit ?? null, notes: opts.notes ?? null,
            status_id: opts.status ? statusId(opts.status) : undefined,
          } });
          expect(r.status(), `${item}: ${await r.text()}`).toBe(201);
          rows.push({ item, container: container ?? null, ...(await r.json()) });
        }
        return { id: list.id, name, rows };
      },
      /** The list as the server has it now, with item names resolved. */
      async fetch(listId) {
        const fresh = await (await request.get('/api/ref')).json();
        const names = new Map(fresh.items.map((i) => [i.id, i.name]));
        const statuses = new Map(fresh.statuses.map((s) => [s.id, s.name]));
        const d = await (await request.get(`/api/lists/${listId}`)).json();
        d.entries = d.contents.map((c) => ({ ...c, item: names.get(c.item_id), container: c.container_id == null ? null : names.get(c.container_id), status: statuses.get(c.status_id) }));
        d.entry = (item, container) => d.entries.find((e) => e.item === item && (container === undefined || e.container === container));
        return d;
      },
      /** Remember a list made through the UI, so it is deleted afterwards. */
      async trackListByName(name) {
        const lists = await (await request.get('/api/lists')).json();
        const l = lists.find((x) => x.name === name);
        if (l) made.lists.push(l.id);
        return l;
      },
      /** Remember a library or settings row made through the UI: track('items', 'Kite'). */
      async track(table, name) {
        const rows = await (await request.get(`/api/ref/${table}`)).json();
        const r = rows.find((x) => x.name === name);
        if (r) made.rows.push([table, r.id]);
        return r;
      },
      async createRef(table, data) {
        const res = await request.post(`/api/ref/${table}`, { data });
        expect(res.status(), await res.text()).toBe(201);
        const row = await res.json();
        made.rows.push([table, row.id]);
        return row;
      },
    };
    await use(api);
    for (const id of made.lists) await request.delete(`/api/lists/${id}`);
    // Items before the categories and locations they use.
    const order = { items: 0, statuses: 1, item_categories: 2, storage_locations: 2 };
    for (const [table, id] of made.rows.sort((a, b) => order[a[0]] - order[b[0]])) await request.delete(`/api/ref/${table}/${id}`);
  },
});

/** Open a list and wait for it to be drawn. */
export async function openList(page, list) {
  await page.goto(`/#/list?id=${list.id}`);
  await expect(page.locator('.list-switch')).toHaveValue(String(list.id));
  await expect(page.locator('.pack > *').first()).toBeVisible();
}

/** An entry's row by item name: a container's heading, or an item in a list (the first one, when the item is on the list more than once). */
export const row = (page, name) => page.locator(`.pack .row.entry[data-name="${name}"]`).first();
/** A container's box (its heading and everything inside it) by item name. */
export const box = (page, name) => row(page, name).locator('xpath=..');
/** The names of the rows in view, in reading order. */
export const rowNames = (page) => page.locator('.pack .row.entry:not(.new)').evaluateAll((els) => els.map((e) => e.dataset.name));
/** The name of the selected entry ('' when nothing is selected). */
export const selectedName = (page) => page.evaluate(() => document.querySelector('.pack .row.selected')?.dataset.name ?? '');
/** Choose an entry of the list's ⋯ menu. */
export async function listMenu(page, label) {
  await page.locator('#btn-list-menu').click();
  await page.locator('.menu button', { hasText: label }).click();
}
export const toast = (page) => page.locator('#toast');
/** Resolves when the next change to a list entry has been answered. */
export const saved = (page, method = 'PATCH') => page.waitForResponse((r) => /\/api\/(contents|lists)\//.test(r.url()) && r.request().method() === method);
