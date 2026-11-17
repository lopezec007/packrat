// How statuses are drawn: the pill with a status's name in its colour, and
// the bar that shows how a list's entries split between the statuses.

import { ref, statuses } from './state.js';
import { el, contrastText, plural } from './util.js';

/**
 * A status as a pill in its colour.
 *
 *   statusChip(status)                       <span>
 *   statusChip(status, {key: true})          with its shortcut as a key cap
 *   statusChip(status, {count: 12})          with a number after the name
 *   statusChip(status, {onClick, title})     a <button>
 */
export function statusChip(status, { key = false, count = null, onClick = null, title = null, label = true } = {}) {
  const color = status?.color || '#9aa3ad';
  const node = el(onClick ? 'button' : 'span', {
    class: 'status-chip', type: onClick ? 'button' : null, title, onClick,
    style: `--c: ${color}; --t: ${contrastText(color)}`, dataset: { status: status?.id ?? '' },
  },
    key && status?.shortcut ? el('span', { class: 'key-cap' }, status.shortcut) : null,
    label ? el('span', { class: 'name' }, status?.name ?? 'Unknown') : null,
    count != null ? el('span', { class: 'count' }, count) : null);
  return node;
}

/** "12 Packed · 3 Missing" for a {status id: rows} map, in status order. */
export function countsText(counts) {
  return statuses().filter((s) => counts[s.id]).map((s) => `${counts[s.id]} ${s.name}`).join(' · ');
}

/**
 * A bar split in proportion to a {status id: rows} map, each part in its
 * status's colour. Empty when there are no rows.
 */
export function statusBar(counts) {
  const total = Object.values(counts || {}).reduce((a, b) => a + b, 0);
  if (!total) return el('span', { class: 'muted' }, '—');
  return el('div', { class: 'status-bar', role: 'img', title: countsText(counts), 'aria-label': `${plural(total, 'entry', 'entries')}: ${countsText(counts)}` },
    statuses().filter((s) => counts[s.id]).map((s) => el('span', { style: { background: s.color, flexGrow: counts[s.id] } })));
}

/** A status id -> rows map for a list of entries. */
export function countByStatus(rows) {
  const counts = {};
  for (const r of rows) counts[r.status_id] = (counts[r.status_id] || 0) + 1;
  return counts;
}

export const statusById = (id) => ref.by.statuses.get(id);
