// The "New list" dialog: a name, the template flag, and what to start from:
// nothing (an empty list), one list (a copy) or several, combined (everything
// that is on any of them). The server does the combining
// (internal/store/merge.go); the dialog asks it how many entries the result
// would have and says so before the list is made.

import { api } from './api.js';
import { defaultStatus } from './state.js';
import { el, toast, sentence, plural, byText } from './util.js';
import { modal } from './widgets.js';

/**
 * Open the dialog.
 *
 *   openNewList({
 *     lists,                 every list (as GET /api/lists returns them)
 *     sources: [3],          lists ticked to begin with (several are combined)
 *     name: 'Copy of …',     the name to begin with
 *     onCreated(list) {},    called with the new list (and its contents)
 *   });
 */
export function openNewList({ lists, sources = [], name = '', onCreated }) {
  let picked = sources.filter((id) => lists.some((l) => l.id === id)); // in the order ticked: the first one wins disagreements
  let preview = null; // how many entries the current `picked` would give; null while that is being asked
  let previewSeq = 0;

  const nameIn = el('input', { type: 'text', id: 'new-list-name', value: name, maxlength: 127, autocomplete: 'off', placeholder: 'e.g. Paris 2027' });
  const templateBox = el('input', { type: 'checkbox', id: 'new-list-template' });
  const sourceList = el('div', { class: 'source-list', role: 'group', 'aria-label': 'Lists to start from' });
  const summary = el('p', { class: 'hint new-list-summary', 'aria-live': 'polite' });

  const byId = new Map(lists.map((l) => [l.id, l]));
  const groups = [
    ['Templates', lists.filter((l) => l.is_template)],
    ['Lists', lists.filter((l) => !l.is_template)],
  ].filter(([, rows]) => rows.length);

  function renderSources() {
    sourceList.replaceChildren();
    if (!lists.length) { sourceList.append(el('div', { class: 'combo-empty' }, 'There are no other lists yet.')); return; }
    for (const [title, rows] of groups) {
      sourceList.append(el('div', { class: 'multi-group' }, title));
      for (const l of [...rows].sort((a, b) => byText(a.name, b.name))) {
        const order = picked.indexOf(l.id);
        const box = el('input', { type: 'checkbox', checked: order >= 0, dataset: { list: l.id }, onChange: () => {
          if (box.checked) picked.push(l.id); else picked = picked.filter((id) => id !== l.id);
          renderSources(); refreshPreview();
        } });
        sourceList.append(el('label', { class: 'multi-opt source-opt' }, box,
          el('span', { class: 'source-name' }, l.name),
          order >= 0 && picked.length > 1 ? el('span', { class: 'badge', title: 'The order the lists were ticked in: where two lists put a container in different places, the earlier one wins' }, order + 1) : null,
          el('span', { class: 'combo-hint' }, plural(l.rows, 'entry', 'entries'))));
      }
    }
  }

  function renderSummary() {
    const def = defaultStatus();
    const count = preview == null ? '' : `: ${plural(preview, 'entry', 'entries')}`;
    let text;
    if (!picked.length) text = 'Nothing ticked: the list starts empty.';
    else if (picked.length === 1) text = `A copy of “${byId.get(picked[0]).name}”${count}.`;
    else text = `Everything that is on any of the ${picked.length} lists${count}. Where they put a container in different places, the list ticked first wins.`;
    const anyPlain = picked.some((id) => !byId.get(id).is_template);
    const anyTemplate = picked.some((id) => byId.get(id).is_template);
    const notes = [];
    if (anyTemplate) notes.push('Entries from a template keep the status the template gives them.');
    if (anyPlain && def) notes.push(`Entries from ${anyTemplate ? 'the other lists' : 'a list that is not a template'} start as “${def.name}”.`);
    summary.replaceChildren(text, notes.length ? el('br') : null, notes.join(' '));
  }

  async function refreshPreview() {
    const seq = ++previewSeq;
    preview = null;
    renderSummary();
    if (!picked.length) return;
    try {
      const p = await api.previewList(picked);
      if (seq !== previewSeq) return;
      preview = p.rows;
    } catch (e) {
      if (seq === previewSeq) toast(sentence(e.message));
      return;
    }
    renderSummary();
  }

  let creating = false;
  async function create() {
    const listName = nameIn.value.trim();
    if (!listName) { nameIn.classList.add('invalid'); nameIn.focus(); return false; }
    if (creating) return false; // Enter pressed twice: the first is still on its way
    creating = true;
    let list;
    try {
      list = await api.createList({ name: listName, is_template: templateBox.checked, sources: picked });
    } catch (e) {
      toast(sentence(e.message));
      if (e.status === 409) { nameIn.classList.add('invalid'); nameIn.focus(); }
      return false;
    } finally {
      creating = false;
    }
    if (onCreated) onCreated(list);
    return true;
  }

  const m = modal({
    title: 'New list',
    body: [
      el('div', { class: 'field' }, el('label', { for: 'new-list-name' }, 'Name'), nameIn),
      el('label', { class: 'check' }, templateBox, el('span', {}, 'Template'), el('span', { class: 'muted' }, '(a list meant to be copied from, such as “Camping Weekend”)')),
      el('div', { class: 'field' }, el('div', { class: 'field-label' }, 'Start from'), sourceList),
      summary,
    ],
    actions: [{ label: 'Cancel' }, { label: 'Create list', primary: true, onClick: create }],
  });
  m.el.classList.add('new-list');
  nameIn.addEventListener('input', () => nameIn.classList.remove('invalid'));
  nameIn.addEventListener('keydown', async (e) => {
    if (e.key === 'Enter') { e.preventDefault(); if (await create()) m.close(); }
  });

  renderSources(); renderSummary();
  refreshPreview();
  nameIn.focus();
  nameIn.select();
  return m;
}
