// Application shell: hash router, navigation, and the page lifecycle.
//
// Routes are #/<page>?<params>; each page is a module in pages/ exporting
//
//   export async function mount(ctx)   // may return a cleanup function
//
// where ctx = {
//   main,      the element to render into (empty; laid out as <main> itself)
//   toolbar,   the title bar slot for the page's own controls (empty; laid out as the toolbar itself)
//   params,    URLSearchParams from the hash, e.g. #/list?id=3
//   setParams(obj, {replace = true}),  rewrite the hash's params without remounting
//   navigate(hash),                    go to another page, e.g. navigate('#/list?id=3')
//   setTitle(text),                    the browser tab's title (default: the page's label)
// }
//
// The reference data (state.js `ref`) is loaded before any page mounts.

import { loadRef, app } from './state.js';
import { el, toast } from './util.js';
import { closePopover, closeModals } from './widgets.js';
import { icon } from './icons.js';

const PAGES = [
  { route: 'lists', label: 'Lists', module: './pages/lists.js' },
  // One list, opened from the Lists page: shown under "Lists" in the navigation.
  { route: 'list', label: 'List', module: './pages/list.js', under: 'lists' },
  { route: 'library', label: 'Library', module: './pages/library.js' },
  { route: 'settings', label: 'Settings', module: './pages/settings.js', gear: true },
];
const DEFAULT_ROUTE = 'lists';

const main = document.getElementById('main');
const toolbar = document.getElementById('toolbar');
const nav = document.getElementById('nav');
const topbar = document.querySelector('.topbar');

// The title bar's exact height as a CSS variable, so sticky things (table
// header rows) sit just beneath it however the toolbar wraps. On a narrow
// window the title bar scrolls away with the page (app.css), and they sit at
// the top of the window instead.
function syncTopbarHeight() {
  const pinned = getComputedStyle(topbar).position === 'sticky';
  document.documentElement.style.setProperty('--topbar-h', `${pinned ? topbar.getBoundingClientRect().height : 0}px`);
}
new ResizeObserver(syncTopbarHeight).observe(topbar);
window.addEventListener('resize', syncTopbarHeight);
syncTopbarHeight();

function parseHash() {
  const raw = location.hash.replace(/^#\/?/, '');
  const [path, query = ''] = raw.split('?');
  const page = PAGES.find((p) => p.route === path) || PAGES.find((p) => p.route === DEFAULT_ROUTE);
  return { page, params: new URLSearchParams(query) };
}

function renderNav(current) {
  const here = current.under || current.route;
  nav.replaceChildren(...PAGES.filter((p) => !p.gear && !p.under).map((p) =>
    el('a', { href: `#/${p.route}`, 'aria-current': p.route === here ? 'page' : null }, p.label)));
  const gear = document.getElementById('btn-settings');
  if (current.gear) gear.setAttribute('aria-current', 'page'); else gear.removeAttribute('aria-current');
  document.title = `${current.label} · Packrat`;
}

let cleanup = null;
let mountSeq = 0;
let suppressHashChange = false;

async function show() {
  const { page, params } = parseHash();
  const seq = ++mountSeq;
  closePopover();
  closeModals();
  if (cleanup) { try { cleanup(); } catch (e) { console.error(e); } cleanup = null; }
  // Each visit gets slots of its own (boxes that lay their children out as
  // if they were not there), so a page still loading when the next one is
  // asked for goes on filling slots that are no longer on screen.
  const slot = () => el('div', { class: 'slot' });
  const pageMain = slot(), pageToolbar = slot();
  main.replaceChildren(pageMain);
  toolbar.replaceChildren(pageToolbar);
  main.className = `page-${page.route}`;
  document.body.dataset.page = page.route; // for styles that depend on the page (the list page's one-line title bar)
  renderNav(page);
  window.scrollTo(0, 0);

  const ctx = {
    main: pageMain, toolbar: pageToolbar, params,
    setParams(obj, { replace = true } = {}) {
      if (seq !== mountSeq) return;
      const q = new URLSearchParams();
      for (const [k, v] of Object.entries(obj || {})) {
        if (v == null || v === '' || (Array.isArray(v) && !v.length)) continue;
        q.set(k, Array.isArray(v) ? v.join(',') : v);
      }
      const s = q.toString();
      const hash = `#/${page.route}${s ? `?${s}` : ''}`;
      if (hash === location.hash) return;
      suppressHashChange = true;
      if (replace) history.replaceState(null, '', hash); else history.pushState(null, '', hash);
      suppressHashChange = false;
    },
    navigate(hash) { if (seq === mountSeq) location.hash = hash; },
    setTitle(text) { if (seq === mountSeq) document.title = `${text} · Packrat`; },
  };
  try {
    const mod = await import(page.module);
    if (seq !== mountSeq) return; // navigated away while loading
    const result = await mod.mount(ctx);
    if (seq !== mountSeq) { if (typeof result === 'function') result(); return; }
    cleanup = typeof result === 'function' ? result : null;
  } catch (e) {
    console.error(e);
    if (seq !== mountSeq) return;
    pageMain.replaceChildren(el('p', { class: 'empty' }, `This page failed to load: ${e.message}`));
  }
}

window.addEventListener('hashchange', () => { if (!suppressHashChange) show(); });

// Ctrl+F (Cmd+F) goes to the page's own filter box, which searches everything
// on the page (a list's entries, the library), not only what is scrolled into
// view. Pressed again with the cursor already there, it is the browser's own
// Find as usual.
document.addEventListener('keydown', (e) => {
  if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey || e.key.toLowerCase() !== 'f') return;
  const box = toolbar.querySelector('.filter-box');
  if (!box || box === document.activeElement || document.querySelector('.modal-backdrop')) return;
  e.preventDefault();
  closePopover();
  box.focus();
  box.select();
});

function renderVersion() {
  const slot = document.getElementById('app-version');
  slot.textContent = app.version ? `v${app.version}` : '';
  slot.title = app.version
    ? `Packrat ${app.version}, released ${app.date}${app.build && app.build !== 'dev' ? ` (build ${app.build})` : ''}`
    : '';
}

async function start() {
  document.getElementById('btn-settings').replaceChildren(icon('gear', { size: 18 }));
  try {
    await loadRef();
  } catch (e) {
    main.replaceChildren(el('p', { class: 'empty' }, `Failed to load: ${e.message}`));
    toast(`Failed to load: ${e.message}`);
    return;
  }
  renderVersion();
  if (!location.hash) history.replaceState(null, '', `#/${DEFAULT_ROUTE}`);
  show();
}

start();
