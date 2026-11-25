// One list (#/list?id=3): its containers as boxes within boxes, the items
// packed into each listed inside it, everything tinted by status
// (list/boxes.js draws it).
//
// This module is the controller. It owns the state (the entries, the
// selection, which containers are closed, the sort order and filter), every
// change to the list (which it sends to the server and applies to the
// state), and the keyboard: the view says where an arrow key leads, and a
// status's shortcut key gives the selected entry that status. A container's
// status is its own: nothing here derives it from what is inside.
//
// The view is {el, render(), showSelection(reveal), key(event) -> handled,
// updateRow(id), focusDraft(), destroy()}.
//
// URL parameter: id (the list). The sort (list/sort.js) and whether the category
// and storage location are shown are remembered per browser; the closed
// containers per browser and list.
//
// Printing (the ⋯ menu, or the browser's own Print) prints what is on screen:
// see @media print in css/list.css.

import { api } from '../api.js';
import { ref, reloadRef, statuses, statusForKey, compartmentsOf } from '../state.js';
import { el, toast, sentence, plural, fold, typingTarget, prefs, byText } from '../util.js';
import { icon } from '../icons.js';
import { menuButton, multiSelect, modal, popover, overlayOpen } from '../widgets.js';
import { promptText } from '../refedit.js';
import { statusChip, countByStatus } from '../status.js';
import { buildTree, applyFilter, visibleNodes, isWithin } from '../list/model.js';
import { normalizeSort, sortButton } from '../list/sort.js';
import { boxesView } from '../list/boxes.js';
import { openNewList } from '../newlist.js';

