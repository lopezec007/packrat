// Library page (#/library?section=items): the master list of items that
// every packing list draws from, with the two tables that describe an item:
// its category and where it is stored when not in use.
//
// Layout (as Squirrel's Settings): a section list on the left, each with its
// row count, and the chosen section's editable table (refedit.js refTable) on
// the right. The title bar holds a filter box and "Add <thing>".
//
// The sections are data: SECTIONS below says, per table, what its columns
// are. URL parameter: section (a table name with dashes, e.g.
// storage-locations).

import { ref, reloadRef, compartmentsOf } from '../state.js';
import { compartmentsDialog } from '../compartments.js';
import { el, toast, plural } from '../util.js';
import { icon } from '../icons.js';
import { refTable } from '../refedit.js';

const name = (extra = {}) => ({ key: 'name', label: 'Name', kind: 'text', required: true, maxLength: 127, primary: true, ...extra });
const count = (key, label, title, one, many) => ({
  key, label, kind: 'computed', title, width: '110px',
  render: (r) => (r[key] ? plural(r[key], one, many) : el('span', { class: 'muted' }, '—')),
});

const SECTIONS = [
  {
    table: 'items', title: 'Items', noun: 'item', label: (r) => r.name,
    hint: 'Everything you own that might go on a list. An item belongs to one category and has one place it is stored when not in use; its name is unique across the whole library. Tick Container for anything other items can be packed inside (a car, a backpack, a toiletry kit): on a list, a container can hold items and other containers, as deep as you like, and can be given compartments to put them in. When adding items, Enter saves one and starts the next.',
    emptyText: 'The library is empty. Add the first item with the button above.',
    columns: [
      name({ width: 'minmax(180px, 1.4fr)' }),
      // (carry: a run of new items, Enter after Enter, starts each with the last one's category and place.)
      { key: 'item_category_id', label: 'Category', kind: 'select', options: 'item_categories', required: true, create: { noun: 'category' }, carry: true, width: 'minmax(130px, 0.9fr)' },
      { key: 'storage_location_id', label: 'Stored in', kind: 'select', options: 'storage_locations', required: true, create: { noun: 'storage location' }, carry: true, title: 'Where the item lives when it is not in use', width: 'minmax(130px, 0.9fr)' },
      { key: 'is_container', label: 'Container', kind: 'check', title: 'Other items can be packed inside it', width: '92px' },
      {
        key: 'compartments', label: 'Compartments', kind: 'computed', width: 'minmax(140px, 1fr)',
        title: 'The parts of a container that things can be packed into: a backpack’s lid pocket, a car’s boot',
        render: (r) => {
          if (!r.is_container) return el('span', { class: 'muted' }, '—');
          const names = compartmentsOf(r.id).map((c) => c.name);
          return el('button', {
            type: 'button', class: `link compartments-btn${names.length ? '' : ' none'}`,
            title: names.length ? `${names.join(', ')}: click to change them` : `Give ${r.name} compartments`, 'aria-label': `Compartments of ${r.name}`,
            onClick: async () => { await compartmentsDialog(r); refreshTable(); },
          }, names.length ? names.join(', ') : 'Add…');
        },
      },
      count('list_count', 'On lists', 'How many lists the item is on', 'list'),
    ],
  },
  {
    table: 'item_categories', title: 'Categories', noun: 'category', label: (r) => r.name, maxWidth: 620,
    hint: 'What kind of thing an item is: Clothing, Electronics, Camp… A category can only be deleted once no item uses it.',
    emptyText: 'No categories yet.',
    columns: [name(), count('item_count', 'Items', 'How many items are in the category', 'item')],
  },
  {
    table: 'storage_locations', title: 'Storage Locations', noun: 'storage location', label: (r) => r.name, maxWidth: 620,
    hint: 'Where items live when they are not in use: “Middle TV Cabinet”, “Bottom Desk Drawer”… A list shows each item’s location, so you know where to fetch it from.',
    emptyText: 'No storage locations yet.',
    columns: [name(), count('item_count', 'Items', 'How many items are stored there', 'item')],
  },
];

const slug = (table) => table.replace(/_/g, '-');

// The table on the page, for the cells that need to redraw it (set in mount).
let refreshTable = () => {};

export async function mount(ctx) {
  // The counts (lists per item, items per category) change as lists change.
  try { await reloadRef(); } catch (e) { toast(e.message); }

  const section = SECTIONS.find((s) => slug(s.table) === ctx.params.get('section')) || SECTIONS[0];
  ctx.setTitle(section.title === 'Items' ? 'Library' : `${section.title} · Library`);

  let table;
  const navEl = el('nav', { class: 'settings-nav', 'aria-label': 'Library sections' });
  function renderNav() {
    navEl.replaceChildren(...SECTIONS.map((s) => el('a', {
      href: `#/library?section=${slug(s.table)}`, 'aria-current': s === section ? 'page' : null,
    }, el('span', { class: 'label' }, s.title), el('span', { class: 'count' }, ref[s.table].length))));
  }

  // After any change: the counts beside the sections, and every row's choices (a category made from one row's list is offered in the others).
  table = refTable({ ...section, onChange: () => { renderNav(); table.refresh(); } });
  refreshTable = () => table.refresh();

  const filter = el('input', { type: 'search', class: 'filter-box', placeholder: `Filter ${section.title.toLowerCase()}…`, 'aria-label': `Filter ${section.title.toLowerCase()}` });
  filter.addEventListener('input', () => table.setFilter(filter.value));
  const addBtn = el('button', { type: 'button', class: 'primary', id: 'btn-add', onClick: () => table.addDraft(addBtn) }, icon('plus', { size: 14 }), `Add ${section.noun}`);
  ctx.toolbar.append(filter, el('span', { class: 'spacer' }), addBtn);

  const wrap = el('div', { class: 'table-wrap' }, table.el);
  if (section.maxWidth) wrap.style.maxWidth = `${section.maxWidth}px`;
  const body = el('div', { class: 'settings-body' },
    el('div', { class: 'section' }, el('h2', {}, section.title), el('p', { class: 'hint' }, section.hint), wrap));
  renderNav();
  ctx.main.append(el('div', { class: 'settings-layout' }, navEl, body));
}
