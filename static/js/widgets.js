// Shared interactive widgets: popover, action menu, combo box, multi-select
// filter and modal dialog. Styles are in css/app.css.

import { el, fold } from './util.js';

// ---------------------------------------------------------------- popover

let openPopover = null;

/**
 * Show `content` in a floating panel anchored under `anchor` (flipped above
 * or shifted left when there is no room). One popover is open at a time; it
 * closes on Escape, on a click outside, or by calling the returned close().
 *
 *   const close = popover(button, panelEl, {align: 'left'|'right', flush: false, onClose, at});
 *
 * at: {x, y} places it at a point of the window (the pointer, for a context
 * menu) instead of under the anchor.
 */
export function popover(anchor, content, { align = 'left', flush = false, onClose, matchWidth = false, at = null } = {}) {
  if (openPopover) openPopover();
  const pop = el('div', { class: `popover${flush ? ' flush' : ''}` }, content);
  document.body.append(pop);

  function place() {
    const a = at ? { left: at.x, right: at.x, top: at.y, bottom: at.y, width: 0 } : anchor.getBoundingClientRect();
    if (matchWidth) pop.style.minWidth = `${a.width}px`;
    const p = pop.getBoundingClientRect();
    let left = align === 'right' ? a.right - p.width : a.left;
    left = Math.max(8, Math.min(left, window.innerWidth - p.width - 8));
    let top = a.bottom + 4;
    if (top + p.height > window.innerHeight - 8 && a.top - p.height - 4 >= 8) top = a.top - p.height - 4;
    // Room neither below nor above the anchor: keep it inside the window all the same.
    top = Math.min(top, window.innerHeight - p.height - 8);
    pop.style.left = `${left}px`;
    pop.style.top = `${Math.max(8, top)}px`;
  }
  place();

  let closed = false;
  function close() {
    if (closed) return;
    closed = true;
    pop.remove();
    document.removeEventListener('mousedown', onDown, true);
    document.removeEventListener('keydown', onKey, true);
    window.removeEventListener('resize', place);
    window.removeEventListener('scroll', place, true);
    if (openPopover === close) openPopover = null;
    if (onClose) onClose();
  }
  function onDown(e) {
    if (pop.contains(e.target) || anchor.contains(e.target)) return;
    // A nested picker (e.g. a combo list opened from inside this popover) is not "outside".
    if (e.target.closest && e.target.closest('.combo-pop')) return;
    close();
  }
  function onKey(e) {
    if (e.key === 'Escape') { e.stopPropagation(); close(); if (anchor.focus) anchor.focus(); }
  }
  document.addEventListener('mousedown', onDown, true);
  document.addEventListener('keydown', onKey, true);
  window.addEventListener('resize', place);
  window.addEventListener('scroll', place, true);
  openPopover = close;
  close.reposition = place;
  close.element = pop;
  return close;
}

/** Close the open popover, if any. */
export function closePopover() { if (openPopover) openPopover(); }

/**
 * A button that opens a menu of actions:
 *   menuButton('Export', [{label: 'CSV', onClick}, ...], {class: 'primary'})
 */
export function menuButton(label, items, attrs = {}) {
  const btn = el('button', { type: 'button', 'aria-haspopup': 'menu', ...attrs, onClick: () => {
    const close = popover(btn, el('div', { class: 'menu', role: 'menu' },
      items.map((it) => el('button', { type: 'button', role: 'menuitem', onClick: () => { close(); it.onClick(); } }, it.label))), { flush: true });
  } }, label);
  return btn;
}

// ---------------------------------------------------------------- combo box

/**
 * A text input that picks one option from a list by typing (autocomplete).
 *
 *   const c = combo({
 *     options: [{value: 3, label: 'Costco', group: 'Merchants', hint: '12 uses'}, ...]  // or () => [...]
 *                                     // an option may carry `color` (a CSS colour): shown as a dot before its label
 *     value: 3,                       // selected option's value (null = none)
 *     placeholder: 'Merchant',
 *     allowClear: true,               // emptying the box selects null
 *     onChange(value, option) {},     // called when the selection changes
 *     onCreate: async (text) => option,   // optional: offers "Create “text”" when nothing matches exactly
 *     inputClass: 'cell-input',
 *   });
 *   c.el          the element to insert
 *   c.value       current value (getter)
 *   c.setValue(v) select programmatically (no onChange)
 *   c.focus()
 *
 * Typing filters by label (case/accent-insensitive, every word must match);
 * Up/Down move, Enter or Tab picks the highlighted match, Escape restores the
 * previous selection.
 */
