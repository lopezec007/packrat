// The shape of a list: its entries (List_Contents rows) arranged as the tree
// the view draws, with the sort order and the filter applied.
//
// An entry sits inside a container, and the container is named by its item
// (row.container_id is an item id), so an entry's children are the rows whose
// container_id is its item. A container is on a list once (the server keeps
// that rule), which is what makes this a tree. An entry can also be in one
// of its container's compartments (row.compartment_id).
//
// Children are kept in reading order: compartment by compartment (in the
// container's own order of them, then what is in none), within each the
// containers first, and those and the items each by the sort (list/sort.js).
//
//   const tree = buildTree(rows, sort);
//   tree.roots                 the top-level nodes, sorted
//   tree.nodes.get(rowId)      a node: {id, row, item, name, parent, compartment, children, depth, size}
//   tree.containers            the container nodes, in display order
//   applyFilter(tree, pred)    sets node.match / node.show on every node
//   visibleNodes(tree, collapsed, filtering)   the nodes the view draws, in reading order

import { ref, nameOf, compartmentsOf } from '../state.js';
import { fold } from '../util.js';
import { compareBy } from './sort.js';

export function buildTree(rows, sort = []) {
  const nodes = new Map();
  for (const row of rows) {
    const item = ref.by.items.get(row.item_id);
    nodes.set(row.id, { id: row.id, row, item, name: item?.name ?? `Item ${row.item_id}`, parent: null, compartment: null, children: [], depth: 0, size: 0, match: true, show: true });
  }
  // The node that holds what is "inside item X" (a container's one place on the list).
  const holder = new Map();
  for (const n of nodes.values()) if (n.item?.is_container && !holder.has(n.row.item_id)) holder.set(n.row.item_id, n);

  const roots = [];
  for (const n of nodes.values()) {
    const p = n.row.container_id == null ? null : holder.get(n.row.container_id);
    if (p && p !== n) { n.parent = p; p.children.push(n); } else roots.push(n);
  }
  const bySort = compareBy(sort);
  const containers = [];
  const seen = new Set();
  (function walk(list, depth, holderNode) {
    // Which compartment each child is in: its place among the container's compartments (none = after them all).
    const order = new Map((holderNode ? compartmentsOf(holderNode.row.item_id) : []).map((c, i) => [c.id, i]));
    for (const n of list) n.compartment = order.has(n.row.compartment_id) ? n.row.compartment_id : null;
    const place = (n) => (n.compartment == null ? order.size : order.get(n.compartment));
    list.sort((a, b) => place(a) - place(b)
      || (a.item?.is_container ? 0 : 1) - (b.item?.is_container ? 0 : 1)
      || bySort(a, b));
    for (const n of list) {
      if (seen.has(n.id)) continue; // cannot happen on a sound list; guards against a loop
      seen.add(n.id);
      n.depth = depth;
      if (n.item?.is_container) containers.push(n);
      walk(n.children, depth + 1, n);
      n.size = n.children.reduce((sum, c) => sum + 1 + c.size, 0); // entries inside, at every depth
    }
  }(roots, 0, null));
  // Anything the walk did not reach (a loop of containers) is shown at the top level rather than lost.
  for (const n of nodes.values()) if (!seen.has(n.id)) { n.parent = null; n.children = []; n.depth = 0; roots.push(n); }
  return { roots, nodes, containers, holder };
}

/** Everything searchable about an entry, folded for matching. */
export function nodeText(n) {
  return fold([
    n.name, nameOf('item_categories', n.item?.item_category_id), nameOf('storage_locations', n.item?.storage_location_id),
    n.row.unit, n.row.notes, nameOf('statuses', n.row.status_id), nameOf('compartments', n.row.compartment_id),
  ].filter(Boolean).join(' '));
}

/**
 * Mark which nodes pass the filter: node.match (the entry itself passes) and
 * node.show (it or something inside it passes, so it must be drawn). Returns
 * the number of matching entries.
 */
export function applyFilter(tree, { words = [], statuses = new Set() } = {}) {
  let matches = 0;
  const active = words.length > 0 || statuses.size > 0;
  (function walk(list) {
    let any = false;
    for (const n of list) {
      const inside = walk(n.children);
      n.match = !active || ((!statuses.size || statuses.has(n.row.status_id)) && (!words.length || (() => { const t = nodeText(n); return words.every((w) => t.includes(w)); })()));
      n.show = n.match || inside;
      if (n.match) matches++;
      if (n.show) any = true;
    }
    return any;
  }(tree.roots));
  return matches;
}

/**
 * The nodes to draw, in order. A collapsed container's contents are left
 * out, except while a filter is on (then every match is in view).
 */
export function visibleNodes(tree, collapsed, filtering = false) {
  const out = [];
  (function walk(list) {
    for (const n of list) {
      if (!n.show) continue;
      out.push(n);
      if (n.children.length && (filtering || !collapsed.has(n.row.item_id))) walk(n.children);
    }
  }(tree.roots));
  return out;
}

/** Is `node` the same as, or inside, `ancestor`? */
export function isWithin(node, ancestor) {
  for (let n = node; n; n = n.parent) if (n === ancestor) return true;
  return false;
}
