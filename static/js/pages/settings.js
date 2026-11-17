// Settings page (#/settings): the statuses an item on a list can have, each
// with the colour it is drawn in and the key that assigns it, and a reference
// of the keys a list answers to.
//
// (Categories and storage locations are on the Library page, beside the items
// they describe.)

import { ref, reloadRef, statuses } from '../state.js';
import { el, toast, plural } from '../util.js';
import { icon } from '../icons.js';
import { refTable, PALETTE } from '../refedit.js';

/** The first palette colour no status uses yet (for a new status). */
function nextColor() {
  const used = new Set(ref.statuses.map((s) => s.color));
  return PALETTE.find((c) => !used.has(c)) || PALETTE[0];
}

const kbd = (...keys) => keys.map((k) => el('kbd', {}, k));

export async function mount(ctx) {
  try { await reloadRef(); } catch (e) { toast(e.message); }

  const table = refTable({
    table: 'statuses', title: 'Statuses', noun: 'status', label: (r) => r.name,
    sort: (a, b) => a.id - b.id,
    columns: [
      { key: 'name', label: 'Name', kind: 'text', required: true, maxLength: 127, primary: true },
      { key: 'color', label: 'Colour', kind: 'color', required: true, default: nextColor, title: 'The colour entries with this status are drawn in' },
      { key: 'shortcut', label: 'Key', kind: 'key', required: true, title: 'The key that gives the selected entry this status (one character; letters work with or without Shift)' },
      {
        key: 'use_count', label: 'In use', kind: 'computed', title: 'How many list entries have this status', width: '150px',
        render: (r) => el('span', {},
          r.use_count ? plural(r.use_count, 'entry', 'entries') : el('span', { class: 'muted' }, '—'),
          r.id === statuses()[0]?.id ? el('span', { class: 'chip small', style: { marginLeft: '8px' }, title: 'New entries start with this status (the first one in the table)' }, 'default') : null),
      },
    ],
    onChange: () => table.refresh(),
  });

  const addBtn = el('button', { type: 'button', class: 'primary', id: 'btn-add', onClick: () => table.addDraft(addBtn) }, icon('plus', { size: 14 }), 'Add status');
  ctx.toolbar.append(el('span', { class: 'spacer' }), addBtn);

  const wrap = el('div', { class: 'table-wrap' }, table.el);
  wrap.style.maxWidth = '760px';
  ctx.main.append(
    el('div', { class: 'section settings-body' },
      el('h2', {}, 'Statuses'),
      el('p', { class: 'hint' },
        'Where an item on a list stands. Each status has a colour (a list tints every entry, and every container’s box, in the colour of its status) and a key: with an entry selected on a list, pressing the key gives it that status. ',
        'New entries start with the first status in the table. A status can only be deleted once no entry uses it.'),
      wrap),
    el('div', { class: 'card keys-card' },
      el('h2', {}, 'Keys on a list'),
      el('dl', { class: 'keys-grid' },
        el('dt', {}, kbd('←', '↑', '→', '↓')), el('dd', {}, 'Move the selection to the nearest entry in that direction: up and down a list, across to the box beside it.'),
        el('dt', {}, kbd('Home', 'End')), el('dd', {}, 'The first or last entry.'),
        el('dt', {}, kbd('Space')), el('dd', {}, 'Open or close the selected container.'),
        el('dt', {}, kbd('Enter')), el('dd', {}, 'Edit the selected entry’s quantity.'),
        el('dt', {}, kbd('Esc')), el('dd', {}, 'Select the container the selection is in. In a text box: leave it, back to the keys above.'),
        el('dt', {}, 'A status’s key'), el('dd', {}, 'Give the selected entry that status. With Shift (letter keys), also move on to the next entry.'))),
  );
}