export function combo({ options, value = null, placeholder = '', allowClear = false, onChange, onCreate, inputClass = '', title = '' }) {
  const getOptions = typeof options === 'function' ? options : () => options;
  let current = value;
  let listEl = null, popEl = null, matches = [], active = -1, creating = false;
  let navigated = false; // an option was reached with the arrow keys since the text last changed

  const input = el('input', {
    type: 'text', class: `combo-input ${inputClass}`.trim(), placeholder, title, autocomplete: 'off', spellcheck: 'false',
    role: 'combobox', 'aria-autocomplete': 'list', 'aria-expanded': 'false',
  });
  const root = el('span', { class: 'combo' }, input);

  const labelOf = (v) => (v == null ? '' : getOptions().find((o) => o.value === v)?.label ?? '');
  const showCurrent = () => { input.value = labelOf(current); };
  showCurrent();

  function filter() {
    const words = fold(input.value).split(/\s+/).filter(Boolean);
    const all = getOptions();
    // Showing the current selection's label unedited lists everything.
    const untouched = input.value === labelOf(current);
    matches = untouched || !words.length ? all.slice() : all.filter((o) => {
      const hay = fold(`${o.label} ${o.group || ''} ${o.hint || ''}`);
      return words.every((w) => hay.includes(w));
    });
    const exact = all.some((o) => fold(o.label) === fold(input.value.trim()));
    creating = !!onCreate && input.value.trim() !== '' && !exact && !untouched;
    active = matches.length ? 0 : creating ? matches.length : -1;
    if (untouched && current != null) {
      const i = matches.findIndex((o) => o.value === current);
      if (i >= 0) active = i;
    }
    // An empty clearable box means "none": nothing is highlighted until the arrow keys choose.
    navigated = false;
    if (allowClear && input.value.trim() === '') active = -1;
  }

  function renderList() {
    if (!listEl) return;
    listEl.replaceChildren();
    let lastGroup = null;
    const LIMIT = 200;
    matches.slice(0, LIMIT).forEach((o, i) => {
      if (o.group && o.group !== lastGroup) { listEl.append(el('div', { class: 'combo-group' }, o.group)); lastGroup = o.group; }
      listEl.append(el('div', {
        class: `combo-opt${i === active ? ' active' : ''}`, role: 'option', dataset: { i },
        onMousedown: (e) => { e.preventDefault(); pick(o); },
      }, o.color ? el('span', { class: 'dot', style: { background: o.color } }) : null,
        el('span', { class: 'combo-label' }, o.label), o.hint ? el('span', { class: 'combo-hint' }, o.hint) : null));
    });
    if (matches.length > LIMIT) listEl.append(el('div', { class: 'combo-empty' }, `…and ${matches.length - LIMIT} more; keep typing`));
    if (creating) {
      listEl.append(el('div', {
        class: `combo-opt create${active === matches.length ? ' active' : ''}`, role: 'option',
        onMousedown: (e) => { e.preventDefault(); create(); },
      }, `Create “${input.value.trim()}”`));
    }
    if (!matches.length && !creating) listEl.append(el('div', { class: 'combo-empty' }, 'No matches'));
    const act = listEl.querySelector('.combo-opt.active');
    if (act) act.scrollIntoView({ block: 'nearest' });
  }

  function place() {
    if (!popEl) return;
    const a = input.getBoundingClientRect();
    popEl.style.minWidth = `${Math.max(a.width, 180)}px`;
    const p = popEl.getBoundingClientRect();
    let top = a.bottom + 2;
    if (top + p.height > window.innerHeight - 8 && a.top - p.height - 2 >= 8) top = a.top - p.height - 2;
    popEl.style.left = `${Math.max(8, Math.min(a.left, window.innerWidth - p.width - 8))}px`;
    popEl.style.top = `${top}px`;
  }

  function open() {
    if (popEl) return;
    listEl = el('div', { class: 'combo-list', role: 'listbox' });
    popEl = el('div', { class: 'popover flush combo-pop' }, listEl);
    document.body.append(popEl);
    input.setAttribute('aria-expanded', 'true');
    window.addEventListener('scroll', place, true);
    window.addEventListener('resize', place);
    filter(); renderList(); place();
  }
  function close() {
    if (!popEl) return;
    popEl.remove(); popEl = null; listEl = null;
    input.setAttribute('aria-expanded', 'false');
    window.removeEventListener('scroll', place, true);
    window.removeEventListener('resize', place);
  }

  function commit(v, option) {
    const changed = v !== current;
    current = v;
    showCurrent();
    close();
    if (changed && onChange) onChange(v, option || null);
  }
  function pick(o) { commit(o.value, o); }
  async function create() {
    const text = input.value.trim();
    close();
    try {
      const o = await onCreate(text);
      if (o) commit(o.value, o);
      else { showCurrent(); if (popEl) { filter(); renderList(); place(); } } // the list may have reopened meanwhile, still offering to create
    } catch (e) {
      showCurrent();
      throw e;
    }
  }

  input.addEventListener('focus', () => { input.select(); open(); });
  input.addEventListener('click', () => open());
  input.addEventListener('input', () => { open(); filter(); renderList(); place(); });
  input.addEventListener('keydown', (e) => {
    const n = matches.length + (creating ? 1 : 0);
    if (e.key === 'ArrowDown') { e.preventDefault(); if (!popEl) open(); else if (n) { active = (active + 1) % n; navigated = true; renderList(); } }
    else if (e.key === 'ArrowUp') { e.preventDefault(); if (popEl && n) { active = active < 0 ? n - 1 : (active - 1 + n) % n; navigated = true; renderList(); } }
    else if (e.key === 'Enter' || e.key === 'Tab') {
      if (!popEl) return;
      if (allowClear && input.value.trim() === '' && !navigated) { if (e.key === 'Enter') e.preventDefault(); commit(null); return; }
      // Tabbing out of an emptied box keeps the selection (it must not pick the first option).
      if (e.key === 'Tab' && input.value.trim() === '' && !navigated) { showCurrent(); close(); return; }
      if (active >= 0 && active < matches.length) {
        if (e.key === 'Enter') e.preventDefault();
        pick(matches[active]);
      } else if (creating && active === matches.length) {
        if (e.key === 'Enter') e.preventDefault();
        create().catch(() => {});
      } else { showCurrent(); close(); }
    } else if (e.key === 'Escape') {
      if (popEl) { e.stopPropagation(); showCurrent(); close(); }
    }
  });
  input.addEventListener('blur', () => {
    if (!popEl) return;
    if (allowClear && input.value.trim() === '') { commit(null); return; }
    // A name typed out in full counts as picked, even when the next thing touched is a button.
    const typed = fold(input.value.trim());
    const exact = typed && input.value !== labelOf(current) ? getOptions().filter((o) => fold(o.label) === typed) : [];
    if (exact.length === 1) pick(exact[0]);
    else { showCurrent(); close(); }
  });

  return {
    el: root,
    input,
    get value() { return current; },
    setValue(v) { current = v; showCurrent(); },
    focus() { input.focus(); },
    refresh() { showCurrent(); },
  };
}

