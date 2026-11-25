// The view of a list: boxes within boxes. Every container is a box, tinted
// in the colour of the container's own status (which is its own: it does not
// follow what is inside), with a heading that carries the container's fields
// and a chip for each status found among the entries directly inside it.
// Inside it sit the boxes of the containers it holds, side by side while
// there is room, and under them the items packed straight into it, as a list
// with all their fields: quantity and unit, category, where the item is
// stored, status and notes. A container's compartments are drawn as plain
// boxes of their own inside it, ahead of what is in no compartment. Every
// row is tinted by its status, so the page reads as a map of what is packed
// and what is not; each level of containers is framed and headed more
// lightly than the one around it. (The layout is the one Squirrel's budget
// item picker uses, with lists in place of its chips.)
//
//   const v = boxesView(st, act);     st and act are the controller's (pages/list.js)
//   v.el, v.render(), v.showSelection(reveal), v.updateRow(id), v.key(event), v.focusDraft(), v.destroy()
//
// A row lays its fields out in one line when its box is wide enough and in
// two when it is not (css/list.css), so narrow boxes can sit side by side.
// st.showMeta turns the category and storage location off.
//
// Quantity, unit and notes are live controls that save when they change.
// While one has the focus the arrow keys and status keys belong to it; Enter
// or Escape gives them back to the list.
//
// Keys: the arrows move the selection to the nearest row in that direction
// (the page is two-dimensional, so they go by position); Home and End to the
// first and last row; Space opens or closes the selected container; Enter
// edits the selected entry's quantity; Escape selects the container the
// selection is in, or nothing once it is at the top level.
//
// Dragging: an entry (a container by its heading, with everything in it) can
// be dragged onto another container's box, onto one of its compartments, or
// out to the top level; see "drag and drop" below.
//
// Printing (css/list.css, @media print): the same boxes, set small and close,
// with an empty tick box in place of each status chip (.print-check) and the
// text boxes' values as plain text that can wrap (.print-text).

import { ref, nameOf, statuses, defaultStatus, statusColor, compartmentsOf } from '../state.js';
import { el, toast, plural, byText, fold, contrastText } from '../util.js';
import { icon } from '../icons.js';
import { combo } from '../widgets.js';
import { statusChip, statusById } from '../status.js';
import { newItemDialog } from './additem.js';

// How wide a box asks to be: one with only items in it asks for BOX_W; one
// that holds other boxes for enough to set them side by side. The page gives
// each line of boxes what is there (css/list.css reads --want).
const BOX_W = 400;
const BOX_GAP = 8;
const BOX_PAD = 18;

