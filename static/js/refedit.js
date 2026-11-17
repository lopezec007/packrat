// Editable reference tables: the data-driven component behind the Library
// and Settings pages (as in Squirrel). A table is described by a list of
// columns; every field of an existing row is a live control that saves when
// it changes (no edit mode, no Save button), and a new row is a draft with an
// accent bar that is created with Save / Enter or dropped with Cancel /
// Escape.
//
//   refTable(config)          a whole framed table for one reference table:
//                             header, sorted rows, filter, draft
//
// A config says which table the rows belong to and what its columns are:
//
//   {
//     table: 'items',                     key in state.js `ref`
//     title: 'Items',                     names the table for assistive tech
//     noun: 'item',                       used in prompts ("Delete item …?")
//     label: (row) => row.name,           what a row is called
//     columns: [...],                     see below
//     sort: (a, b) => number,             row order when the table is built (default: by label)
//     emptyText: 'No items yet.',
//     actionsWidth: '40px',
//     onChange() {},                      a row was created, saved or deleted
//   }
//
// A column:
//
//   {
//     key: 'item_category_id',  the row's field (and the key sent to the API)
//     label: 'Category',        header text; also names the control for assistive tech
//     kind: 'text' | 'check' | 'select' | 'color' | 'key'   (editable)
//         | 'computed',                                     (read-only)
//     width: '110px',           grid track (default by kind)
//     required: true,           must be filled in
//     maxLength, placeholder, title (header tooltip), cls (extra cell class)
//     options: 'item_categories' | () => [{value, label}]   select: a reference table (sorted by name) or a list
//     create: {noun: 'category'}                            select from a table: adds a "New category…" choice
//     default: value | () => value                          the value a draft row starts with
//     render: (row) => node | text                          computed: the cell's content
//   }
//
// All writes go through saveRef / removeRef (state.js), so the shared
// reference cache follows. A failed save restores the control and shows the
// server's message in a toast.

import { ref, saveRef, removeRef } from './state.js';
import { el, toast, byText, fold, sentence } from './util.js';
import { popover, modal } from './widgets.js';
import { icon } from './icons.js';

// ---------------------------------------------------------------- helpers

const READ_ONLY = new Set(['computed']);
const DEFAULT_WIDTH = {
  text: 'minmax(150px, 1fr)', check: '96px', select: 'minmax(150px, 1fr)', color: '170px', key: '96px', computed: '110px',
};
/** The colours the palette offers (the planner's and Squirrel's set). */
export const PALETTE = ['#4e79a7', '#f28e2b', '#59a14f', '#e15759', '#76b7b2', '#edc948', '#b07aa1', '#ff9da7', '#9c755f', '#8a94a3', '#2e8540', '#d62828'];

const same = (a, b) => (a ?? null) === (b ?? null);
const resolve = (v) => (typeof v === 'function' ? v() : v);

/** A column's options as [{value, label}] (a reference table's rows sorted by name, or the column's own list). */
export function optionsOf(col) {
  if (typeof col.options === 'function') return col.options();
  if (Array.isArray(col.options)) return col.options;
  return ref[col.options].map((r) => ({ value: r.id, label: r.name })).sort((a, b) => byText(a.label, b.label));
}

/** What a cell shows, as plain text (for the filter box). */
function cellText(col, row) {
  const v = row[col.key];
  if (v == null || READ_ONLY.has(col.kind) || col.kind === 'check') return '';
  if (col.kind === 'select') return optionsOf(col).find((o) => o.value === v)?.label ?? '';
  return String(v);
}

/** Everything searchable in a row, folded for matching. */
const rowText = (spec, row) => fold(spec.columns.map((c) => cellText(c, row)).join(' '));

function cellClass(col, editable) {
  return [
    col.kind === 'check' || col.kind === 'key' ? 'center' : '', col.primary ? 'name' : '', editable ? 'ed' : '', col.cls || '',
  ].filter(Boolean).join(' ');
}

const headerClass = (col) => (/\bnum\b/.test(col.cls || '') ? 'num' : col.kind === 'check' || col.kind === 'key' ? 'center' : null);

function gridTemplate(columns, actionsWidth = '40px') {
  return [...columns.map((c) => c.width || DEFAULT_WIDTH[c.kind] || DEFAULT_WIDTH.text), actionsWidth].join(' ');
}

function headerRow(columns) {
  return el('div', { class: 'row header', role: 'row' },
    columns.map((c) => el('div', { class: headerClass(c), role: 'columnheader', title: c.title, dataset: { col: c.key } }, c.label)),
    el('div', { class: 'actions', role: 'columnheader', 'aria-label': 'Actions' }));
}

