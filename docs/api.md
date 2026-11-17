# Packrat HTTP API

Everything the web UI does goes through this API; it has no other access to
the database. All routes are under `/api/`, speak JSON, and need no
authentication (Packrat is meant for a trusted network; see the README).

## Conventions

- Request and response bodies are JSON (`Content-Type: application/json`).
- Rows are objects whose keys are the column names in lower case: `id`,
  `name`, `item_category_id`, `is_container`...
- Errors are `{"error": "message"}` with one of:

  | Status | Meaning |
  |---|---|
  | 400 | The request is wrong: a missing or malformed field, an unknown field, a reference to a row that does not exist. |
  | 404 | No such row, or no such endpoint. |
  | 409 | The request is well formed but the data forbids it: a duplicate name, deleting a row that is in use, a second placement of a container. |
  | 413 | The body is larger than 1 MiB. |
  | 500 | A bug or a database failure; the details are in the service's log. |

  The message is written to be shown to a person.
- `POST` creates and answers `201` with the new row. `PATCH` is partial:
  keys that are absent are left alone, and an explicit `null` clears a
  nullable field. `DELETE` answers `204` with no body.
- Unknown keys in a body are refused (400), except that the computed keys a
  row came with (`item_count`, `list_count`, `use_count`, `id`) may be sent
  back and are ignored.

## Version

`GET /api/version` → `{"version": "0.1.0", "date": "2026-11-16", "build": "dev"}`

## Library and settings tables

Four tables share one set of routes; `{table}` is one of:

| `{table}` | Fields | Computed (read-only) | Order |
|---|---|---|---|
| `item_categories` | `name` | `item_count`: items in the category | by name |
| `storage_locations` | `name` | `item_count`: items stored there | by name |
| `statuses` | `name`, `color`, `shortcut` | `use_count`: list entries with the status | by id |
| `items` | `name`, `item_category_id`, `storage_location_id`, `is_container` (default `false`) | `list_count`: lists the item is on | by name |

| Route | |
|---|---|
| `GET /api/ref` | Every table at once: `{"item_categories": [...], "storage_locations": [...], "statuses": [...], "items": [...]}` |
| `GET /api/ref/{table}` | One table's rows. |
| `POST /api/ref/{table}` | Create a row. |
| `PATCH /api/ref/{table}/{id}` | Change a row. |
| `DELETE /api/ref/{table}/{id}` | Delete a row; 409 while it is in use. |

Rules:

- `name`: required, at most 127 characters, unique within its table (item
  names are unique across all categories).
- `color`: a hex colour. `"#2E8540"` and `"2e8540"` are both accepted; it is
  stored as six lower-case digits and always returned as `"#2e8540"`.
- `shortcut`: exactly one visible character, stored in lower case, unique.
- The first status (lowest `id`) is the default for new list entries.
- `is_container` cannot be set to `false` while the item holds other items
  on any list, nor to `true` while the item is on some list more than once
  (409, naming the list).
- A category or storage location with items, an item that is on a list, and
  a status that entries have cannot be deleted (409).

```bash
curl -s -X POST localhost:8082/api/ref/items -H 'Content-Type: application/json' \
  -d '{"name": "Kite", "item_category_id": 2, "storage_location_id": 5}'
# {"id":44,"is_container":false,"item_category_id":2,"list_count":0,"name":"Kite","storage_location_id":5}
```

## Lists

A list:

```json
{"id": 3, "name": "Park Day", "is_template": false, "rows": 7, "status_counts": {"1": 2, "2": 4, "3": 1}}
```

`rows` is the number of entries and `status_counts` splits them by status id
(statuses with no entries are left out). Routes that return one list add
`"contents": [...]`, its entries ordered by id.

| Route | |
|---|---|
| `GET /api/lists` | Every list, by name (without `contents`). |
| `POST /api/lists` | Create a list; returns it with its contents. |
| `POST /api/lists/preview` | `{"sources": [1, 2]}` → `{"rows": 31}`: how many entries a list made from those lists would have. Creates nothing. |
| `GET /api/lists/{id}` | One list with its contents. |
| `PATCH /api/lists/{id}` | `{"name": ..., "is_template": ...}`; returns the list with its contents. |
| `DELETE /api/lists/{id}` | Delete the list and its entries. The library is not touched. |
| `POST /api/lists/{id}/statuses` | `{"status_id": 1}`: give every entry that status; returns the list with its contents. |

### Creating a list

```json
{"name": "Winter Camp", "is_template": false, "sources": [1, 2]}
```

- `name`: required, at most 127 characters, unique.
- `sources`: ids of the lists to make it from, in order; empty or absent
  starts an empty list. Repeats are ignored; an id that is not a list is 400.
  One source gives a copy of it. Several are combined into their union:
  every placement (an item in a container) on any source.
  - The same placement on several sources is one entry, with the largest
    quantity when the units match and the unit, status and notes of the
    first source that has it.
  - A container is placed where the first source that has it puts it; what
    later sources put inside it follows it there.
- Statuses: an entry taken from a source that is a template keeps its
  status; an entry taken from an ordinary list gets the default status.

## Entries

An entry is one `List_Contents` row:

```json
{"id": 33, "list_id": 3, "container_id": 2, "item_id": 20, "quantity": 2, "unit": null, "status_id": 2, "notes": null}
```

`container_id` is the **item** the entry is inside (`null` = the top level of
the list), not another entry's id.

| Route | |
|---|---|
| `POST /api/lists/{id}/contents` | Add an item to the list. |
| `PATCH /api/contents/{id}` | Change an entry. |
| `DELETE /api/contents/{id}` | Remove an entry and, when it is a container, everything inside it at every depth. |
| `DELETE /api/contents/{id}?contents=keep` | Remove an entry but keep what was directly inside it, moved to where it was. 409 when one of those items is already there. |

Fields:

| Field | On create | Rules |
|---|---|---|
| `item_id` | required | Cannot be changed afterwards. |
| `container_id` | default `null` | See the rules below. `PATCH` with `null` moves the entry to the top level. |
| `quantity` | default `1` | A whole number from 1 up. |
| `unit` | default `null` | At most 127 characters; `""` and `null` both clear it. |
| `status_id` | default: the first status | Must exist. |
| `notes` | default `null` | At most 255 characters; `""` and `null` both clear it. |

What keeps a list a tree:

1. `container_id` must be an item with `is_container` that is itself on the
   same list (400 otherwise).
2. A container item can be on a list only once (409 on a second placement).
   Other items can be on a list several times, but only once per container
   (409: change the quantity instead).
3. An entry cannot be moved into itself or into anything inside it (400).
   Moving a container takes its contents along: they refer to its item.

```bash
# Put two pairs of socks (item 24) in the backpack (item 2) on list 3, already packed (status 2)
curl -s -X POST localhost:8082/api/lists/3/contents -H 'Content-Type: application/json' \
  -d '{"item_id": 24, "container_id": 2, "quantity": 2, "unit": "pairs", "status_id": 2}'

# Mark an entry missing (status 3)
curl -s -X PATCH localhost:8082/api/contents/95 -H 'Content-Type: application/json' -d '{"status_id": 3}'
```