// ---------------------------------------------------------------- multi-select filter

/**
 * A filter button that opens a searchable checklist.
 *
 *   const m = multiSelect({
 *     label: 'Account',
 *     options: [{value: 1, label: 'USAA Checking', group: 'Checking'}, ...]   // or () => [...]
 *     selected: new Set([1]),      // empty set = no filter (everything passes)
 *     onChange(selectedSet) {},
 *   });
 *   m.el, m.selected (Set), m.setSelected(iterable), m.clear()
 *
 * The button shows the label and a badge with the number picked.
 */
export function multiSelect({ label, options, selected = new Set(), onChange, align = 'left' }) {
  const getOptions = typeof options === 'function' ? options : () => options;
  let sel = new Set(selected);
  const badge = el('span', { class: 'badge' });
  const btn = el('button', { type: 'button', class: 'multi-btn', 'aria-haspopup': 'true' }, label, badge);

  function sync() {
    badge.textContent = sel.size || '';
    badge.hidden = sel.size === 0;
    btn.classList.toggle('on', sel.size > 0);
    const names = getOptions().filter((o) => sel.has(o.value)).map((o) => o.label);
    btn.title = names.length ? `${label}: ${names.join(', ')}` : `Filter by ${label.toLowerCase()}`;
  }
  sync();

  btn.addEventListener('click', () => {
    const search = el('input', { type: 'search', class: 'multi-search', placeholder: `Search ${label.toLowerCase()}…` });
    const list = el('div', { class: 'multi-list' });
    const fire = () => { sync(); if (onChange) onChange(new Set(sel)); };
    function visible() {
      const words = fold(search.value).split(/\s+/).filter(Boolean);
      return getOptions().filter((o) => { const hay = fold(`${o.label} ${o.group || ''}`); return words.every((w) => hay.includes(w)); });
    }
    function render() {
      list.replaceChildren();
      let lastGroup = null;
      const opts = visible();
      for (const o of opts) {
        if (o.group && o.group !== lastGroup) { list.append(el('div', { class: 'multi-group' }, o.group)); lastGroup = o.group; }
        const box = el('input', { type: 'checkbox', checked: sel.has(o.value), onChange: () => { if (box.checked) sel.add(o.value); else sel.delete(o.value); fire(); } });
        list.append(el('label', { class: 'multi-opt', title: o.label }, box,
          o.color ? el('span', { class: 'dot', style: { background: o.color, marginRight: '0' } }) : null, el('span', {}, o.label)));
      }
      if (!opts.length) list.append(el('div', { class: 'combo-empty' }, 'No matches'));
    }
    search.addEventListener('input', render);
    const panel = el('div', { class: 'multi-panel' },
      search,
      el('div', { class: 'multi-actions' },
        el('button', { type: 'button', onClick: () => { for (const o of visible()) sel.add(o.value); render(); fire(); } }, 'All'),
        el('button', { type: 'button', onClick: () => { if (search.value) { for (const o of visible()) sel.delete(o.value); } else sel.clear(); render(); fire(); } }, 'None')),
      list);
    render();
    popover(btn, panel, { align });
    search.focus();
  });

  return {
    el: btn,
    get selected() { return sel; },
    setSelected(values) { sel = new Set(values); sync(); },
    clear() { sel = new Set(); sync(); },
  };
}

