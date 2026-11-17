// Reference data shared by every page: the library (items, their categories
// and storage locations) and the statuses, loaded once from GET /api/ref and
// kept current as pages create, change and delete rows.
//
//   ref.items                   -> array of rows (as the API returns them)
//   ref.by.items.get(id)        -> one row
//   await saveRef('items', null, {name: 'Comb', ...})   -> create
//   await saveRef('statuses', 3, {color: '#d62828'})    -> update
//   await removeRef('item_categories', 4)

import { api } from './api.js';
import { byText } from './util.js';

export const REF_TABLES = ['item_categories', 'storage_locations', 'statuses', 'items'];

export const ref = { by: {} };
for (const t of REF_TABLES) { ref[t] = []; ref.by[t] = new Map(); }

/** Server facts from GET /api/version: {version, date, build}. */
export const app = { version: '', date: '', build: '' };

function index(table) {
  ref.by[table] = new Map(ref[table].map((r) => [r.id, r]));
}

/** (Re)load every reference table and the server facts. */
export async function loadRef() {
  const [all, version] = await Promise.all([api.ref(), api.version()]);
  for (const t of REF_TABLES) { ref[t] = all[t] || []; index(t); }
  Object.assign(app, version);
}

/** Reload the reference tables only (their computed counts go stale as lists change). */
export async function reloadRef() {
  const all = await api.ref();
  for (const t of REF_TABLES) { ref[t] = all[t] || []; index(t); }
}

/** Create (id == null) or update a reference row; the cache follows. Returns the saved row. */
export async function saveRef(table, id, body) {
  const row = id == null ? await api.createRef(table, body) : await api.updateRef(table, id, body);
  const i = ref[table].findIndex((r) => r.id === row.id);
  if (i >= 0) ref[table][i] = row; else ref[table].push(row);
  index(table);
  return row;
}

/** Delete a reference row (rejects with the server's message while it is in use). */
export async function removeRef(table, id) {
  await api.deleteRef(table, id);
  ref[table] = ref[table].filter((r) => r.id !== id);
  index(table);
}

// ---------------------------------------------------------------- lookups

/** The name of a row in a table, or '' when the id is null/unknown. */
export const nameOf = (table, id) => (id == null ? '' : ref.by[table].get(id)?.name ?? '');

/** Sorted copy of a table by name. */
export const sortedRows = (table) => [...ref[table]].sort((a, b) => byText(a.name, b.name));

/** The statuses in the order they are offered everywhere: as created (by id). */
export const statuses = () => [...ref.statuses].sort((a, b) => a.id - b.id);

/** The status a newly added entry starts with: the first one. */
export const defaultStatus = () => statuses()[0] || null;

/** A status's colour ("#rrggbb"); grey for an unknown id. */
export const statusColor = (id) => ref.by.statuses.get(id)?.color || '#9aa3ad';

/** The status whose shortcut is this key (matched without regard to case), or undefined. */
export function statusForKey(key) {
  if (!key || key.length !== 1) return undefined;
  const k = key.toLowerCase();
  return ref.statuses.find((s) => s.shortcut === k);
}
