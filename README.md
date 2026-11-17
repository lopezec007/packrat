# Packrat

**P**acking **A**ssistant for **C**amping, **K**it, **R**oadtrips **A**nd
**T**ravel. One master library of the things you own, and packing lists that
draw from it.

A single Go binary serving a web UI and a JSON API over a PostgreSQL database.
It runs directly on the OS as a systemd service (no container), speaks plain
HTTP, and is meant for a trusted local network or VPN.

| Page | What it is |
|---|---|
| **Lists** | Every list, ordinary ones apart from the templates, with how its entries split between the statuses. Make a list from scratch or by combining other lists; rename, copy, delete. |
| **A list** | The list as boxes within boxes: every container is a box, the containers inside it sit side by side in it, and the items packed into it are listed inside with all their fields (quantity, unit, category, where it is stored, status, notes). Entries and boxes are tinted by status. Containers open and close, one at a time or all at once. The arrow keys move the selection and a status's key assigns it. Add items, set quantities and notes, move things between containers. |
| **Library** | The master list of items, each with a category, the place it is stored when not in use, and whether it is a container; and the categories and storage locations themselves. |
| **Settings** (gear) | The statuses: name, colour and shortcut key. A reference of the keys a list answers to. |

The specification this was built from is [spec.md](spec.md); what was decided
where it leaves a question open is under [Assumptions](#assumptions); the HTTP
API is documented in [docs/api.md](docs/api.md).

## Install on Debian 13

```bash
cd packrat
sudo deploy/install.sh                # builds with the local Go toolchain (Go 1.26+)
```

or, to build elsewhere and install a prebuilt binary:

```bash
make dist                             # -> dist/packrat (static linux/amd64 binary)
sudo deploy/install.sh dist/packrat
```

The script is idempotent (re-run it to upgrade) and:

1. installs `postgresql` from Debian's repositories if it is missing (set
   `PACKRAT_SKIP_PACKAGES=1` to skip);
2. creates a system user `packrat`, and a PostgreSQL role and database of the
   same name. The service connects over PostgreSQL's local socket by *peer
   authentication*: the OS user `packrat` is let in as the database role
   `packrat`, so there is no database password to keep anywhere;
3. installs `/usr/local/bin/packrat`, `/etc/systemd/system/packrat.service`
   and `/etc/packrat/packrat.env`;
4. starts the service, which creates (or upgrades) the schema on start.

Then open `http://<server>:8082/`. A fresh install has three statuses (In
Storage Location, Packed, Missing) and nothing else: add items under
**Library**, then make a list.

```bash
systemctl status packrat              # is it running
journalctl -u packrat -f              # logs
sudoedit /etc/packrat/packrat.env     # configuration; then: sudo systemctl restart packrat
sudo deploy/uninstall.sh              # remove the service (keeps the database; --purge drops it)
```

### Configuration

`/etc/packrat/packrat.env` (see
[deploy/packrat.env.example](deploy/packrat.env.example)):

| Variable | Default | |
|---|---|---|
| `PACKRAT_ADDR` | `0.0.0.0:8082` | Listen address. (8080 and 8081 are the planner's and Squirrel's.) |
| `PACKRAT_DATABASE_URL` | `postgres:///packrat?host=/var/run/postgresql` | PostgreSQL connection. |

### Security

Packrat has no login: anyone who can reach the port can read and change
everything, by design for a tool reached only over the local network or a
VPN (spec.md, Security). Do not expose the port to the internet. It serves
plain HTTP; to put it behind TLS, set `PACKRAT_ADDR=127.0.0.1:8082` and point
a reverse proxy at it. Nothing is encrypted at rest beyond what the disk
does.

The service runs as its own unprivileged user under systemd's sandboxing
(see [deploy/packrat.service](deploy/packrat.service)): read-only file
system, no home directories, no capabilities.

### Backup

Everything is in the one database:

```bash
sudo -u postgres pg_dump -Fc packrat > packrat-$(date +%F).dump
sudo -u postgres pg_restore --clean --if-exists -d packrat packrat-2026-11-16.dump
```

## Using it

**The library** holds each thing you own once: its name (unique across the
library), category, storage location, and whether it is a *container*,
something other items can be packed inside.

**A list** places items from the library: how many, in what unit if not a
simple count, with what status, and inside which container. Containers nest
as deep as you like: a car holding a backpack holding a toiletry kit. Two
rules keep that a tree:

- A container is on a list **once**: "inside the Blue Backpack" has to mean
  one place. Move it rather than add it again.
- Anything else can be on a list **several times**, in different containers
  (socks in the suitcase and a spare pair in the day bag), but only once per
  container: change the quantity instead.