function iconButton(name, attrs) {
  return el('button', { type: 'button', ...attrs }, icon(name, { size: 14 }));
}

function flash(cell) {
  if (!cell) return;
  cell.classList.remove('saved');
  void cell.offsetWidth; // restart the animation
  cell.classList.add('saved');
}

/**
 * Ask for one line of text in a dialog; resolves to the trimmed text, or
 * null when the dialog is dismissed.
 *
 *   const name = await promptText({title: 'New category', label: 'Name', value: '', maxLength: 127, action: 'Create'});
 */
export function promptText({ title, label, value = '', maxLength = 127, action = 'Save', hint = '' }) {
  return new Promise((done) => {
    let result = null;
    const input = el('input', { type: 'text', value, maxlength: maxLength, autocomplete: 'off', 'aria-label': label });
    const submit = () => {
      const text = input.value.trim();
      if (!text) { input.classList.add('invalid'); input.focus(); return false; }
      result = text;
      return true;
    };
    const m = modal({
      title,
      body: el('div', { class: 'field' }, el('label', {}, label), input, hint ? el('p', { class: 'hint' }, hint) : null),
      actions: [{ label: 'Cancel' }, { label: action, primary: true, onClick: submit }],
      onClose: () => done(result),
    });
    input.addEventListener('input', () => input.classList.remove('invalid'));
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); if (submit()) m.close(); } });
    input.select();
  });
}

// ---------------------------------------------------------------- editors
//
// makeEditor(col, ctx) -> {
//   nodes,            what goes in the cell
//   control,          the focusable element (null for read-only kinds)
//   show(value, row)  display a saved value: called on every redraw of the row (so options and
//                     derived content follow the cache too), and to revert a failed edit
//   read()            -> {value} or {error: 'Name is required.'}
//   mark(bad)         paint the control as invalid (draft rows)
//   focus()
// }
// ctx = {draft, label, row(), commit()}: in a live row the editor calls
// ctx.commit() when the person changes it; in a draft row nothing is saved
// until the row's Save, which read()s every editor.

function textEditor(col, ctx) {
  let shown = '';
  const input = el('input', {
    type: 'text', class: `cell-input ${col.inputCls || ''}`.trim(), 'aria-label': ctx.label, autocomplete: 'off', spellcheck: 'false',
    placeholder: col.placeholder ?? (ctx.draft ? col.label : null), maxlength: col.maxLength, title: col.inputTitle,
  });
  if (ctx.draft) {
    input.addEventListener('input', () => input.classList.remove('invalid'));
  } else {
    input.addEventListener('change', () => ctx.commit());
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); input.blur(); }
      else if (e.key === 'Escape') { input.value = shown; input.blur(); }
    });
  }
  return {
    nodes: [input],
    control: input,
    show(v) { shown = v ?? ''; input.value = shown; input.classList.remove('invalid'); },
    read() {
      const text = input.value.trim();
      if (!text) return col.required ? { error: `${col.label} is required.` } : { value: null };
      return { value: text };
    },
    mark(bad) { input.classList.toggle('invalid', bad); },
    focus() { input.focus(); },
  };
}

/** A status's shortcut: one character, shown as a key cap. Typing replaces what is there. */
function keyEditor(col, ctx) {
  const base = textEditor({ ...col, maxLength: 1, inputCls: 'key-input', placeholder: ctx.draft ? 'key' : '' }, ctx);
  base.control.addEventListener('focus', () => base.control.select());
  const read = base.read;
  base.read = () => {
    const r = read();
    if (r.error) return r;
    if (/\s/.test(r.value)) return { error: `${col.label} must be a visible character.` };
    return { value: r.value.toLowerCase() };
  };
  return base;
}

function checkEditor(col, ctx) {
  const box = el('input', { type: 'checkbox', 'aria-label': ctx.label, title: col.title });
  if (!ctx.draft) box.addEventListener('change', () => ctx.commit());
  return {
    nodes: [box],
    control: box,
    show(v) { box.checked = !!v; },
    read: () => ({ value: box.checked }),
    mark() {},
    focus() { box.focus(); },
  };
}

const NEW_OPTION = '\u0000new';

