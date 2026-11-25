// Sorting a list's entries: the order of sort columns, the comparator, and
// the Sort menu in the title bar. It works as the planner's does.
//
// The sort is a list of {key, dir} applied in order: sort by status, then
// within one status by name, and so on. An empty list is the plain order,
// by name. The menu shows the columns in effect, lets each one's direction
// be flipped or the column removed, and adds further ones ("+ then by…").
//
// Whatever the sort, an entry keeps to its place in the layout: the entries
// of one container (and of one compartment of it) are sorted among
// themselves, and the containers among them come before the items (they are
// drawn as boxes, ahead of the list of items).
//
//   SORT_COLUMNS                 key -> {label, defaultDir?}
//   normalizeSort(stored)        a clean list of {key, dir} from whatever was stored
//   compareBy(sort)              comparator for sibling nodes (list/model.js)
//   sortButton({get, set})       the title bar button with its menu; .refresh() after the sort changes elsewhere

import { nameOf, statusRank } from '../state.js';
import { el } from '../util.js';
import { icon } from '../icons.js';
import { popover } from '../widgets.js';

export const SORT_COLUMNS = {
  name: { label: 'Name' },
  quantity: { label: 'Quantity', defaultDir: 'desc' }, // the most first
  category: { label: 'Category' },
  location: { label: 'Stored in' },
  status: { label: 'Status' },                         // by the statuses' rank: the worst first
  notes: { label: 'Notes' },                           // entries without notes last
};

const MAX_KEYS = Object.keys(SORT_COLUMNS).length;

export const defaultDir = (key) => SORT_COLUMNS[key]?.defaultDir || 'asc';

/** Sanitise a stored sort: known keys, valid directions, no duplicates. (A bare key, as stored before there were several, is one column.) */
export function normalizeSort(sort) {
  if (typeof sort === 'string') sort = sort === 'name' ? [] : [{ key: sort, dir: 'asc' }];
  const out = [];
  const seen = new Set();
  for (const s of Array.isArray(sort) ? sort : []) {
    if (!s || !SORT_COLUMNS[s.key] || seen.has(s.key)) continue;
    seen.add(s.key);
    out.push({ key: s.key, dir: s.dir === 'desc' ? 'desc' : 'asc' });
    if (out.length >= MAX_KEYS) break;
  }
  return out;
}

// ---- comparator ----

/** The sortable value of a node for a column; null sorts last in both directions. */
function value(n, key) {
  switch (key) {
    case 'name': return n.name;
    case 'quantity': return n.row.quantity;
    case 'category': return nameOf('item_categories', n.item?.item_category_id) || null;
    case 'location': return nameOf('storage_locations', n.item?.storage_location_id) || null;
    case 'status': return statusRank(n.row.status_id);
    case 'notes': return n.row.notes || null;
    default: return null;
  }
}

function cmpValues(a, b) {
  if (a == null && b == null) return 0;
  if (a == null) return 1; // missing values last, whatever the direction
  if (b == null) return -1;
  if (typeof a === 'string') return a.localeCompare(b, undefined, { sensitivity: 'base', numeric: true });
  return a - b;
}

/** Comparator for sibling nodes by the sort columns; ties fall back to the name, then to the order added. */
export function compareBy(sort) {
  const keys = normalizeSort(sort);
  return (a, b) => {
    for (const { key, dir } of keys) {
      const va = value(a, key), vb = value(b, key);
      let d = cmpValues(va, vb);
      if (d && dir === 'desc' && va != null && vb != null) d = -d;
      if (d) return d;
    }
    return cmpValues(a.name, b.name) || a.id - b.id;
  };
}

// ---- title bar menu ----

const ARROW = { asc: '↑', desc: '↓' };

/**
 * The Sort button: an icon with the number of sort columns in effect, and a
 * menu to change them.
 *
 *   const s = sortButton({get: () => sort, set: (sort) => {...}});
 *   s.el, s.refresh()
 */
export function sortButton({ get, set }) {
  const badge = el('span', { class: 'badge', id: 'sort-badge' });
  const btn = el('button', { type: 'button', id: 'btn-sort', 'aria-label': 'Sort', 'aria-haspopup': 'true', 'aria-expanded': 'false' }, icon('sort'), badge);
  const panel = el('div', { class: 'sort-panel', id: 'sort-panel', role: 'group', 'aria-label': 'Sort' });
  let close = null;

  const current = () => normalizeSort(get());
  const change = (sort) => { set(normalizeSort(sort)); refresh(); };

  function refresh() {
    const sort = current();
    const summary = sort.length ? sort.map((s) => `${SORT_COLUMNS[s.key].label} ${ARROW[s.dir]}`).join(', ') : 'Name';
    badge.textContent = sort.length || '';
    badge.hidden = !sort.length;
    btn.classList.toggle('active', sort.length > 0);
    btn.title = `Sort: ${summary}. Entries are sorted within their container; containers come before items.`;
    if (close) { fill(); close.reposition(); }
  }

  function fill() {
    const sort = current();
    panel.replaceChildren();
    panel.append(el('div', { class: 'field-label' }, sort.length ? 'Sort by' : 'Sorted by name'));
    if (sort.length) {
      const list = el('ol', { class: 'sort-list' });
      sort.forEach((s, i) => {
        const label = SORT_COLUMNS[s.key].label;
        list.append(el('li', { class: 'sort-item', dataset: { sortKey: s.key } },
          el('span', { class: 'sort-item-label' }, label),
          el('button', {
            type: 'button', class: 'icon sort-dir', title: s.dir === 'asc' ? 'Ascending: click for descending' : 'Descending: click for ascending',
            'aria-label': `${label}: ${s.dir === 'asc' ? 'ascending' : 'descending'}`,
            onClick: () => change(sort.map((x, j) => (j === i ? { ...x, dir: x.dir === 'asc' ? 'desc' : 'asc' } : x))),
          }, ARROW[s.dir]),
          el('button', {
            type: 'button', class: 'icon sort-remove', title: 'Remove from the sort', 'aria-label': `Remove ${label}`,
            onClick: () => change(sort.filter((_, j) => j !== i)),
          }, '×')));
      });
      panel.append(list);
    }
    // Columns that can still be added.
    const addable = Object.entries(SORT_COLUMNS).filter(([k]) => !sort.some((s) => s.key === k));
    if (addable.length && sort.length < MAX_KEYS) {
      const add = el('select', { class: 'sort-add', 'aria-label': 'Add a sort column' }, el('option', { value: '' }, sort.length ? '+ then by…' : '+ sort by…'));
      for (const [k, c] of addable) add.append(el('option', { value: k }, c.label));
      add.addEventListener('change', () => { if (add.value) change([...sort, { key: add.value, dir: defaultDir(add.value) }]); });
      panel.append(add);
    }
    if (sort.length) panel.append(el('button', { type: 'button', class: 'sort-clear', onClick: () => change([]) }, 'Back to by name'));
    panel.append(el('p', { class: 'sort-note muted' }, 'Entries are sorted within their container (and compartment); the containers among them come first.'));
  }

  btn.addEventListener('click', () => {
    if (close) { close(); return; }
    fill();
    btn.setAttribute('aria-expanded', 'true');
    close = popover(btn, panel, { onClose: () => { close = null; btn.setAttribute('aria-expanded', 'false'); } });
  });

  refresh();
  return { el: btn, refresh };
}