export function boxesView(st, act) {
  const root = el('div', { class: 'pack', role: 'tree' });
  let rowEls = new Map(); // entry id -> its row element (a container's heading is its row)
  let draft = null;       // the add-entry row's handle, while it is open
  let lastStatus = null;  // the status the last entry was added with: the next one starts there

  const isContainer = (n) => !!n.item?.is_container;
  /** Is this container drawn open (its contents showing)? */
  const isOpen = (n) => n.children.some((c) => c.show) && (st.filtering || !st.collapsed.has(n.row.item_id));

  // ---------------------------------------------------------------- rows

  /** A text box that saves when it changes; Enter commits, Escape puts the old value back. */
  function liveInput(attrs, { get, save }) {
    const input = el('input', { type: 'text', autocomplete: 'off', spellcheck: 'false', ...attrs, class: `cell-input ${attrs.class || ''}`.trim() });
    input.value = get();
    input.addEventListener('change', async () => {
      const ok = await save(input.value.trim());
      if (ok === false) { input.value = get(); return; }
      input.value = get();
      const cell = input.closest('.row > *');
      if (cell) { cell.classList.remove('saved'); void cell.offsetWidth; cell.classList.add('saved'); }
    });
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); input.blur(); }
      else if (e.key === 'Escape') { e.preventDefault(); input.value = get(); input.blur(); }
    });
    return input;
  }

  function parseQuantity(text) {
    const s = text.replace(/[,\s]/g, '');
    if (!/^\d+$/.test(s) || Number(s) < 1 || Number(s) > 2147483647) return null;
    return Number(s);
  }

  /**
   * What is directly inside a container, in brief: a chip for each status
   * found among those entries (the worst first, by the statuses' rank), each
   * with how many have it. What is inside the containers within it is theirs
   * to report. The statuses' names show where there is room (css/list.css);
   * the tooltip always says them. (.inside-total, the number of entries inside
   * at every depth, is what paper shows instead.)
   */
  function insideChips(n) {
    const counts = new Map();
    for (const c of n.children) counts.set(c.row.status_id, (counts.get(c.row.status_id) || 0) + 1);
    const present = statuses().filter((s) => counts.has(s.id));
    const deeper = n.size - n.children.length;
    return el('span', {
      class: 'inside',
      title: `Directly inside ${n.name}: ${present.map((s) => `${counts.get(s.id)} ${s.name}`).join(', ')}.`
        + (deeper ? ` (And ${plural(deeper, 'entry', 'entries')} inside the containers in it.)` : ''),
    },
      present.map((s) => el('span', { class: 'inside-chip', style: `--c: ${s.color}; --t: ${contrastText(s.color)}`, dataset: { status: s.id } },
        el('span', { class: 'inside-count' }, counts.get(s.id)), el('span', { class: 'inside-name' }, s.name))),
      el('span', { class: 'inside-total' }, n.size));
  }

  /** One entry as a row: a container's heading, or an item in a list. */
  function entryRow(n) {
    const row = n.row;
    const container = isContainer(n);
    const open = container && (isOpen(n) || sectionsOpen(n));

    const caret = !container ? null : n.children.length
      ? el('button', {
        type: 'button', class: 'caret', 'aria-expanded': String(open), disabled: st.filtering,
        'aria-label': open ? `Close ${n.name}` : `Open ${n.name}`,
        title: st.filtering ? 'Every match is shown while the filter is on' : open ? 'Close' : `Open (${plural(n.size, 'entry', 'entries')} inside)`,
        onClick: (e) => { e.stopPropagation(); act.toggle(n); },
      })
      : el('span', { class: 'caret-space' });
    const nameCell = el('div', { class: 'name' },
      caret,
      container ? el('span', { class: 'box-mark', title: 'A container: other items can be packed inside it' }, icon('box', { size: 14 })) : null,
      el('span', { class: 'item-name', title: n.name }, n.name),
      n.children.length ? insideChips(n) : null);

    const qty = liveInput({ class: 'num qty-input', inputmode: 'numeric', 'aria-label': `Quantity of ${n.name}`, title: 'Quantity' }, {
      get: () => String(row.quantity),
      save: async (text) => {
        const v = parseQuantity(text);
        if (v == null) { toast('Quantity must be a whole number from 1 up.'); return false; }
        if (v === row.quantity) return true;
        const ok = await act.edit(n.id, { quantity: v });
        onPaper();
        return ok;
      },
    });
    const unit = liveInput({ class: 'unit-input', maxlength: 127, placeholder: 'unit', 'aria-label': `Unit of ${n.name}`, title: 'For when the quantity is not a simple count: kg, pairs…' }, {
      get: () => row.unit ?? '',
      save: async (text) => {
        if (text === (row.unit ?? '')) return true;
        const ok = await act.edit(n.id, { unit: text || null });
        onPaper();
        return ok;
      },
    });
    const notes = liveInput({ class: 'notes-input', maxlength: 255, placeholder: 'notes', 'aria-label': `Notes on ${n.name}`, title: 'Notes' }, {
      get: () => row.notes ?? '',
      save: async (text) => {
        if (text === (row.notes ?? '')) return true;
        const ok = await act.edit(n.id, { notes: text || null });
        notes.closest('.row.entry')?.classList.toggle('has-notes', !!row.notes);
        onPaper();
        return ok;
      },
    });
    // The same values as plain text, for paper: a text box cannot wrap, and would cut a long note short.
    const qtyText = el('span', { class: 'print-text print-qty' });
    const notesText = el('span', { class: 'print-text print-notes' });
    function onPaper() {
      qtyText.textContent = row.unit ? `${row.quantity} ${row.unit}` : String(row.quantity);
      notesText.textContent = row.notes ?? '';
    }
    onPaper();

    const status = statusById(row.status_id);
    const chip = statusChip(status, { title: `Status of ${n.name}: click to change it, or select the row and press a status’s key`, onClick: (e) => { e.stopPropagation(); act.select(n.id, { reveal: false }); act.openStatusMenu(chip, n); } });

    const category = nameOf('item_categories', n.item?.item_category_id);
    const location = nameOf('storage_locations', n.item?.storage_location_id);
    const actions = el('div', { class: 'actions' },
      container ? el('button', { type: 'button', class: 'add-inside', title: `Add an item inside ${n.name}`, 'aria-label': `Add an item inside ${n.name}`, onClick: (e) => { e.stopPropagation(); act.select(n.id, { reveal: false }); act.startAdd(row.item_id); } }, icon('add-inside', { size: 14 })) : null,
      el('button', { type: 'button', class: 'move', title: 'Move into another container…', 'aria-label': `Move ${n.name}`, 'aria-haspopup': 'menu', onClick: (e) => { e.stopPropagation(); act.select(n.id, { reveal: false }); act.openMoveMenu(e.currentTarget, n); } }, icon('move', { size: 14 })),
      el('button', { type: 'button', class: 'danger delete', title: 'Remove from the list', 'aria-label': `Remove ${n.name}`, onClick: (e) => { e.stopPropagation(); act.remove(n.id); } }, icon('trash', { size: 14 })));

    const rowEl = el('div', {
      class: `row entry${container ? ' container' : ''}${n.match ? '' : ' dim'}${n.id === st.selected ? ' selected' : ''}${row.notes ? ' has-notes' : ''}`,
      role: 'treeitem', draggable: 'true', 'aria-selected': String(n.id === st.selected), 'aria-level': n.depth + 1,
      'aria-expanded': n.children.length ? String(open) : null,
      style: `--c: ${statusColor(row.status_id)}`,
      dataset: { id: n.id, item: row.item_id, name: n.name, depth: n.depth, status: row.status_id },
    },
      nameCell,
      el('div', { class: 'qty ed' }, qty, unit, qtyText),
      el('div', { class: 'meta' },
        el('span', { class: 'category', title: `Category: ${category}` }, category),
        el('span', { class: 'location', title: `Stored in: ${location}` }, location)),
      // (On paper the chip gives way to a box to tick.)
      el('div', { class: 'status' }, chip, el('span', { class: 'print-check', 'aria-hidden': 'true' })),
      el('div', { class: 'notes ed' }, notes, notesText),
      actions);
    rowEl.addEventListener('click', () => act.select(n.id, { reveal: false }));
    rowEl.addEventListener('focusin', () => act.select(n.id, { reveal: false }));
    if (container) nameCell.addEventListener('dblclick', () => act.toggle(n));
    rowEls.set(n.id, rowEl);
    return rowEl;
  }

  // ---------------------------------------------------------------- the add-entry row

  /** The library items that can still go into `container` (an item id, or null for the top level). */
  function addable(container) {
    const here = new Set(st.rows.filter((r) => (r.container_id ?? null) === container).map((r) => r.item_id));
    const onList = new Set(st.rows.map((r) => r.item_id));
    return ref.items
      // Already in this place; or a container that is on the list (a container has one place: move it instead).
      .filter((it) => !here.has(it.id) && !(it.is_container && onList.has(it.id)))
      .sort((a, b) => byText(a.name, b.name))
      .map((it) => ({ value: it.id, label: it.name, hint: `${nameOf('item_categories', it.item_category_id)}${it.is_container ? ' · container' : ''}` }));
  }

  function draftRow(container, compartment = null) {
    const parent = container == null ? null : st.tree.holder.get(container);
    let busy = false;

    const picker = combo({
      options: () => addable(container), inputClass: 'cell-input',
      placeholder: container == null ? 'Item to add at the top level: type to search the library' : 'Item: type to search the library',
      onChange: () => { picker.input.classList.remove('invalid'); describe(); },
      onCreate: async (text) => {
        // A name the library already has is only missing from the choices because it cannot go here.
        const existing = ref.items.find((it) => fold(it.name) === fold(text));
        if (existing) {
          toast(existing.is_container
            ? `“${existing.name}” is a container and is already on this list: a container has one place, so move it instead.`
            : `“${existing.name}” is already in this place: change its quantity instead.`, { info: true });
          return null;
        }
        const item = await newItemDialog(text);
        return item ? { value: item.id, label: item.name } : null;
      },
    });
    const where = parent ? `${parent.name}${compartment != null ? `, ${nameOf('compartments', compartment)}` : ''}` : '';
    picker.input.setAttribute('aria-label', parent ? `Item to add inside ${where}` : 'Item to add to the list');
    const qty = el('input', { type: 'text', class: 'cell-input num qty-input', value: '1', inputmode: 'numeric', 'aria-label': 'Quantity' });
    const unit = el('input', { type: 'text', class: 'cell-input unit-input', maxlength: 127, placeholder: 'unit', 'aria-label': 'Unit' });
    const notes = el('input', { type: 'text', class: 'cell-input notes-input', maxlength: 255, placeholder: 'Notes', 'aria-label': 'Notes' });
    const start = statusById(lastStatus) || defaultStatus();
    const status = el('select', { class: 'cell-select', 'aria-label': 'Status' }, statuses().map((s) => el('option', { value: String(s.id) }, s.name)));
    status.value = String(start?.id ?? '');
    const category = el('span', { class: 'category' });
    const location = el('span', { class: 'location' });
    function describe() {
      const it = ref.by.items.get(picker.value);
      category.textContent = it ? nameOf('item_categories', it.item_category_id) : '';
      location.textContent = it ? nameOf('storage_locations', it.storage_location_id) : '';
    }

    async function save() {
      if (busy) return;
      const itemId = picker.value;
      const n = parseInt(qty.value.replace(/[,\s]/g, ''), 10);
      const badQty = !/^\d+$/.test(qty.value.replace(/[,\s]/g, '')) || n < 1;
      picker.input.classList.toggle('invalid', itemId == null);
      qty.classList.toggle('invalid', badQty);
      if (itemId == null) { toast('Pick the item to add (type a new name to create it).'); picker.focus(); return; }
      if (badQty) { toast('Quantity must be a whole number from 1 up.'); qty.focus(); return; }
      busy = true;
      lastStatus = Number(status.value);
      // On success the list is drawn again with this row still in its place, emptied for the next item.
      const row = await act.add({
        item_id: itemId, container_id: container, compartment_id: compartment, quantity: n, unit: unit.value.trim() || null,
        status_id: lastStatus, notes: notes.value.trim() || null,
      });
      busy = false;
      if (row) { reset(); focusDraft(); }
    }

    /** Empty the row for the next entry (the status stays: a run of entries usually shares one). */
    function reset() {
      picker.setValue(null);
      qty.value = '1';
      unit.value = '';
      notes.value = '';
      for (const input of [picker.input, qty]) input.classList.remove('invalid');
      describe();
    }

    const rowEl = el('div', { class: 'row entry new', dataset: { draft: container ?? '' } },
      el('div', { class: 'name ed' }, picker.el),
      el('div', { class: 'qty ed' }, qty, unit),
      el('div', { class: 'meta' }, category, location),
      el('div', { class: 'status ed' }, status),
      el('div', { class: 'notes ed' }, notes),
      el('div', { class: 'actions' },
        el('button', { type: 'button', class: 'primary save', title: 'Add (Enter)', 'aria-label': 'Add the entry', onClick: save }, icon('check', { size: 14 })),
        el('button', { type: 'button', class: 'cancel', title: 'Done adding (Escape)', 'aria-label': 'Done adding', onClick: () => act.cancelAdd() }, icon('x', { size: 14 }))));
    for (const input of [qty, unit, notes]) input.addEventListener('input', () => input.classList.remove('invalid'));
    // Enter adds and Escape closes from anywhere in the row. (The item
    // picker takes both for itself while its list is showing, and marks the
    // event as handled.)
    rowEl.addEventListener('keydown', (e) => {
      if (e.isComposing || e.defaultPrevented) return;
      if (e.key === 'Escape') { e.preventDefault(); act.cancelAdd(); }
      else if (e.key === 'Enter' && !e.target.closest('button') && e.target.tagName !== 'SELECT') { e.preventDefault(); save(); }
    });
    return { el: rowEl, container, compartment, focus: () => picker.focus() };
  }

  function focusDraft() { if (draft) draft.focus(); }

  // ---------------------------------------------------------------- boxes

  /** The width a group of entries asks for: BOX_W for items alone, more to set the boxes among them side by side. */
  function wantOf(kids) {
    const boxes = kids.filter(isContainer);
    if (!boxes.length) return BOX_W;
    return Math.max(BOX_W, boxes.reduce((sum, b) => sum + want(b), 0) + BOX_GAP * (boxes.length - 1) + BOX_PAD);
  }

  /** The compartments of a container that are drawn: all of them, or while a filter is on only those with a match in them. */
  const sectionsOf = (n, kids) => compartmentsOf(n.row.item_id)
    .filter((c) => !st.filtering || kids.some((k) => k.compartment === c.id) || isAdding(n.row.item_id, c.id));

  /** The width a container's box asks for (see BOX_W). */
  function want(n) {
    if (!isOpen(n) && !sectionsOpen(n)) return BOX_W;
    const kids = n.children.filter((c) => c.show);
    const sections = sectionsOf(n, kids);
    const widths = sections.map((c) => wantOf(kids.filter((k) => k.compartment === c.id)));
    const loose = kids.filter((k) => k.compartment == null && isContainer(k));
    const count = sections.length + loose.length;
    if (!count) return BOX_W;
    return Math.max(BOX_W, widths.reduce((a, w) => a + w, 0) + loose.reduce((sum, b) => sum + want(b), 0) + BOX_GAP * (count - 1) + BOX_PAD);
  }

  /** Is the add-entry row open in this place? */
  const isAdding = (container, compartment = null) => !!st.adding && st.adding.container === container && (st.adding.compartment ?? null) === compartment;
  /** An empty container still shows its compartments (somewhere to add to), unless it was closed. */
  const sectionsOpen = (n) => !n.children.length && !st.filtering && !st.collapsed.has(n.row.item_id) && compartmentsOf(n.row.item_id).length > 0;

  /** A list of rows, with the add-entry row first when it belongs here. */
  const list = (rows, draftEl = null) => el('div', { class: 'pk-list', role: 'group' }, draftEl, rows.map(entryRow));

  /**
   * One compartment of a container: a plain box with the compartment's name,
   * holding the containers and the items that are in it.
   */
  function section(n, c, kids, draftFor) {
    const boxes = kids.filter(isContainer), items = kids.filter((k) => !isContainer(k));
    const adding = isAdding(n.row.item_id, c.id);
    if (adding) draft = draftFor(n.row.item_id, c.id);
    return el('section', { class: 'pk-comp', style: `--want: ${wantOf(kids)}px`, 'aria-label': `${c.name} of ${n.name}`, dataset: { compartment: c.id, name: c.name, container: n.row.item_id } },
      el('div', { class: 'pk-comp-head' },
        el('span', { class: 'pk-comp-name', title: `A compartment of ${n.name}` }, c.name),
        kids.length ? null : el('span', { class: 'pk-comp-empty' }, 'empty'),
        el('button', {
          type: 'button', class: 'bare icon add-to-compartment', title: `Add an item to ${c.name}`, 'aria-label': `Add an item to ${c.name} of ${n.name}`,
          onClick: (e) => { e.stopPropagation(); act.startAdd(n.row.item_id, c.id); },
        }, icon('plus', { size: 12 }))),
      adding ? list([], draft.el) : null,
      boxes.length ? el('div', { class: 'pk-boxes' }, boxes.map((b) => box(b, draftFor))) : null,
      items.length ? list(items) : null);
  }

  /**
   * A container as a box: its heading, the add-entry row when something is
   * being added to it, its compartments and the boxes of the containers
   * that are in none of them, then the items packed straight into it.
   */
  function box(n, draftFor) {
    const open = isOpen(n) || sectionsOpen(n);
    const kids = open ? n.children.filter((c) => c.show) : [];
    const sections = open ? sectionsOf(n, kids) : [];
    const loose = kids.filter((k) => k.compartment == null);
    const boxes = loose.filter(isContainer), items = loose.filter((c) => !isContainer(c));
    const adding = isAdding(n.row.item_id);
    if (adding) draft = draftFor(n.row.item_id, null);
    const head = entryRow(n);
    const body = el('div', { class: 'pk-body' },
      adding ? list([], draft.el) : null,
      sections.length || boxes.length ? el('div', { class: 'pk-boxes' },
        sections.map((c) => section(n, c, kids.filter((k) => k.compartment === c.id), draftFor)),
        boxes.map((b) => box(b, draftFor))) : null,
      items.length ? list(items) : null);
    return el('section', {
      class: `pk-box${body.childElementCount ? '' : ' closed'}`, style: `--c: ${statusColor(n.row.status_id)}; --want: ${want(n)}px`,
      // level: how deep the container is, for the framing (0 = the outermost; 3 stands for anything deeper).
      'aria-label': n.name, dataset: { box: n.id, container: n.row.item_id, level: Math.min(n.depth, 3) },
    }, head, body.childElementCount ? body : null);
  }

  function render() {
    rowEls = new Map();
    // The add-entry row outlives a redraw (what was typed into it stays), as long as it is for the same place.
    const kept = draft && st.adding && draft.container === st.adding.container && draft.compartment === (st.adding.compartment ?? null) ? draft : null;
    const focused = kept && kept.el.contains(document.activeElement) ? document.activeElement : null;
    draft = null;
    const draftFor = (container, compartment = null) => kept || draftRow(container, compartment);

    const top = st.tree.roots.filter((n) => n.show);
    const boxes = top.filter(isContainer), loose = top.filter((n) => !isContainer(n));
    const outer = el('div', { class: 'pk-boxes top' });
    // Adding at the top level: the row comes first, where it is in view.
    if (st.adding && st.adding.container == null) {
      draft = draftFor(null);
      outer.append(el('section', { class: 'pk-box loose adding', style: '--want: 100%' }, list([], draft.el)));
    }
    outer.append(...boxes.map((b) => box(b, draftFor)));
    // What is on the list without being in anything.
    if (loose.length) {
      outer.append(el('section', { class: 'pk-box loose', style: `--want: ${BOX_W}px`, 'aria-label': 'Not in a container', dataset: { container: '' } },
        boxes.length ? el('div', { class: 'pk-loose-head' }, 'Not in a container') : null, list(loose)));
    }

    root.setAttribute('aria-label', st.list.name);
    root.classList.toggle('no-meta', !st.showMeta);
    if (!st.rows.length && !st.adding) {
      root.replaceChildren(el('div', { class: 'empty list-empty' },
        el('p', {}, 'This list is empty.'),
        el('p', {}, el('button', { type: 'button', class: 'primary', onClick: () => act.startAdd(null) }, icon('plus', { size: 14 }), 'Add the first item')),
        el('p', { class: 'muted' }, 'Start with a container (a car, a suitcase, a backpack) and add what goes inside it with the box button at the end of its heading.')));
    } else if (!st.visible.length && st.rows.length && !st.adding) {
      root.replaceChildren(el('div', { class: 'empty list-empty' }, 'No entries match the filter.'));
    } else {
      // (The strip is somewhere to drop for the top level; it only shows while something is being dragged.)
      root.replaceChildren(outer, el('div', { class: 'pk-top-drop', 'aria-hidden': 'true' }, 'Drop here for the top level of the list'));
    }
    if (focused && focused.isConnected) focused.focus();
  }

  function showSelection(reveal) {
    for (const [rowId, rowEl] of rowEls) {
      const on = rowId === st.selected;
      rowEl.classList.toggle('selected', on);
      rowEl.setAttribute('aria-selected', String(on));
    }
    if (reveal && st.selected != null) rowEls.get(st.selected)?.scrollIntoView({ block: 'nearest' });
  }

  /** Redraw one row from the state (after a save the server refused). */
  function updateRow(rowId) {
    const n = st.tree.nodes.get(rowId);
    const old = rowEls.get(rowId);
    if (!n || !old) return;
    old.replaceWith(entryRow(n));
  }

  // ---------------------------------------------------------------- drag and drop
  //
  // An entry is dragged by its row (a container by its heading: the whole box
  // comes along) and dropped on the place it should go:
  //
  //   a compartment's box       into that compartment of its container
  //   a container's box         into the container, in no compartment
  //   anywhere else on the page to the top level of the list
  //
  // The innermost place under the pointer is the one meant. A place the entry
  // cannot go (where it already is, inside itself, where the same item
  // already is) does not take the drop: act.canMove decides, with the same
  // rules as the Move menu.

  let dragging = null;   // the node being dragged
  let dropEl = null;     // the place under the pointer that would take it

  /** The place a point of the page stands for: {el, container, compartment}. */
  function placeAt(target) {
    const zone = target.closest ? target.closest('.pk-comp, .pk-box, .pack') : null;
    if (!zone) return null;
    const container = zone.dataset.container ? Number(zone.dataset.container) : null;
    const compartment = zone.dataset.compartment ? Number(zone.dataset.compartment) : null;
    // The page itself, or the box of what is in no container: the top level (marked on the page as a whole).
    return { el: container == null ? root : zone, container, compartment };
  }

  function markDrop(elm) {
    if (dropEl === elm) return;
    if (dropEl) dropEl.classList.remove('drop-target');
    dropEl = elm;
    if (dropEl) dropEl.classList.add('drop-target');
  }

  function endDrag() {
    markDrop(null);
    root.classList.remove('drag-on');
    root.querySelectorAll('.dragging').forEach((e) => e.classList.remove('dragging'));
    dragging = null;
  }

  // A row is draggable, but not by its text boxes and buttons: pressing one of those is for it alone.
  root.addEventListener('pointerdown', (e) => {
    const rowEl = e.target.closest('.row.entry');
    if (rowEl && !rowEl.classList.contains('new')) rowEl.draggable = !e.target.closest('input, select, textarea, button');
  }, true);

  root.addEventListener('dragstart', (e) => {
    const rowEl = e.target.closest ? e.target.closest('.row.entry:not(.new)') : null;
    const node = rowEl ? st.tree.nodes.get(Number(rowEl.dataset.id)) : null;
    if (!node) return;
    dragging = node;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', node.name);
    // A container is seen to leave with everything in it.
    const whole = isContainer(node) ? rowEl.parentElement : rowEl;
    if (whole !== rowEl) e.dataTransfer.setDragImage(whole, 16, 14);
    act.select(node.id, { reveal: false });
    // (After the browser has taken its picture of what is dragged.)
    setTimeout(() => { if (dragging === node) { whole.classList.add('dragging'); root.classList.add('drag-on'); } }, 0);
  });

  // Entering an element and moving over it both say where the pointer is.
  const over = (e) => {
    if (!dragging) return;
    const place = placeAt(e.target);
    if (place && act.canMove(dragging, place.container, place.compartment)) {
      e.preventDefault(); // this place takes the drop
      e.dataTransfer.dropEffect = 'move';
      markDrop(place.el);
    } else {
      markDrop(null);
    }
  };
  root.addEventListener('dragenter', over);
  root.addEventListener('dragover', over);

  // Leaving the page for good unmarks the place; leaving one element of it for the next does not (the
  // dragover that follows says where the pointer is now). Told apart by where the pointer is.
  root.addEventListener('dragleave', (e) => {
    const r = root.getBoundingClientRect();
    if (e.clientX <= r.left || e.clientX >= r.right || e.clientY <= r.top || e.clientY >= r.bottom) markDrop(null);
  });

  root.addEventListener('drop', (e) => {
    if (!dragging) return;
    const node = dragging;
    const place = placeAt(e.target);
    endDrag();
    if (!place || !act.canMove(node, place.container, place.compartment)) return;
    e.preventDefault();
    act.move(node.id, place.container, place.compartment);
  });

  root.addEventListener('dragend', endDrag);

  // ---------------------------------------------------------------- keys

  /** The gap between two ranges on one axis (0 when they overlap). */
  const gap = (a0, a1, b0, b1) => Math.max(0, Math.max(a0, b0) - Math.min(a1, b1));

  /**
   * The row nearest to `cur` in a direction: the closest one that lies
   * beyond cur's edge, preferring those level with it; among equals, the one
   * whose leading edge lines up best (so Down from a container's heading
   * lands on the first box inside it, not the middle one).
   */
  function nearest(cur, dir) {
    const c = cur.getBoundingClientRect();
    let best = null, bestScore = Infinity;
    const eps = 0.5;
    for (const [rowId, rowEl] of rowEls) {
      if (rowEl === cur) continue;
      const t = rowEl.getBoundingClientRect();
      let along, across, edge;
      if (dir === 'right') { if (t.left < c.right - eps) continue; along = t.left - c.right; across = gap(c.top, c.bottom, t.top, t.bottom); edge = Math.abs(t.top - c.top); }
      else if (dir === 'left') { if (t.right > c.left + eps) continue; along = c.left - t.right; across = gap(c.top, c.bottom, t.top, t.bottom); edge = Math.abs(t.top - c.top); }
      else if (dir === 'down') { if (t.top < c.bottom - eps) continue; along = t.top - c.bottom; across = gap(c.left, c.right, t.left, t.right); edge = Math.abs(t.left - c.left); }
      else { if (t.bottom > c.top + eps) continue; along = c.top - t.bottom; across = gap(c.left, c.right, t.left, t.right); edge = Math.abs(t.left - c.left); }
      const score = along + across * 4 + edge * 0.001;
      if (score < bestScore) { bestScore = score; best = rowId; }
    }
    return best;
  }

  function key(e) {
    const rows = st.visible;
    if (!rows.length) return false;
    const cur = st.selected != null ? st.tree.nodes.get(st.selected) : null;
    const curEl = cur ? rowEls.get(cur.id) : null;
    const dir = { ArrowRight: 'right', ArrowLeft: 'left', ArrowDown: 'down', ArrowUp: 'up' }[e.key];
    if (dir) {
      // Nothing selected yet: start at the first row (the last, going up).
      if (!curEl) { act.select(rows[dir === 'up' ? rows.length - 1 : 0].id); return true; }
      const next = nearest(curEl, dir);
      if (next != null) act.select(next);
      return true;
    }
    switch (e.key) {
      case 'Home': act.select(rows[0].id); return true;
      case 'End': act.select(rows[rows.length - 1].id); return true;
      case ' ':
        if (!cur || !cur.children.length || st.filtering) return !!cur;
        act.toggle(cur);
        return true;
      case 'Enter': {
        if (!curEl) return false;
        const input = curEl.querySelector('.qty-input');
        if (input) { input.focus(); input.select(); }
        return true;
      }
      case 'Escape':
        if (!cur) return false;
        act.select(cur.parent ? cur.parent.id : null); // out one level; from the top level, to nothing
        return true;
      default: return false;
    }
  }

  return { el: root, render, showSelection, updateRow, key, focusDraft, destroy() {} };
}