A container's status is its own. Packing everything inside the backpack does
not mark the backpack packed, and marking the backpack does not touch what is
inside.

**Templates** are ordinary lists with a flag saying they are meant to be
copied from. The flag changes two things: templates are kept apart on the
Lists page and offered first in *New list*; and a list made from a template
starts with the template's statuses (on a template, an entry's status is the
default for lists made from it), whereas entries taken from an ordinary list
start at the default status.

**New list** starts empty, or from any number of other lists. One list gives
a copy of it; several are combined into everything that is on any of them
(their union), and the dialog says how many entries that comes to before the
list is made:

- The same item in the same container on two lists is one entry, with the
  larger of the two quantities (when they count in the same unit) and the
  unit, status and notes of the list ticked first.
- A container goes where the first list ticked puts it, and what the other
  lists pack into it follows it there.

### Keys on a list

With no text box in use:

| Key | |
|---|---|
| `←` `↑` `→` `↓` | Move the selection to the nearest entry in that direction: up and down a list, across to the box beside it. |
| `Home` `End` | The first / last entry. |
| `Space` | Open or close the selected container. |
| `Enter` | Edit the selected entry's quantity. |
| `Esc` | Select the container the selection is in. (In a text box: leave it.) |
| a status's key | Give the selected entry that status. |
| `Shift` + a status's letter | ...and move on to the next entry. |

A status's key is one character, matched without regard to Shift or Caps
Lock, so `p` and `P` are the same shortcut. While the cursor is in a text box
(quantity, unit, notes, the filter) keys are text; `Enter` or `Esc` hands
them back to the list. The same things can be done with the mouse: click an
entry's status chip, or select an entry and click a status in the legend in
the title bar.

Which containers are closed is remembered per list in the browser, as is the
sort order (containers first, then by name, category, storage location or
status; chosen from the ⋯ menu).

## Assumptions

Decisions made where spec.md leaves a question open. Each is small to change;
the place to change it is named.

1. **A list made from several lists is their union**: everything on any of
   them (spec.md said "intersection"; confirmed as a slip). Where the lists
   disagree, the one ticked first wins: it places the containers and supplies
   the unit, status and notes of an entry they share, which gets the larger
   quantity. (`internal/store/merge.go`.)
2. **A container is on a list at most once.** `List_Contents.Container_ID`
   refers to `Items.ID` (as specified), not to another `List_Contents` row,
   so a container that was on a list twice would show its contents in both
   places. The store refuses a second placement of a container, refuses
   moving a container into itself or its own contents, and only lets
   `Is_Container` change while that keeps every list sound.
   (`internal/store/contents.go`, `guardContainerFlag` in `ref.go`.)
3. **Only items flagged `Is_Container` can hold things.** "Any item can be a
   container" is taken to mean any item can be flagged as one (in the
   Library), not that the flag is ignored.
4. **Templates carry their statuses; ordinary lists do not** when a new list
   is made from them (see Using it). (`statusFor` in `merge.go`.)