function selectEditor(col, ctx) {
  let current = null;
  const sel = el('select', { class: 'cell-select', 'aria-label': ctx.label });
  const picked = () => optionsOf(col).find((o) => String(o.value) === sel.value)?.value ?? null;
  function fill() {
    const opts = optionsOf(col);
    const kids = [];
    if (!col.required) kids.push(el('option', { value: '' }, '—'));
    else if (current == null) kids.push(el('option', { value: '' }, opts.length || col.create ? `${col.label}…` : 'Nothing to pick yet'));
    for (const o of opts) kids.push(el('option', { value: String(o.value) }, o.label));
    if (col.create) kids.push(el('option', { value: NEW_OPTION }, `New ${col.create.noun}…`));
    sel.replaceChildren(...kids);
    sel.value = current == null ? '' : String(current);
    sel.classList.toggle('placeholder', sel.value === '');
  }
  sel.addEventListener('change', async () => {
    sel.classList.remove('invalid');
    if (sel.value === NEW_OPTION) {
      // Make a row in the other table on the spot, then pick it.
      sel.value = current == null ? '' : String(current);
      const name = await promptText({ title: `New ${col.create.noun}`, label: 'Name', action: 'Create' });
      if (!name) { sel.focus(); return; }
      let row;
      try {
        row = await saveRef(col.options, null, { name });
      } catch (e) {
        toast(sentence(e.message));
        return;
      }
      fill();
      sel.value = String(row.id);
    }
    sel.classList.toggle('placeholder', sel.value === '');
    if (ctx.draft) current = picked(); else ctx.commit();
  });
  return {
    nodes: [sel],
    control: sel,
    show(v) { current = v ?? null; fill(); sel.classList.remove('invalid'); },
    read() {
      const v = picked();
      return v == null && col.required ? { error: `${col.label} is required.` } : { value: v };
    },
    mark(bad) { sel.classList.toggle('invalid', bad); },
    focus() { sel.focus(); },
  };
}

/** A colour swatch that opens a palette (and a "Custom…" picker for any colour). */
function colorEditor(col, ctx) {
  let value = null;
  const swatch = el('button', { type: 'button', class: 'swatch', 'aria-label': ctx.label, 'aria-haspopup': 'dialog' });
  const text = el('span', { class: 'swatch-label' });
  function paint() {
    swatch.style.background = value || 'transparent';
    swatch.classList.toggle('none', !value);
    swatch.dataset.color = value || '';
    swatch.title = value ? `Colour ${value}: click to change it` : 'Click to choose a colour';
    text.textContent = value || '';
  }
  function choose(c) {
    value = c;
    paint();
    if (!ctx.draft) ctx.commit();
  }
  swatch.addEventListener('click', () => {
    const custom = el('input', { type: 'color', class: 'swatch-custom-input', tabindex: '-1', 'aria-label': 'Custom colour', value: value || '#888888' });
    custom.addEventListener('change', () => { close(); choose(custom.value.toLowerCase()); });
    const panel = el('div', { class: 'swatch-pop', role: 'dialog', 'aria-label': ctx.label },
      el('div', { class: 'swatch-grid' }, PALETTE.map((c) => el('button', {
        type: 'button', class: 'swatch-opt', style: { background: c }, title: c, 'aria-label': c,
        'aria-pressed': value === c ? 'true' : 'false', onClick: () => { close(); choose(c); },
      }))),
      el('div', { class: 'swatch-actions' },
        el('button', { type: 'button', title: 'Any colour', onClick: () => custom.click() }, 'Custom…'),
        custom));
    const close = popover(swatch, panel);
    (panel.querySelector('[aria-pressed="true"]') || panel.querySelector('.swatch-opt')).focus();
  });
  return {
    nodes: [swatch, text],
    control: swatch,
    show(v) { value = v ?? null; paint(); },
    read: () => (value == null && col.required ? { error: `${col.label} is required.` } : { value }),
    mark() {},
    focus() { swatch.focus(); },
  };
}

function computedEditor(col) {
  const holder = el('span', { class: 'computed' });
  return {
    nodes: [holder],
    control: null,
    show(_value, row) { const out = row ? col.render(row) : null; holder.replaceChildren(...(out == null ? [] : [out])); },
  };
}

const EDITORS = { text: textEditor, key: keyEditor, check: checkEditor, select: selectEditor, color: colorEditor, computed: computedEditor };

function makeEditor(col, ctx) {
  return (EDITORS[col.kind] || textEditor)(col, ctx);
}

// ---------------------------------------------------------------- an existing row

