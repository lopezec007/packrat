// Lists page (#/lists): every list, the ordinary ones first and the
// templates after them, each with how many entries it has and how they split
// between the statuses. From here a list is opened, renamed, flagged as a
// template, copied or deleted, and new ones are made (newlist.js).
//
// (A template is a list like any other; the flag only says it is meant to be
// copied from, so it is kept apart here and offered first in "New list".)

import { api } from '../api.js';
import { el, toast, sentence, plural, byText, fold } from '../util.js';
import { icon } from '../icons.js';
import { statusBar } from '../status.js';
import { openNewList } from '../newlist.js';

export async function mount(ctx) {
  let lists = await api.lists();
  let words = [];
  let renaming = null; // id of the list whose name is being edited

  const filter = el('input', { type: 'search', class: 'filter-box', placeholder: 'Filter lists…', 'aria-label': 'Filter lists' });
  filter.addEventListener('input', () => { words = fold(filter.value).split(/\s+/).filter(Boolean); render(); });
  const newBtn = el('button', { type: 'button', class: 'primary', id: 'btn-new-list', onClick: () => newList() }, icon('plus', { size: 14 }), 'New list');
  ctx.toolbar.append(filter, el('span', { class: 'spacer' }), newBtn);

  const body = el('div', { class: 'table-wrap constrained lists-wrap' });
  ctx.main.append(body);

  function newList(sources = [], name = '') {
    openNewList({ lists, sources, name, onCreated: (list) => ctx.navigate(`#/list?id=${list.id}`) });
  }

  async function reload() {
    try { lists = await api.lists(); } catch (e) { toast(sentence(e.message)); }
    render();
  }

  async function patch(list, body) {
    try {
      await api.updateList(list.id, body);
    } catch (e) {
      toast(sentence(e.message));
      return false;
    }
    await reload();
    return true;
  }

  async function remove(list) {
    const what = list.rows ? ` and its ${plural(list.rows, 'entry', 'entries')}` : '';
    if (!confirm(`Delete the list "${list.name}"${what}?\n\nThe items themselves stay in the library.`)) return;
    try {
      await api.deleteList(list.id);
    } catch (e) {
      toast(sentence(e.message));
      return;
    }
    await reload();
  }

  function nameCell(list) {
    if (renaming !== list.id) {
      return el('div', { class: 'name' },
        el('a', { class: 'list-link', href: `#/list?id=${list.id}`, title: `Open ${list.name}` }, list.name));
    }
    const input = el('input', { type: 'text', class: 'cell-input', value: list.name, maxlength: 127, 'aria-label': `Name of ${list.name}` });
    let done = false;
    const cell = el('div', { class: 'name ed' }, input);
    const finish = async (save) => {
      if (done) return;
      done = true;
      const text = input.value.trim();
      renaming = null;
      if (save && text && text !== list.name) { if (await patch(list, { name: text })) return; }
      // Unchanged: put the name back in place. (Redrawing the table here would
      // swallow the click that took the focus away, on whatever it was aimed at.)
      if (cell.isConnected) cell.replaceWith(nameCell(list)); else render();
    };
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); finish(true); }
      else if (e.key === 'Escape') { e.preventDefault(); finish(false); }
    });
    input.addEventListener('blur', () => finish(true));
    queueMicrotask(() => { input.focus(); input.select(); });
    return cell;
  }

  function row(list) {
    const template = el('input', { type: 'checkbox', checked: list.is_template, 'aria-label': `${list.name} is a template`,
      title: 'A template is a list meant to be copied from', onChange: () => patch(list, { is_template: template.checked }) });
    return el('div', { class: 'row', role: 'row', dataset: { id: list.id, name: list.name } },
      nameCell(list),
      el('div', { class: 'center' }, template),
      el('div', { class: 'num' }, list.rows ? list.rows : el('span', { class: 'muted' }, '0')),
      el('div', { class: 'bar-cell' }, statusBar(list.status_counts)),
      el('div', { class: 'actions' },
        el('button', { type: 'button', class: 'rename', title: 'Rename', 'aria-label': `Rename ${list.name}`, onClick: () => { renaming = list.id; render(); } }, icon('edit', { size: 14 })),
        el('button', { type: 'button', class: 'copy', title: 'New list from this one…', 'aria-label': `New list from ${list.name}`, onClick: () => newList([list.id], list.is_template ? '' : `Copy of ${list.name}`) }, icon('copy', { size: 14 })),
        el('button', { type: 'button', class: 'danger delete', title: 'Delete', 'aria-label': `Delete ${list.name}`, onClick: () => remove(list) }, icon('trash', { size: 14 }))));
  }

  function render() {
    if (!lists.length) {
      body.replaceChildren(el('div', { class: 'card lists-intro' },
        el('h2', {}, 'No lists yet'),
        el('p', {}, 'A list is what you pack for one trip or outing: the items, how many of each, and the containers they go in, nested as deep as you like (a car holding a backpack holding a toiletry kit).'),
        el('p', {}, 'Lists draw their items from the ', el('a', { href: '#/library' }, 'Library'), ', so it helps to add a few items there first; you can also create items as you go.'),
        el('p', {}, el('button', { type: 'button', class: 'primary', onClick: () => newList() }, icon('plus', { size: 14 }), 'New list'))));
      return;
    }
    const match = (l) => { const text = fold(l.name); return words.every((w) => text.includes(w)); };
    const table = el('div', { class: 'table lists-table', role: 'table', 'aria-label': 'Lists' },
      el('div', { class: 'row header', role: 'row' },
        el('div', {}, 'Name'), el('div', { class: 'center', title: 'A template is a list meant to be copied from' }, 'Template'),
        el('div', { class: 'num' }, 'Entries'), el('div', { class: 'bar-cell' }, 'Status'), el('div', { class: 'actions', 'aria-label': 'Actions' })));
    let shown = 0;
    for (const [title, rows, empty] of [
      ['Lists', lists.filter((l) => !l.is_template), 'No lists yet: make one with “New list”, from scratch or from templates.'],
      ['Templates', lists.filter((l) => l.is_template), 'No templates yet: tick Template on a list you want to copy from.'],
    ]) {
      const visible = rows.filter(match).sort((a, b) => byText(a.name, b.name));
      table.append(el('div', { class: 'row group-row', role: 'row' },
        el('div', { style: { gridColumn: '1 / -1' } }, title, el('span', { class: 'muted group-count' }, rows.length))));
      if (!rows.length) table.append(el('div', { class: 'empty group-empty' }, empty));
      for (const l of visible) { table.append(row(l)); shown++; }
    }
    if (!shown && words.length) table.append(el('div', { class: 'empty' }, 'No lists match the filter.'));
    body.replaceChildren(table);
  }

  render();
}
