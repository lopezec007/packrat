// Small DOM and text helpers shared by every page.

/** Build an element: el('div', {class: 'x', onClick: fn, dataset: {id: 1}}, child, 'text', [more]). */
export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') node.className = v;
    else if (k === 'dataset') Object.assign(node.dataset, v);
    else if (k.startsWith('on') && typeof v === 'function') node.addEventListener(k.slice(2).toLowerCase(), v);
    else if (k === 'style' && typeof v === 'object') Object.assign(node.style, v);
    else if (k === 'value' || k === 'checked' || k === 'disabled' && typeof v === 'boolean') node[k] = v;
    else if (v === true) node.setAttribute(k, '');
    else node.setAttribute(k, v);
  }
  for (const c of children.flat(Infinity)) {
    if (c == null || c === false) continue;
    node.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return node;
}

let toastTimer;
/**
 * Show a message at the bottom of the window: red for errors (default), dark
 * for {info: true}. {action: {label, onClick}} adds a button (e.g. Undo).
 */
export function toast(msg, { info = false, ms = 5000, action = null } = {}) {
  const t = document.getElementById('toast');
  t.replaceChildren(el('span', {}, msg));
  if (action) {
    t.append(el('button', { type: 'button', class: 'toast-action', onClick: () => { t.hidden = true; action.onClick(); } }, action.label));
  }
  t.classList.toggle('info', info);
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, ms);
}

/** A server message as a sentence: "that item already exists" -> "That item already exists". */
export const sentence = (s) => (s ? s[0].toUpperCase() + s.slice(1) : s);

/** Pick black or white text for a background hex colour ("#rrggbb"). */
export function contrastText(hex) {
  const n = parseInt(String(hex).replace('#', ''), 16);
  const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
  const lum = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return lum > 0.6 ? '#000' : '#fff';
}

export function debounce(fn, ms = 200) {
  let t;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}

/** A whole number with thousands separators. */
export function fmtNum(n) {
  if (n == null || n === '') return '';
  return Number(n).toLocaleString('en-US');
}

/** plural(3, 'entry', 'entries') -> "3 entries"; plural(1, 'item') -> "1 item". */
export function plural(n, one, many = `${one}s`) {
  return `${fmtNum(n)} ${n === 1 ? one : many}`;
}

/** Case- and accent-insensitive text for matching what someone typed. */
export const fold = (s) => String(s ?? '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();

/** Compare two strings the way a person would sort them. */
export const byText = (a, b) => String(a ?? '').localeCompare(String(b ?? ''), undefined, { sensitivity: 'base', numeric: true });

/** Is the keyboard focus somewhere that takes typing (so page-level keys must stand back)? */
export function typingTarget(target = document.activeElement) {
  if (!target || !target.tagName) return false;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable;
}

/** Read and write small per-browser preferences (collapsed containers, sort order...). */
export const prefs = {
  get(key, fallback = null) {
    try {
      const raw = localStorage.getItem(`packrat.${key}`);
      return raw == null ? fallback : JSON.parse(raw);
    } catch { return fallback; }
  },
  set(key, value) {
    try { localStorage.setItem(`packrat.${key}`, JSON.stringify(value)); } catch { /* private mode: not remembered */ }
  },
};
