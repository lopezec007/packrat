// Creating a library item without leaving the list: the "New item" dialog
// that the add-entry row opens when the name typed is not in the library yet.

import { ref, saveRef, sortedRows } from '../state.js';
import { el, toast, sentence } from '../util.js';
import { modal } from '../widgets.js';
import { promptText } from '../refedit.js';

const NEW = '\u0000new';

/**
 * A <select> over a reference table with a "New …" choice at the end that
 * creates a row on the spot. remember: the prefs-like object {get, set} that
 * keeps the last choice, so a run of new items starts where the last one was.
 */
function refSelect(table, noun, label, initial) {
  let current = initial ?? null;
  const sel = el('select', { 'aria-label': label });
  function fill() {
    sel.replaceChildren(
      current == null ? el('option', { value: '' }, `${label}…`) : null,
      ...sortedRows(table).map((r) => el('option', { value: String(r.id) }, r.name)),
      el('option', { value: NEW }, `New ${noun}…`));
    sel.value = current == null ? '' : String(current);
  }
  sel.addEventListener('change', async () => {
    sel.classList.remove('invalid');
    if (sel.value !== NEW) { current = sel.value === '' ? null : Number(sel.value); return; }
    sel.value = current == null ? '' : String(current);
    const name = await promptText({ title: `New ${noun}`, label: 'Name', action: 'Create' });
    if (name) {
      try {
        current = (await saveRef(table, null, { name })).id;
      } catch (e) {
        toast(sentence(e.message));
      }
    }
    fill();
    sel.focus();
  });
  fill();
  return { el: sel, get value() { return current; }, mark(bad) { sel.classList.toggle('invalid', bad); } };
}

let lastCategory = null;
let lastLocation = null;

/**
 * Ask for the rest of a new library item (its category, where it is stored,
 * whether it is a container) and create it. Resolves to the item, or null
 * when the dialog is dismissed.
 */
export function newItemDialog(name = '') {
  return new Promise((done) => {
    let created = null;
    const nameIn = el('input', { type: 'text', id: 'new-item-name', value: name, maxlength: 127, autocomplete: 'off' });
    const category = refSelect('item_categories', 'category', 'Category', ref.by.item_categories.has(lastCategory) ? lastCategory : null);
    const location = refSelect('storage_locations', 'storage location', 'Stored in', ref.by.storage_locations.has(lastLocation) ? lastLocation : null);
    const container = el('input', { type: 'checkbox', id: 'new-item-container' });

    let busy = false;
    async function create() {
      if (busy) return false;
      const text = nameIn.value.trim();
      nameIn.classList.toggle('invalid', !text);
      category.mark(category.value == null);
      location.mark(location.value == null);
      if (!text) { nameIn.focus(); return false; }
      if (category.value == null) { toast('Pick a category (or create one from the end of the list).'); category.el.focus(); return false; }
      if (location.value == null) { toast('Pick where the item is stored (or create a place from the end of the list).'); location.el.focus(); return false; }
      busy = true;
      try {
        created = await saveRef('items', null, {
          name: text, item_category_id: category.value, storage_location_id: location.value, is_container: container.checked,
        });
      } catch (e) {
        toast(sentence(e.message));
        if (e.status === 409) { nameIn.classList.add('invalid'); nameIn.focus(); }
        return false;
      } finally {
        busy = false;
      }
      lastCategory = category.value;
      lastLocation = location.value;
      return true;
    }

    const m = modal({
      title: 'New item',
      body: [
        el('p', { class: 'hint' }, 'This adds the item to the library, then to the list.'),
        el('div', { class: 'field' }, el('label', { for: 'new-item-name' }, 'Name'), nameIn),
        el('div', { class: 'form-grid' },
          el('div', { class: 'field' }, el('div', { class: 'field-label' }, 'Category'), category.el),
          el('div', { class: 'field' }, el('div', { class: 'field-label' }, 'Stored in'), location.el)),
        el('label', { class: 'check' }, container, el('span', {}, 'Container'), el('span', { class: 'muted' }, '(other items can be packed inside it)')),
      ],
      actions: [{ label: 'Cancel' }, { label: 'Create item', primary: true, onClick: create }],
      onClose: () => done(created),
    });
    m.el.classList.add('new-item');
    nameIn.addEventListener('input', () => nameIn.classList.remove('invalid'));
    nameIn.addEventListener('keydown', async (e) => { if (e.key === 'Enter') { e.preventDefault(); if (await create()) m.close(); } });
    // The name is already typed: start on the first thing still to fill in.
    (name ? category.el : nameIn).focus();
  });
}