/** One saved row as live controls: {el, row, refresh()}. */
function liveRow(spec, initial, hooks = {}) {
  let row = initial;
  let queue = Promise.resolve(); // saves of one row go to the server in order
  const name = () => spec.label(row);
  const rowEl = el('div', { class: 'row', role: 'row', dataset: { id: row.id } });

  const cells = spec.columns.map((col) => {
    const c = { col };
    c.ed = makeEditor(col, { draft: false, label: `${col.label} of ${name()}`, row: () => row, commit: () => commit(c) });
    c.cell = el('div', { class: cellClass(col, !!c.ed.control) || null, role: 'cell', dataset: { col: col.key } }, c.ed.nodes);
    return c;
  });
  const deleteBtn = iconButton('trash', { class: 'danger delete', title: 'Delete', onClick: remove });
  rowEl.append(...cells.map((c) => c.cell), el('div', { class: 'actions', role: 'cell' }, deleteBtn));

  /** Bring every control in line with `row`; the one being typed in is left alone. */
  function draw() {
    rowEl.dataset.name = name();
    for (const c of cells) {
      const control = c.ed.control;
      if (!control || control !== document.activeElement) c.ed.show(row[c.col.key], row);
      if (control) control.setAttribute('aria-label', `${c.col.label} of ${name()}`);
    }
    deleteBtn.setAttribute('aria-label', `Delete ${name()}`);
  }

  function commit(c) {
    const key = c.col.key;
    const r = c.ed.read();
    if (r.error) { toast(r.error); c.ed.show(row[key], row); return; }
    if (same(r.value, row[key])) { c.ed.show(row[key], row); return; }
    queue = queue.then(async () => {
      try {
        row = await saveRef(spec.table, row.id, { [key]: r.value });
      } catch (e) {
        toast(sentence(e.message));
        c.ed.show(row[key], row);
        return;
      }
      draw();
      flash(c.cell);
      if (hooks.onSaved) hooks.onSaved(row);
    });
  }

  async function remove() {
    if (!confirm(`Delete ${spec.noun} "${name()}"?`)) return;
    try {
      await removeRef(spec.table, row.id);
    } catch (e) {
      toast(sentence(e.message), { ms: 8000 });
      return;
    }
    rowEl.remove();
    if (hooks.onDeleted) hooks.onDeleted(row);
  }

  draw();
  return {
    el: rowEl,
    get row() { return row; },
    refresh() { row = ref.by[spec.table].get(row.id) || row; draw(); },
  };
}

// ---------------------------------------------------------------- a new (unsaved) row

/** A draft row: the same columns as empty controls, an accent bar, Save and Cancel. Enter saves, Escape cancels. */
function draftRow(spec, hooks = {}) {
  let busy = false;
  const rowEl = el('div', { class: 'row new', role: 'row' });
  const cells = spec.columns.map((col) => {
    const editable = !READ_ONLY.has(col.kind);
    const c = { col, ed: null };
    c.cell = el('div', { class: cellClass(col, editable) || null, role: 'cell', dataset: { col: col.key } });
    if (editable) {
      c.ed = makeEditor(col, { draft: true, label: `${col.label} of the new ${spec.noun}`, row: () => null, commit() {} });
      c.ed.show(resolve(col.default) ?? (col.kind === 'check' ? false : null), null);
      c.cell.append(...c.ed.nodes.filter(Boolean));
    }
    return c;
  });
  const saveBtn = iconButton('check', { class: 'primary save', title: 'Save (Enter)', 'aria-label': `Save the new ${spec.noun}`, onClick: save });
  const cancelBtn = iconButton('x', { class: 'cancel', title: 'Cancel (Escape)', 'aria-label': `Discard the new ${spec.noun}`, onClick: cancel });
  rowEl.append(...cells.map((c) => c.cell), el('div', { class: 'actions', role: 'cell' }, saveBtn, cancelBtn));

  const focus = () => cells.find((c) => c.ed && c.ed.control)?.ed.focus();

  async function save() {
    if (busy) return;
    const body = {};
    let firstBad = null;
    for (const c of cells) {
      if (!c.ed) continue;
      const r = c.ed.read();
      c.ed.mark(!!r.error);
      if (r.error) { if (!firstBad) firstBad = { c, error: r.error }; continue; }
      if (r.value != null) body[c.col.key] = r.value;
    }
    if (firstBad) { toast(firstBad.error); firstBad.c.ed.focus(); return; }
    busy = true;
    saveBtn.disabled = true;
    let row;
    try {
      row = await saveRef(spec.table, null, body);
    } catch (e) {
      toast(sentence(e.message));
      return;
    } finally {
      busy = false;
      saveBtn.disabled = false;
    }
    if (hooks.onSaved) hooks.onSaved(row);
  }

  function cancel() { if (hooks.onCancel) hooks.onCancel(); }

  // Enter saves and Escape cancels from anywhere in the row, except that a
  // <select> whose list may be showing keeps those two keys for itself.
  let listOpen = false;
  rowEl.addEventListener('mousedown', (e) => { if (e.target.tagName === 'SELECT') listOpen = true; });
  rowEl.addEventListener('change', () => { listOpen = false; });
  rowEl.addEventListener('focusout', () => { listOpen = false; });
  rowEl.addEventListener('keydown', (e) => {
    if (e.isComposing) return;
    const onSelect = e.target.tagName === 'SELECT';
    if (onSelect && (e.key === ' ' || e.key === 'F4' || (e.altKey && (e.key === 'ArrowDown' || e.key === 'ArrowUp')))) { listOpen = true; return; }
    if (e.key !== 'Escape' && e.key !== 'Enter') return;
    if (onSelect && listOpen) { listOpen = false; return; }
    if (e.key === 'Escape') { e.preventDefault(); cancel(); }
    else if (!e.target.closest('button')) { e.preventDefault(); save(); }
  });

  return { el: rowEl, focus };
}