5. **The default status is the first one** (lowest ID): on a fresh install,
   In Storage Location. The schema has no "default" column and none was
   added. Three statuses are seeded (the spec's examples) so a new install
   works straight away; they can be renamed, re-keyed or deleted.
6. **Shortcuts are case-insensitive** and stored in lower case; any single
   visible character is allowed. With Shift, a letter shortcut also advances
   the selection (an addition, for packing down a list quickly).
7. **One view instead of a list and a treemap.** spec.md asks for an
   indented list plus a treemap; at your direction both are replaced by a
   single view that is both: containers as boxes within boxes (the treemap's
   nesting and colour-coding by status) with the items in each box as a list
   carrying all their fields. A box is as big as what is in it; there is
   nothing in the schema to weigh entries by. (`static/js/list/boxes.js`.)
8. **Arrow keys move by position**: to the nearest entry in that direction,
   since the page is two-dimensional. Space opens and closes a container.
9. **There is no manual ordering of entries**: the schema has no order
   column. Entries are sorted within their container, containers first, by a
   chosen key.
10. **Removing a container from a list** asks whether to remove what is inside
    it too or keep it (moving it up one level). Removing a plain entry is
    immediate, with an Undo.
11. **Deleting is refused while something is in use**: an item that is on a
    list, a category or storage location that has items, a status that
    entries have. Deleting a list deletes its entries and leaves the library
    alone.
12. **Statuses live under Settings**; categories and storage locations sit
    with the items on the Library page ("one page to manage this").
13. **Port 8082**, a system user and database named `packrat`, peer
    authentication over the local socket: the same arrangement as Squirrel.
14. **No real-time sync between browsers.** A list is re-read when its tab
    regains focus, which covers packing on a phone and checking on a laptop.

Not done, deliberately: drag-and-drop (moving is a menu), selecting several
entries at once, and any import or export.

## Database

PostgreSQL 15 or later (Debian 13 ships 17). The schema is
[internal/db/migrations/0001_schema.sql](internal/db/migrations/0001_schema.sql),
table for table as in spec.md; `0002_seed.sql` adds the three starting
statuses. Migrations are applied on start, each in its own transaction, and
recorded in `Schema_Migrations`. A released migration is never edited: schema
changes are new files.

Identifiers are not double-quoted, so PostgreSQL folds them to lower case:
`SELECT Name FROM Items` and `select name from items` are the same query.

### Departures from spec.md

Corrections of what look like slips (marked `[~]` in the schema):

| Where | spec.md | Here | Why |
|---|---|---|---|
| `List_Contents.Container_ID` | `smallint` | `int` | It refers to `Items.ID`, which is `serial` (int). |
| `List_Contents` primary key and unique constraint | named `PK_Items`, `UN_Items` | `PK_List_Contents`, `UN_List_Contents` | The names are already taken by `Items`; PostgreSQL would refuse the table. |
| `FK_List_Contents_Status_ID` | references `Status.ID` | references `Statuses (ID)` | The table is `Statuses`. |
| `UN_List_Contents` | `UNIQUE (List_ID, Container_ID, Item_ID)` | `UNIQUE NULLS NOT DISTINCT (...)` | Otherwise the rule would not hold at the top level, where `Container_ID` is null (nulls count as distinct by default). |

Additions (marked `[+]`):

| What | Why |
|---|---|
| `CK_Statuses_Color`: six lower-case hex digits | So a colour is always drawable. Stored without the `#`, as specified; the API speaks `#rrggbb`. |
| `CK_Statuses_Shortcut`: one visible character, lower case | See assumption 6. |
| `CK_List_Contents_Quantity`: `Quantity > 0` | A quantity of nothing is not an entry. |
| `CK_List_Contents_Container`: `Container_ID <> Item_ID` | Nothing is inside itself. |
| Indexes on the foreign-key columns | So "is this in use?" checks and joins do not scan. |
| `Schema_Migrations` table | Records which migrations have been applied. |

The rules of assumption 2 span rows and tables, so they are enforced by the
store rather than by constraints; anything that writes to `List_Contents`
directly must keep them itself.

## Development

Needs Go 1.26+, a local PostgreSQL, and Node.js for the browser tests.

```bash
createdb packrat_dev && createdb packrat_test && createdb packrat_e2e

make demo          # wipe packrat_dev and load the demo library and lists (scripts/demo-data.sql)
make run           # serve it on http://127.0.0.1:8082, UI straight from static/ (edit and reload)

export PACKRAT_TEST_DATABASE_URL='postgres:///packrat_test?host=/var/run/postgresql'
make test          # go vet + go test; the database tests skip without the variable

export PACKRAT_E2E_DATABASE_URL='postgres:///packrat_e2e?host=/var/run/postgresql'
make e2e           # Playwright, against a freshly built binary and the demo data
```

The UI is plain ES modules and CSS, with no build step and no dependencies;
the binary embeds `static/`. The look (theme tokens, title bar, grid table,
pinned header band, live-control editing) is the planner's and Squirrel's.

```
main.go                    flags, migrations on start, HTTP server
internal/db/               connection pool; migrations/*.sql (embedded)
internal/store/            data access
    ref.go                 categories, storage locations, statuses, items (one generic implementation)
    lists.go               lists: create (empty or from other lists), rename, delete
    contents.go            a list's entries, and the rules that keep them a tree
    merge.go               combining lists into a new one: their union (pure functions)
internal/api/              JSON handlers over the store
internal/testdb/           a throwaway schema per test
cmd/devtool/               reset / load demo data into a scratch database
deploy/                    install.sh, uninstall.sh, the systemd unit, the env example
static/
    js/app.js              hash router and page lifecycle
    js/state.js            the library and statuses, cached and kept current
    js/refedit.js          editable tables (Library, Settings)
    js/widgets.js          popover, menu, combo box, multi-select, modal
    js/newlist.js          the New list dialog
    js/pages/              lists.js, list.js (the list page's controller), library.js, settings.js
    js/list/model.js       entries -> tree, sorting, filtering
    js/list/boxes.js       the view of a list: boxes within boxes, movement by position
e2e/                       Playwright tests
```