// ---------------------------------------------------------------- modal dialog

const openModals = []; // the close functions of the dialogs showing, bottom to top

/** Close every open dialog (on leaving the page they belong to). */
export function closeModals() { for (const close of [...openModals].reverse()) close(); }

/** Is a dialog or a popover (menu, picker) showing? Page-level keys stand back while one is. */
export const overlayOpen = () => openModals.length > 0 || !!document.querySelector('.popover');

/**
 * A modal dialog.
 *
 *   const m = modal({title: 'New account', body: formEl, wide: false,
 *     actions: [{label: 'Cancel'}, {label: 'Save', primary: true, onClick: async () => { ...; return true; }}]});
 *
 * An action without onClick closes the dialog. An onClick that returns (or
 * resolves to) false keeps it open (e.g. validation failed); a thrown error
 * also keeps it open and is re-thrown to the caller's handler. Escape and the
 * × close it. m.close() closes it; m.el is the dialog element.
 */
export function modal({ title, body, actions = [{ label: 'Close' }], wide = false, onClose }) {
  closePopover();
  const foot = el('div', { class: 'modal-foot' });
  const dialog = el('div', { class: `modal${wide ? ' wide' : ''}`, role: 'dialog', 'aria-modal': 'true', 'aria-label': title },
    el('div', { class: 'modal-head' }, el('h2', {}, title),
      el('button', { type: 'button', class: 'bare icon', 'aria-label': 'Close', title: 'Close', onClick: () => close() }, '×')),
    el('div', { class: 'modal-body' }, body),
    foot);
  const backdrop = el('div', { class: 'modal-backdrop' }, dialog);
  const previous = document.activeElement;

  let closed = false;
  function close() {
    if (closed) return;
    closed = true;
    backdrop.remove();
    document.removeEventListener('keydown', onKey, true);
    const i = openModals.indexOf(close);
    if (i >= 0) openModals.splice(i, 1);
    if (previous && previous.focus) previous.focus();
    if (onClose) onClose();
  }
  function onKey(e) {
    // Escape closes the dialog on top only (a dialog can open another).
    if (e.key !== 'Escape' || openModals[openModals.length - 1] !== close) return;
    if (!document.querySelector('.popover')) { e.stopImmediatePropagation(); close(); }
  }
  openModals.push(close);
  for (const a of actions) {
    if (a.spacer) { foot.append(el('span', { class: 'spacer' })); continue; }
    const btn = el('button', { type: 'button', class: [a.primary ? 'primary' : '', a.danger ? 'danger' : ''].join(' ').trim() || null, onClick: async () => {
      if (!a.onClick) { close(); return; }
      btn.disabled = true;
      try {
        const keep = (await a.onClick()) === false;
        if (!keep) close();
      } finally {
        btn.disabled = false;
      }
    } }, a.label);
    foot.append(btn);
  }
  backdrop.addEventListener('mousedown', (e) => { if (e.target === backdrop) close(); });
  document.addEventListener('keydown', onKey, true);
  document.body.append(backdrop);
  // Focus starts in the first field; a dialog without one starts on its
  // first action (never on a button in the body, which may be the very
  // thing being confirmed), so the keys do not reach what is behind it.
  const first = dialog.querySelector('.modal-body input, .modal-body select, .modal-body textarea') || foot.querySelector('button');
  if (first) first.focus();
  return { el: dialog, close };
}