export async function mount(ctx) {
  const id = Number(ctx.params.get('id'));
  let lists, detail;
  try {
    [lists, detail] = await Promise.all([api.lists(), api.list(id), reloadRef()]);
  } catch (e) {
    ctx.main.append(el('p', { class: 'empty' },
      e.status === 404 ? 'There is no such list (it may have been deleted). ' : `${sentence(e.message)}. `,
      el('a', { href: '#/lists' }, 'Back to the lists')));
    return undefined;
  }

  // ---------------------------------------------------------------- state

  const st = {
    list: { id: detail.id, name: detail.name, is_template: detail.is_template },
    rows: detail.contents,
    tree: null,
    visible: [],          // the nodes that are drawn, in reading order
    selected: null,       // an entry's id
    collapsed: new Set(prefs.get(`collapsed.${id}`, [])), // item ids of the containers that are closed
    sort: normalizeSort(prefs.get('list.sort')), // [{key, dir}] in order; empty = by name (list/sort.js)
    words: [],
    statusFilter: new Set(),
    filtering: false,
    matches: 0,
    adding: null,         // {container: item id | null, compartment: id | null} while the add-entry row is open
    showMeta: prefs.get('list.meta', true) !== false, // each entry's category and storage location are shown
  };
  ctx.setTitle(st.list.name);

  const rowById = (rowId) => st.rows.find((r) => r.id === rowId);
  const saveCollapsed = () => prefs.set(`collapsed.${id}`, [...st.collapsed]);

  function rebuild() {
    const before = st.visible.map((n) => n.id); // the order the entries were in view, to find the selection a new home
    st.tree = buildTree(st.rows, st.sort);
    st.filtering = st.words.length > 0 || st.statusFilter.size > 0;
    st.matches = applyFilter(st.tree, { words: st.words, statuses: st.statusFilter });
    st.visible = visibleNodes(st.tree, st.collapsed, st.filtering);
    // The add-entry row needs its place to still be there.
    if (st.adding && st.adding.container != null && !st.tree.holder.has(st.adding.container)) st.adding = null;
    if (st.adding && st.adding.compartment != null && ref.by.compartments.get(st.adding.compartment)?.item_id !== st.adding.container) st.adding.compartment = null;
    reselect(before);
  }

  /**
   * Keep the selection on an entry that is drawn, so a status key never
   * changes something out of sight: an entry inside a container that is now
   * closed hands the selection to that container; one that is gone, or no
   * longer passes the filter, to the entry that came after it (or before).
   */
  function reselect(before) {
    if (st.selected == null) return;
    const inView = new Set(st.visible.map((n) => n.id));
    if (!inView.has(st.selected)) {
      const node = st.tree.nodes.get(st.selected);
      let home = null;
      if (node && node.show) for (let p = node.parent; p && home == null; p = p.parent) if (inView.has(p.id)) home = p.id;
      if (home == null) {
        const i = before.indexOf(st.selected);
        if (i >= 0) home = before.slice(i + 1).find((x) => inView.has(x)) ?? before.slice(0, i).reverse().find((x) => inView.has(x)) ?? null;
      }
      st.selected = home;
    }
  }

  // Changes go to the server one at a time, in the order they were made, so
  // their answers come back in that order too. `writes` counts every change
  // started or finished: refresh() uses it to tell that what it read is stale.
  let queue = Promise.resolve();
  let writes = 0;
  async function tracked(fn) {
    writes++;
    try { return await fn(); } finally { writes++; }
  }
  function enqueue(fn) {
    const run = queue.then(() => tracked(fn));
    queue = run.catch(() => {});
    return run;
  }
  // Status changes on their way, per entry: {confirmed: the status the server last agreed to, n: answers awaited}.
  const pendingStatus = new Map();

  // ---------------------------------------------------------------- elements

  const legend = el('div', { class: 'legend-row', role: 'group', 'aria-label': 'Statuses' });
  const viewHolder = el('div', { class: 'list-view' });
  // The heading of the printed page (on screen the title bar says all this).
  const printTitle = el('div', { class: 'print-only print-title' });
  ctx.main.append(printTitle, viewHolder);

  let theView; // made below, once `act` exists
  const view = () => theView;

  function render() {
    rebuild();
    renderLegend();
    syncToolbar();
    view().render();
  }

  /** The statuses with their keys and counts; clicking one gives it to the selected entry. */
  function renderLegend() {
    const counts = countByStatus(st.rows);
    const selected = st.selected != null ? st.tree.nodes.get(st.selected) : null;
    legend.replaceChildren(
      ...statuses().map((s) => statusChip(s, {
        key: true, count: counts[s.id] || 0,
        title: selected ? `Give “${selected.name}” the status ${s.name} (key ${s.shortcut.toUpperCase()})` : `${s.name}: select an entry, then press ${s.shortcut.toUpperCase()} (or click here)`,
        onClick: () => { if (st.selected != null) act.setStatus(st.selected, s.id); else toast('Select an entry first.', { info: true, ms: 2500 }); },
      })),
      el('span', { class: 'legend-total muted', title: st.filtering ? 'Entries that pass the filter, of all the entries on the list' : 'Entries on the list' }, st.filtering
        ? `${st.matches} of ${plural(st.rows.length, 'entry', 'entries')}`
        : plural(st.rows.length, 'entry', 'entries')));
    fitLegend();
  }

  /**
   * Make the legend fit the one line of the title bar: when it is wider than
   * the room it has, drop the entry count, then the chips' names (which the
   * tooltips still give). Measured rather than guessed from the window's
   * width: how much room there is depends on the statuses and their names.
   */
  function fitLegend() {
    legend.classList.remove('no-total', 'no-names');
    const tooWide = () => legend.scrollWidth > legend.clientWidth + 1;
    if (tooWide()) legend.classList.add('no-total');
    if (tooWide()) legend.classList.add('no-names');
  }
  const legendFit = new ResizeObserver(() => fitLegend());
  legendFit.observe(document.getElementById('toolbar'));

  // ---------------------------------------------------------------- actions

  const act = {
    /** Select an entry (null = nothing). */
    select(rowId, { reveal = true } = {}) {
      if (st.selected === rowId) { if (reveal) view().showSelection(true); return; }
      st.selected = rowId;
      view().showSelection(reveal);
      renderLegend();
    },

    /** Open or close a container (open: true/false; omitted = the other state). */
    toggle(node, open) {
      if (!node.children.length) return;
      const isOpen = !st.collapsed.has(node.row.item_id);
      const want = open ?? !isOpen;
      if (want === isOpen) return;
      if (want) st.collapsed.delete(node.row.item_id); else st.collapsed.add(node.row.item_id);
      // Closing a container that holds the selection selects the container.
      const sel = st.selected != null ? st.tree.nodes.get(st.selected) : null;
      if (!want && sel && sel !== node && isWithin(sel, node)) st.selected = node.id;
      saveCollapsed();
      render();
    },

    expandAll() { st.collapsed.clear(); saveCollapsed(); render(); },

    collapseAll() {
      for (const n of st.tree.containers) if (n.children.length) st.collapsed.add(n.row.item_id);
      // The selection moves up to its outermost container, which is still in view.
      let sel = st.selected != null ? st.tree.nodes.get(st.selected) : null;
      while (sel && sel.parent) sel = sel.parent;
      if (sel) st.selected = sel.id;
      saveCollapsed();
      render();
    },

    /**
     * Give an entry a status; {advance} then moves the selection to the next
     * entry in view. The change shows at once. Several in a row on one entry
     * (two keys in quick succession) are sent in order and the last answer
     * decides: a failure falls back to the last status the server agreed to.
     */
    setStatus(rowId, statusId, { advance = false } = {}) {
      const row = rowById(rowId);
      if (!row) return;
      const next = advance ? st.visible[st.visible.findIndex((n) => n.id === rowId) + 1] : null;
      if (row.status_id === statusId) {
        if (next) act.select(next.id); else view().showSelection(true);
        return;
      }
      const p = pendingStatus.get(rowId) || { confirmed: row.status_id, n: 0 };
      p.n++;
      pendingStatus.set(rowId, p);
      row.status_id = statusId;
      if (st.selected === rowId && next) st.selected = next.id;
      render();
      view().showSelection(true);
      const settle = (err) => {
        p.n--;
        if (err) toast(sentence(err.message));
        if (p.n > 0) return; // a later change to this entry is still on its way: its answer decides
        pendingStatus.delete(rowId);
        const now = rowById(rowId);
        if (now && now.status_id !== p.confirmed) { now.status_id = p.confirmed; render(); }
      };
      enqueue(() => api.updateContent(rowId, { status_id: statusId })).then((saved) => { p.confirmed = saved.status_id; settle(null); }, settle);
    },

    /** Save a change to an entry's quantity, unit or notes; resolves to whether it was saved. */
    async edit(rowId, body) {
      const row = rowById(rowId);
      if (!row) return false;
      const keys = Object.keys(body);
      const before = Object.fromEntries(keys.map((k) => [k, row[k]]));
      Object.assign(row, body); // at once, so that a redraw while it is being saved shows what was typed
      try {
        const saved = await enqueue(() => api.updateContent(rowId, body));
        const now = rowById(rowId);
        if (now) for (const k of keys) now[k] = saved[k]; // as the server stored it (trimmed)
      } catch (e) {
        const now = rowById(rowId);
        if (now) Object.assign(now, before);
        toast(sentence(e.message));
        view().updateRow(rowId);
        return false;
      }
      return true;
    },

    /** Open the add-entry row at the top level (no arguments), inside a container (an item id), or in one of its compartments. */
    startAdd(container = null, compartment = null) {
      if (!statuses().length) { toast('There are no statuses yet: add one under Settings first.'); return; }
      if (container != null && st.collapsed.delete(container)) saveCollapsed();
      st.adding = { container, compartment: container == null ? null : compartment };
      render();
      view().focusDraft();
    },

    cancelAdd() { st.adding = null; render(); },

    /** Add an entry; resolves to the new row, or null when the server refused it. */
    async add(body) {
      let row;
      try {
        row = await tracked(() => api.addContent(st.list.id, body));
      } catch (e) {
        toast(sentence(e.message));
        return null;
      }
      st.rows.push(row);
      st.selected = row.id;
      render();
      return row;
    },

    /**
     * Move an entry into a container (an item id) or to the top level (null),
     * and into one of that container's compartments (or none); what is inside
     * the entry follows.
     */
    async move(rowId, container, compartment = null) {
      const row = rowById(rowId);
      if (!row) return;
      try {
        Object.assign(row, await enqueue(() => api.updateContent(rowId, { container_id: container, compartment_id: compartment })));
      } catch (e) {
        toast(sentence(e.message));
        return;
      }
      if (container != null && st.collapsed.delete(container)) saveCollapsed();
      st.selected = rowId;
      render();
      view().showSelection(true);
    },

    /**
     * Can an entry go to this place: a container (an item id; null = the top
     * level) and one of its compartments (or none)? Not where it already is,
     * not into itself or anything inside it, and not where the same item
     * already is (an item is in a container once, whatever the compartment).
     */
    canMove(node, container, compartment = null) {
      const here = node.row.container_id ?? null;
      if (container === here && compartment === (node.compartment ?? null)) return false;
      if (container == null) { if (compartment != null) return false; }
      else {
        const target = st.tree.holder.get(container);
        if (!target || isWithin(target, node)) return false;
        if (compartment != null && ref.by.compartments.get(compartment)?.item_id !== container) return false;
      }
      return container === here || !st.rows.some((r) => r.id !== node.id && r.item_id === node.row.item_id && (r.container_id ?? null) === container);
    },

    /** Offer the places an entry can move to: the top level, every container it can go into, and their compartments. */
    openMoveMenu(anchor, node) {
      const here = node.row.container_id ?? null;
      const inCompartment = node.compartment;
      // The same item cannot be in one container twice (whatever the compartment), nor twice at the top level.
      const taken = (container) => st.rows.some((r) => r.id !== node.id && r.item_id === node.row.item_id && (r.container_id ?? null) === container);
      const items = [];
      if (here != null && !taken(null)) items.push({ label: 'Top level', depth: 0, to: null, compartment: null });
      for (const c of st.tree.containers) {
        if (isWithin(c, node)) continue; // not into itself, nor into anything inside it
        const itemId = c.row.item_id;
        const isHere = itemId === here;
        if (!isHere && taken(itemId)) continue;
        const compartments = compartmentsOf(itemId).filter((k) => !(isHere && k.id === inCompartment));
        // Where it already is: only its other compartments (and out of the one it is in) are places to go.
        if (isHere && !compartments.length && inCompartment == null) continue;
        items.push({ label: c.name, depth: c.depth + 1, to: itemId, compartment: null, box: true, disabled: isHere && inCompartment == null,
          title: isHere ? (inCompartment == null ? 'It is in this container (in no compartment)' : 'Out of its compartment: in the container, in no compartment') : null });
        for (const k of compartments) items.push({ label: k.name, depth: c.depth + 2, to: itemId, compartment: k.id });
      }
      const close = popover(anchor, el('div', { class: 'menu move-menu', role: 'menu', 'aria-label': `Move ${node.name} to` },
        el('div', { class: 'combo-group' }, `Move “${node.name}” to`),
        items.length
          ? items.map((it) => el('button', {
            type: 'button', role: 'menuitem', class: it.compartment != null ? 'to-compartment' : null, disabled: it.disabled, title: it.title,
            style: { paddingLeft: `${12 + it.depth * 14}px` }, dataset: { container: it.to ?? '', compartment: it.compartment ?? '' },
            onClick: () => { close(); act.move(node.id, it.to, it.compartment); },
          }, it.box ? icon('box', { size: 13 }) : null, it.label))
          : el('div', { class: 'combo-empty' }, 'There is nowhere else to put it: add a container to the list first.')), { flush: true, align: 'right' });
    },

    /** Offer the statuses for an entry (the mouse's way of doing what the keys do); `at` = {x, y} opens it at the pointer. */
    openStatusMenu(anchor, node, at = null) {
      const close = popover(anchor, el('div', { class: 'menu status-menu', role: 'menu', 'aria-label': `Status of ${node.name}` },
        statuses().map((s) => el('button', { type: 'button', role: 'menuitemradio', 'aria-checked': String(s.id === node.row.status_id),
          onClick: () => { close(); act.setStatus(node.id, s.id); } }, statusChip(s, { key: true })))), { flush: true, at });
    },

    /** Take an entry off the list. A container with things inside asks what becomes of them. */
    async remove(rowId) {
      const node = st.tree.nodes.get(rowId);
      if (!node) return;
      const drop = async (keepContents) => {
        try {
          await tracked(() => queue.then(() => api.deleteContent(rowId, { keepContents })));
        } catch (e) {
          toast(sentence(e.message));
          return false;
        }
        // The server's word on what is left (a container takes its contents with it, or leaves them one level up).
        const next = st.visible[st.visible.findIndex((n) => n.id === rowId) + 1];
        try { st.rows = (await api.list(st.list.id)).contents; } catch (e) { toast(sentence(e.message)); }
        st.selected = next && st.rows.some((r) => r.id === next.id) ? next.id : null;
        render();
        return true;
      };
      if (node.children.length) {
        modal({
          title: `Remove “${node.name}”`,
          body: el('p', {}, `“${node.name}” has ${plural(node.size, 'entry', 'entries')} inside it on this list. Remove them too, or keep them and move what is directly inside it up to ${node.parent ? `“${node.parent.name}”` : 'the top level'}?`),
          actions: [
            { label: 'Cancel' }, { spacer: true },
            { label: 'Keep the contents', onClick: () => drop(true) },
            { label: `Remove all ${node.size + 1}`, danger: true, onClick: () => drop(false) },
          ],
        });
        return;
      }
      const { item_id, container_id, compartment_id, quantity, unit, status_id, notes } = node.row;
      if (await drop(false)) {
        toast(`Removed “${node.name}”.`, { info: true, ms: 7000, action: { label: 'Undo', onClick: () => act.add({ item_id, container_id, compartment_id, quantity, unit, status_id, notes }) } });
      }
    },

  };

  theView = boxesView(st, act);

  // ---------------------------------------------------------------- title bar

  const switcher = el('select', { class: 'list-switch', 'aria-label': 'List', title: 'Switch to another list' });
  function fillSwitcher() {
    const group = (label, rows) => (rows.length ? el('optgroup', { label }, rows.sort((a, b) => byText(a.name, b.name)).map((l) => el('option', { value: String(l.id) }, l.name))) : null);
    switcher.replaceChildren(group('Lists', lists.filter((l) => !l.is_template)), group('Templates', lists.filter((l) => l.is_template)));
    switcher.value = String(st.list.id);
  }
  switcher.addEventListener('change', () => ctx.navigate(`#/list?id=${switcher.value}`));

  const templateChip = el('span', { class: 'chip template-chip', title: 'This list is a template: a list meant to be copied from' }, 'Template');
  const expandBtn = el('button', { type: 'button', id: 'btn-expand-all', 'aria-label': 'Expand all', title: 'Open every container', onClick: () => act.expandAll() }, icon('expand-all'));
  const collapseBtn = el('button', { type: 'button', id: 'btn-collapse-all', 'aria-label': 'Collapse all', title: 'Close every container', onClick: () => act.collapseAll() }, icon('collapse-all'));
  // "Add item" adds at the top level of the list. (Into a container: the box button on its heading; into a
  // compartment: the + beside its name.)
  const addBtn = el('button', { type: 'button', class: 'primary', id: 'btn-add-item',
    title: 'Add an item at the top level of the list. To add inside a container, use the box button on its heading.',
    onClick: () => act.startAdd(null) }, icon('plus', { size: 14 }), 'Add item');

  const filterBox = el('input', { type: 'search', class: 'filter-box', placeholder: 'Filter entries…', 'aria-label': 'Filter entries' });
  filterBox.addEventListener('input', () => { st.words = fold(filterBox.value).split(/\s+/).filter(Boolean); render(); });
  // Enter and Escape hand the keys back to the list; the filter stays as typed. (Left alone, Escape would
  // empty a search box without telling anyone.)
  filterBox.addEventListener('keydown', (e) => { if (e.key === 'Escape' || e.key === 'Enter') { e.preventDefault(); filterBox.blur(); } });
  const statusFilter = multiSelect({
    label: 'Status',
    options: () => statuses().map((s) => ({ value: s.id, label: s.name, color: s.color })),
    onChange: (sel) => { st.statusFilter = sel; render(); },
  });
  // The Sort menu (list/sort.js): several columns, in order.
  const sorter = sortButton({
    get: () => st.sort,
    set: (sort) => { st.sort = sort; prefs.set('list.sort', sort); render(); view().showSelection(true); },
  });

  // The ⋯ menu reads its items when it opens, so their wording can follow the state (the template flag, what is shown).
  const templateItem = { label: '', onClick: toggleTemplate };
  const metaItem = { label: '', onClick: () => { st.showMeta = !st.showMeta; prefs.set('list.meta', st.showMeta); render(); } };
  const menu = menuButton(icon('more'), [
    metaItem,
    { label: 'Print…', onClick: () => window.print() },
    { label: 'Rename…', onClick: renameList },
    templateItem,
    { label: 'Give every entry a status…', onClick: setAllStatuses },
    { label: 'New list from this one…', onClick: () => openNewList({ lists, sources: [st.list.id], name: st.list.is_template ? '' : `Copy of ${st.list.name}`, onCreated: (l) => ctx.navigate(`#/list?id=${l.id}`) }) },
    { label: 'Delete this list…', onClick: deleteList },
  ], { id: 'btn-list-menu', 'aria-label': 'List actions', title: 'Show or hide details, print, rename, template, copy, delete…' });

  ctx.toolbar.append(
    switcher, templateChip,
    el('div', { class: 'group', role: 'group', 'aria-label': 'Containers' }, expandBtn, collapseBtn),
    addBtn, filterBox, statusFilter.el, sorter.el, legend,
    el('span', { class: 'spacer' }), menu);

  function syncToolbar() {
    templateChip.hidden = !st.list.is_template;
    templateItem.label = st.list.is_template ? 'Stop being a template' : 'Make it a template';
    metaItem.label = st.showMeta ? 'Hide category and storage location' : 'Show category and storage location';
    printTitle.replaceChildren(
      el('h1', {}, st.list.name, st.list.is_template ? el('span', { class: 'print-sub' }, ' (template)') : null),
      el('p', { class: 'print-sub' }, [
        st.filtering ? `${st.matches} of ${plural(st.rows.length, 'entry', 'entries')} (filtered)` : plural(st.rows.length, 'entry', 'entries'),
        new Date().toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' }),
      ].join(' · ')));
    const anyOpenable = st.tree.containers.some((n) => n.children.length);
    expandBtn.disabled = !anyOpenable || st.filtering;
    collapseBtn.disabled = !anyOpenable || st.filtering;
    expandBtn.title = st.filtering ? 'Every match is shown while the filter is on' : 'Open every container';
    collapseBtn.title = st.filtering ? 'Every match is shown while the filter is on' : 'Close every container';
  }

  // ---------------------------------------------------------------- list-level actions

  async function patchList(body) {
    try {
      const d = await tracked(() => api.updateList(st.list.id, body));
      st.list = { id: d.id, name: d.name, is_template: d.is_template };
      lists = await api.lists();
    } catch (e) {
      toast(sentence(e.message));
      return;
    }
    ctx.setTitle(st.list.name);
    fillSwitcher();
    syncToolbar();
  }

  async function renameList() {
    const name = await promptText({ title: 'Rename list', label: 'Name', value: st.list.name });
    if (name && name !== st.list.name) await patchList({ name });
  }

  async function toggleTemplate() {
    const want = !st.list.is_template;
    await patchList({ is_template: want });
    if (st.list.is_template === want) toast(want ? `“${st.list.name}” is now a template.` : `“${st.list.name}” is no longer a template.`, { info: true, ms: 3000 });
  }

  function setAllStatuses() {
    if (!st.rows.length) { toast('The list is empty.', { info: true }); return; }
    const m = modal({
      title: 'Give every entry a status',
      body: [
        el('p', {}, `All ${plural(st.rows.length, 'entry', 'entries')} of “${st.list.name}” get the status you pick: to start packing afresh, say.`),
        el('div', { class: 'status-pick' }, statuses().map((s) => statusChip(s, { onClick: async () => {
          try {
            st.rows = (await tracked(() => queue.then(() => api.setAllStatuses(st.list.id, s.id)))).contents;
          } catch (e) {
            toast(sentence(e.message));
            return;
          }
          m.close();
          render();
        } }))),
      ],
      actions: [{ label: 'Cancel' }],
    });
  }

  async function deleteList() {
    const what = st.rows.length ? ` and its ${plural(st.rows.length, 'entry', 'entries')}` : '';
    if (!confirm(`Delete the list "${st.list.name}"${what}?\n\nThe items themselves stay in the library.`)) return;
    try {
      await api.deleteList(st.list.id);
    } catch (e) {
      toast(sentence(e.message));
      return;
    }
    ctx.navigate('#/lists');
  }

  // ---------------------------------------------------------------- keyboard

  // The button or link the pointer last pressed (see onKey); Tab forgets it.
  let clicked = null;
  const onPointer = (e) => { clicked = e.target.closest ? e.target.closest('button, a') : null; };
  document.addEventListener('pointerdown', onPointer, true);

  // A click on empty space (the page around and between the boxes, a box's
  // own margins) selects nothing. Clicks on an entry select it (list/boxes.js);
  // clicks on controls, in the title bar, or in a menu or dialog are not "empty space".
  const onEmptyClick = (e) => {
    const t = e.target;
    if (!t.closest || st.selected == null) return;
    if (t.closest('.row.entry, button, a, input, select, textarea, label, .popover, .modal-backdrop, .topbar, #toast')) return;
    act.select(null);
  };
  document.addEventListener('click', onEmptyClick);

  function onKey(e) {
    if (e.key === 'Tab') { clicked = null; return; }
    if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey || e.isComposing) return;
    if (typingTarget(e.target) || overlayOpen()) return;
    // A button or link reached with the keyboard keeps the keys that press
    // it. One that merely holds the focus because it was last clicked (the
    // view switch, a caret) does not: Space and Enter belong to the list.
    const onControl = e.target.closest && e.target.closest('button, a');
    if (onControl && (e.key === ' ' || e.key === 'Enter')) {
      if (onControl !== clicked) return;
      onControl.blur();
    }
    if (view().key(e)) { e.preventDefault(); return; }
    const status = statusForKey(e.key);
    if (status && st.selected != null) {
      e.preventDefault();
      // Shift moves on to the next entry, for letters only: a symbol that needs Shift to type is just its key.
      const letter = e.key.toLowerCase() !== e.key.toUpperCase();
      act.setStatus(st.selected, status.id, { advance: e.shiftKey && letter });
    }
  }
  document.addEventListener('keydown', onKey);

  // ---------------------------------------------------------------- staying current

  // Coming back to the tab (after packing with the phone, say) shows what changed meanwhile.
  let refreshing = false;
  const busy = () => !!st.adding || overlayOpen() || typingTarget();
  async function refresh() {
    if (refreshing || document.hidden || busy()) return;
    refreshing = true;
    const seen = writes;
    try {
      const [fresh] = await Promise.all([api.list(st.list.id), reloadRef()]);
      if (busy()) return;
      // What was read is only used when nothing was changed here meanwhile (else it may predate the change).
      if (seen === writes && !pendingStatus.size) {
        st.rows = fresh.contents;
        st.list = { id: fresh.id, name: fresh.name, is_template: fresh.is_template };
        ctx.setTitle(st.list.name);
      }
      render(); // always: the statuses and the library may have changed too
    } catch (e) {
      /* offline or the list is gone: the next action will say so */
    } finally {
      refreshing = false;
    }
  }
  document.addEventListener('visibilitychange', refresh);
  window.addEventListener('focus', refresh);

  // ---------------------------------------------------------------- go

  fillSwitcher();
  viewHolder.replaceChildren(view().el);
  render();

  return () => {
    document.removeEventListener('keydown', onKey);
    document.removeEventListener('pointerdown', onPointer, true);
    document.removeEventListener('click', onEmptyClick);
    document.removeEventListener('visibilitychange', refresh);
    window.removeEventListener('focus', refresh);
    legendFit.disconnect();
    view().destroy();
  };
}
