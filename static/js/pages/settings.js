// Settings page (#/settings): the statuses an item on a list can have, each
// with the colour it is drawn in and the key that assigns it, ranked from the
// worst to the best; and a reference of the keys a list answers to.
//
// (Categories and storage locations are on the Library page, beside the items
// they describe.)

import { ref, reloadRef, statuses, defaultStatus, orderStatuses } from '../state.js';
import { el, toast, plural, sentence } from '../util.js';
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

  const wrap = el('div', { class: 'table-wrap' });
  wrap.style.maxWidth = '820px';
  let table;

  /** Move a status one place towards the worst (-1) or the best (+1) end and draw the table again in the new order. */
  async function move(status, by) {
    const order = statuses().map((s) => s.id);
    const i = order.indexOf(status.id), j = i + by;
    if (i < 0 || j < 0 || j >= order.length) return;
    [order[i], order[j]] = [order[j], order[i]];
    try {
      await orderStatuses(order);
    } catch (e) {
      toast(sentence(e.message));
      return;
    }
    build();
    wrap.querySelector(`.row[data-id="${status.id}"] .${by < 0 ? 'rank-up' : 'rank-down'}`)?.focus();
  }

  /** The table, in rank order: the worst status first. (Built again after a reordering: rows do not move by themselves.) */
  function build() {
    const order = statuses();
    const first = defaultStatus();
    table = refTable({
      table: 'statuses', title: 'Statuses', noun: 'status', label: (r) => r.name,
      sort: (a, b) => a.rank - b.rank || a.id - b.id,
      actionsWidth: '92px',
      columns: [
        { key: 'name', label: 'Name', kind: 'text', required: true, maxLength: 127, primary: true },
        { key: 'color', label: 'Colour', kind: 'color', required: true, default: nextColor, title: 'The colour entries with this status are drawn in' },
        { key: 'shortcut', label: 'Key', kind: 'key', required: true, title: 'The key that gives the selected entry this status (one character; letters work with or without Shift)' },
        {
          key: 'use_count', label: 'In use', kind: 'computed', title: 'How many list entries have this status', width: '150px',
          render: (r) => el('span', {},
            r.use_count ? plural(r.use_count, 'entry', 'entries') : el('span', { class: 'muted' }, '—'),
            r.id === first?.id ? el('span', { class: 'chip small', style: { marginLeft: '8px' }, title: 'New entries start with this status (the one that was made first)' }, 'default') : null),
        },
      ],
      rowActions: (r) => {
        const i = order.findIndex((s) => s.id === r.id);
        return [
          el('button', { type: 'button', class: 'rank-up', disabled: i <= 0, title: 'Worse: move up', 'aria-label': `Rank ${r.name} worse`, onClick: () => move(r, -1) }, icon('arrow-up', { size: 14 })),
          el('button', { type: 'button', class: 'rank-down', disabled: i < 0 || i >= order.length - 1, title: 'Better: move down', 'aria-label': `Rank ${r.name} better`, onClick: () => move(r, +1) }, icon('arrow-down', { size: 14 })),
        ];
      },
      // A new status goes to the end (the best); one deleted changes who can move where.
      onChange: () => { if (ref.statuses.length !== order.length) build(); else table.refresh(); },
    });
    wrap.replaceChildren(table.el);
  }
  build();

  const addBtn = el('button', { type: 'button', class: 'primary', id: 'btn-add', onClick: () => table.addDraft(addBtn) }, icon('plus', { size: 14 }), 'Add status');
  ctx.toolbar.append(el('span', { class: 'spacer' }), addBtn);

  ctx.main.append(
    el('div', { class: 'section settings-body' },
      el('h2', {}, 'Statuses'),
      el('p', { class: 'hint' },
        'Where an item on a list stands. Each status has a colour (a list tints every entry, and every container’s box, in the colour of its status) and a key: with an entry selected on a list, pressing the key gives it that status. ',
        'They are ranked from the worst at the top to the best at the bottom (the arrows move one up or down): that is the order they are shown in everywhere, and on a list a container’s heading has a chip for each status among the entries directly inside it. ',
        'New entries start with the status marked default. A status can only be deleted once no entry uses it.'),
      wrap),
    el('div', { class: 'card keys-card' },
      el('h2', {}, 'Keys on a list'),
      el('p', { class: 'hint' }, 'An entry can also be dragged by its name onto another container, onto one of its compartments, or out to the top level.'),
      el('dl', { class: 'keys-grid' },
        el('dt', {}, kbd('←', '↑', '→', '↓')), el('dd', {}, 'Move the selection to the nearest entry in that direction: up and down a list, across to the box beside it.'),
        el('dt', {}, kbd('Home', 'End')), el('dd', {}, 'The first or last entry.'),
        el('dt', {}, kbd('Space')), el('dd', {}, 'Open or close the selected container.'),
        el('dt', {}, kbd('Enter')), el('dd', {}, 'Edit the selected entry’s quantity.'),
        el('dt', {}, kbd('Esc')), el('dd', {}, 'Select the container the selection is in; from the top level, select nothing (as does a click on empty space). In a text box: leave it, back to the keys above.'),
        el('dt', {}, 'A status’s key'), el('dd', {}, 'Give the selected entry that status. With Shift (letter keys), also move on to the next entry.'),
        el('dt', {}, kbd('Ctrl', 'F')), el('dd', {}, 'Go to the filter box, which searches the whole list (in the Library, all of the table). Pressed again there, it is the browser’s own Find.'),
        el('dt', {}, kbd('Ctrl', 'P')), el('dd', {}, 'Print the list as it is on screen, with a box to tick in place of each status.'))),
  );
}
