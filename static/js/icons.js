// Inline SVG icons (16x16, stroke = currentColor), as in the planner.
//
//   icon('trash')            -> <svg class="icon">
//   icon('plus', {size: 14})
//
// Buttons that show only an icon must carry an aria-label and a title: the
// label names the control for assistive technology and the title is the
// mouseover description.

// Each entry is the inner markup of a 16x16 viewBox; strokes use
// currentColor, 1.6px, round caps and joins unless the entry says otherwise.
const PATHS = {
  plus: '<path d="M8 3v10M3 8h10"/>',
  trash: '<path d="M3 4.5h10M6.5 4.5V3h3v1.5M4.5 4.5l.6 8.5h5.8l.6-8.5M6.8 7v4M9.2 7v4"/>',
  check: '<path d="M3 8.5l3.2 3.2L13 5"/>',
  x: '<path d="M4 4l8 8M12 4l-8 8"/>',
  edit: '<path d="M3 13l.6-2.8L10.9 3l2.1 2.1-7.3 7.3L3 13zM9.6 4.3l2.1 2.1"/>',
  copy: '<path d="M5.5 5.5v-3h8v8h-3M2.5 5.5h8v8h-8z"/>',
  // A cog: eight teeth around a hub (not a sun: the teeth are blocks joined at the rim).
  gear: '<path d="M6.79 2.94L7.08 1.16L8.92 1.16L9.21 2.94L10.72 3.57L12.19 2.51L13.49 3.81L12.43 5.28L13.06 6.79L14.84 7.08L14.84 8.92L13.06 9.21L12.43 10.72L13.49 12.19L12.19 13.49L10.72 12.43L9.21 13.06L8.92 14.84L7.08 14.84L6.79 13.06L5.28 12.43L3.81 13.49L2.51 12.19L3.57 10.72L2.94 9.21L1.16 8.92L1.16 7.08L2.94 6.79L3.57 5.28L2.51 3.81L3.81 2.51L5.28 3.57ZM10.10 8a2.1 2.1 0 1 0 -4.20 0a2.1 2.1 0 1 0 4.20 0Z"/>',
  // Show / hide what is inside every container (as the planner's expand-all and collapse-all).
  'expand-all': '<path d="M4 5.5l4-3.5 4 3.5M4 10.5l4 3.5 4-3.5"/>',
  'collapse-all': '<path d="M4 2l4 3.5 4-3.5M4 14l4-3.5 4 3.5"/>',
  // Move a row into another container.
  move: '<path d="M2.5 5.5h9M9 2.5l3 3-3 3M13.5 10.5h-9M7 7.5l-3 3 3 3"/>',
  // Add something inside a container: a box with a plus.
  'add-inside': '<path d="M2.5 6.5v6h8v-2.5M2.5 6.5l1.6-2.5h4.4M10.5 2v5M8 4.5h5"/>',
  // The mark beside a container's name: an open box.
  box: '<path d="M2.5 5.5l5.5-2.5 5.5 2.5v6l-5.5 2.5-5.5-2.5zM2.5 5.5l5.5 2.5 5.5-2.5M8 8v6"/>',
  more: '<circle cx="3.5" cy="8" r="1.2" fill="currentColor" stroke="none"/><circle cx="8" cy="8" r="1.2" fill="currentColor" stroke="none"/><circle cx="12.5" cy="8" r="1.2" fill="currentColor" stroke="none"/>',
};

const NS = 'http://www.w3.org/2000/svg';

/** An icon by name; {size} sets width and height (default 16), {title} adds a tooltip. */
export function icon(name, { size = 16, title } = {}) {
  const svg = document.createElementNS(NS, 'svg');
  svg.setAttribute('viewBox', '0 0 16 16');
  svg.setAttribute('width', size);
  svg.setAttribute('height', size);
  svg.setAttribute('class', `icon icon-${name}`);
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.6');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('focusable', 'false');
  svg.innerHTML = (title ? `<title></title>` : '') + (PATHS[name] || '');
  if (title) svg.querySelector('title').textContent = title;
  return svg;
}
