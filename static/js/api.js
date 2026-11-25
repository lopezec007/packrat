// Thin fetch wrappers around the JSON API (docs/api.md). Every function
// returns a promise that rejects with an Error whose message is the server's
// error text (err.status has the HTTP status).

async function req(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(path, opts);
  } catch (e) {
    throw new Error('Cannot reach the Packrat server');
  }
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || `${method} ${path} failed (${res.status})`);
    err.status = res.status;
    throw err;
  }
  return data;
}

export const api = {
  version: () => req('GET', '/api/version'),

  // The library and settings tables; `table` is 'item_categories',
  // 'storage_locations', 'statuses' or 'items'.
  ref: () => req('GET', '/api/ref'),
  createRef: (table, body) => req('POST', `/api/ref/${table}`, body),
  updateRef: (table, id, body) => req('PATCH', `/api/ref/${table}/${id}`, body),
  deleteRef: (table, id) => req('DELETE', `/api/ref/${table}/${id}`),
  /** Rank the statuses: ids lists every status, from the worst to the best. Returns the statuses. */
  orderStatuses: (ids) => req('POST', '/api/statuses/order', { ids }),

  // Lists
  lists: () => req('GET', '/api/lists'),
  list: (id) => req('GET', `/api/lists/${id}`),
  /** {name, is_template, sources: [list ids to combine]} -> the new list with its contents. */
  createList: (body) => req('POST', '/api/lists', body),
  /** How many entries a list made from these sources would have: {rows}. */
  previewList: (sources) => req('POST', '/api/lists/preview', { sources }),
  updateList: (id, body) => req('PATCH', `/api/lists/${id}`, body),
  deleteList: (id) => req('DELETE', `/api/lists/${id}`),
  /** Give every entry of a list the same status; returns the list with its contents. */
  setAllStatuses: (id, statusId) => req('POST', `/api/lists/${id}/statuses`, { status_id: statusId }),

  // What is on a list
  addContent: (listId, body) => req('POST', `/api/lists/${listId}/contents`, body),
  updateContent: (id, body) => req('PATCH', `/api/contents/${id}`, body),
  /** Remove an entry; {keepContents: true} moves what is inside a container up instead of removing it too. */
  deleteContent: (id, { keepContents = false } = {}) => req('DELETE', `/api/contents/${id}${keepContents ? '?contents=keep' : ''}`),
};