// ---------------------------------------------------------------- a whole table

/**
 * A framed, editable table for one reference table.
 *
 *   const t = refTable(config);     see the top of this file
 *   t.el                             the element to insert
 *   t.addDraft(returnFocusTo)        open (or focus) the new-row draft, right under the header
 *   t.setFilter(text)                show only rows containing every word
 *   t.counts()                       {total, shown}
 *   t.refresh()                      redraw every row from the cache (options, computed cells)
 *
 * Rows are sorted once, when the table is built: editing a name never makes
 * a row jump, and a new row stays where its draft was until the next visit.
 */
export function refTable(config) {
  const spec = config;
  const handles = new Map(); // id -> liveRow handle
  let words = [];
  let draft = null;
  const fresh = new Set(); // rows created or edited since the filter last changed stay in view

  const header = headerRow(spec.columns);
  const note = el('div', { class: 'empty', hidden: true });
  const table = el('div', { class: 'table ref-table', role: 'table', 'aria-label': config.title || spec.table, dataset: { table: spec.table } }, header, note);
  table.style.setProperty('--cols', gridTemplate(spec.columns, config.actionsWidth));

  const changed = () => { sync(); if (config.onChange) config.onChange(); };

  function add(row) {
    const h = liveRow(spec, row, {
      onSaved(saved) { fresh.add(saved.id); changed(); },
      onDeleted(gone) { handles.delete(gone.id); fresh.delete(gone.id); changed(); },
    });
    handles.set(row.id, h);
    return h;
  }

  /** Apply the filter, and say why when nothing is left to show. */
  function sync() {
    let shown = 0;
    for (const [id, h] of handles) {
      const visible = fresh.has(id) || !words.length || (() => { const text = rowText(spec, h.row); return words.every((w) => text.includes(w)); })();
      h.el.hidden = !visible;
      if (visible) shown++;
    }
    let message = '';
    if (!shown && !draft) message = handles.size ? 'No rows match the filter.' : (config.emptyText || `No ${spec.noun}s yet.`);
    note.textContent = message;
    note.hidden = !message;
  }

  const sort = config.sort || ((a, b) => byText(spec.label(a), spec.label(b)));
  for (const row of [...ref[spec.table]].sort(sort)) note.before(add(row).el);
  sync();

  return {
    el: table,
    addDraft(returnFocusTo = null) {
      if (draft) { draft.focus(); return; }
      draft = draftRow(spec, {
        onSaved(row) {
          const h = add(row);
          fresh.add(row.id);
          draft.el.replaceWith(h.el);
          draft = null;
          changed();
          if (returnFocusTo) returnFocusTo.focus();
        },
        onCancel() { draft.el.remove(); draft = null; sync(); if (returnFocusTo) returnFocusTo.focus(); },
      });
      header.after(draft.el);
      sync();
      draft.focus();
    },
    setFilter(text) { words = fold(text).split(/\s+/).filter(Boolean); fresh.clear(); sync(); },
    counts() {
      const rows = [...handles.values()];
      return { total: rows.length, shown: rows.filter((h) => !h.el.hidden).length };
    },
    refresh() { for (const h of handles.values()) h.refresh(); sync(); },
  };
}
