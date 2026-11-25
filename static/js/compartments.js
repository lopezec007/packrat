// The compartments of a container (a backpack's lid pocket, a car's boot):
// a dialog that lists them, lets each be renamed or deleted, and adds new
// ones. Opened from the Library's Items table.
//
// On a list, an entry that is in a container can be put in one of the
// container's compartments. Deleting a compartment leaves what was in it in
// the container, in no compartment.

import { saveRef, removeRef, compartmentsOf } from './state.js';
import { el, toast, sentence, plural } from './util.js';
import { icon } from './icons.js';
import { modal } from './widgets.js';

/**
 * Open the editor for an item's compartments; resolves when it is closed.
 *
 *   await compartmentsDialog(item);
 */
export function compartmentsDialog(item) {
  return new Promise((done) => {
    const list = el('div', { class: 'comp-list' });
    const newName = el('input', { type: 'text', class: 'comp-new', maxlength: 127, autocomplete: 'off', placeholder: 'New compartment, e.g. Lid Pocket', 'aria-label': 'Name of the new compartment' });
    const addBtn = el('button', { type: 'button', class: 'primary comp-add', onClick: add }, icon('plus', { size: 14 }), 'Add');

    function row(c) {
      const input = el('input', { type: 'text', class: 'comp-name', value: c.name, maxlength: 127, autocomplete: 'off', 'aria-label': `Name of the compartment ${c.name}` });
      input.addEventListener('change', async () => {
        const name = input.value.trim();
        if (!name || name === c.name) { input.value = c.name; return; }
        try {
          c = await saveRef('compartments', c.id, { name });
        } catch (e) {
          toast(sentence(e.message));
        }
        input.value = c.name;
      });
      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') { e.preventDefault(); input.blur(); newName.focus(); }
        else if (e.key === 'Escape') { e.stopPropagation(); input.value = c.name; input.blur(); }
      });
      const used = c.use_count ? plural(c.use_count, 'entry', 'entries') : '';
      return el('div', { class: 'comp-row', dataset: { name: c.name } }, input,
        el('span', { class: 'muted comp-used', title: used ? 'List entries that are in this compartment' : null }, used),
        el('button', { type: 'button', class: 'bare icon danger comp-delete', title: 'Delete', 'aria-label': `Delete the compartment ${c.name}`, onClick: () => remove(c) }, icon('trash', { size: 14 })));
    }

    function render() {
      const all = compartmentsOf(item.id);
      list.replaceChildren(...(all.length ? all.map(row) : [el('p', { class: 'muted comp-none' }, 'No compartments yet.')]));
    }

    async function add() {
      const name = newName.value.trim();
      if (!name) { newName.focus(); return; }
      try {
        await saveRef('compartments', null, { item_id: item.id, name });
      } catch (e) {
        toast(sentence(e.message));
        newName.classList.add('invalid');
        newName.focus();
        return;
      }
      newName.value = '';
      render();
      newName.focus();
    }

    async function remove(c) {
      if (c.use_count && !confirm(`Delete the compartment "${c.name}"?\n\n${plural(c.use_count, 'entry', 'entries')} on lists ${c.use_count === 1 ? 'is' : 'are'} in it; ${c.use_count === 1 ? 'it stays' : 'they stay'} in ${item.name}, in no compartment.`)) return;
      try {
        await removeRef('compartments', c.id);
      } catch (e) {
        toast(sentence(e.message));
        return;
      }
      render();
    }

    newName.addEventListener('input', () => newName.classList.remove('invalid'));
    newName.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); add(); } });
    render();
    const m = modal({
      title: `Compartments of ${item.name}`,
      body: [
        el('p', { class: 'hint' }, `The parts of ${item.name} that things can be packed into. On a list, anything inside ${item.name} can be put in one of them (or left in none).`),
        list,
        el('div', { class: 'comp-add-row' }, newName, addBtn),
      ],
      actions: [{ label: 'Done', primary: true }],
      onClose: () => done(),
    });
    m.el.classList.add('compartments');
    newName.focus();
  });
}
