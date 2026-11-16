# Packrat
PACKRAT: Packing Assistant for Camping, Kit, Roadtrips And Travel



# Requirements
## Infrastructure
The tool must be installable on Debian 13 as a service.  It should run direclty on the OS; not in a container or VM.



## Security
The tool will only be accessed over the local network or VPN, so HTTP-only is acceptable to start.  Data will reside
on a LUKS-encrypted server I control, so it doesn't need to be further encrypted at rest.



## Database
The database should be built on PostgreSQL.  Table details (column assumed to be nullable when nullability is not
specified):


### Lists
Enum of packing/equipment lists.

Columns:
- `ID smallserial NOT NULL`
- `Name varchar(127) NOT NULL` (e.g. "Park Day", "Paris 2027")
- `Is_Template bool NOT NULL`

Constraints:
- `PK_Lists PRIMARY KEY (ID)`
- `UN_Lists UNIQUE (Name)`


### Item_Categories
Enum of item categories.

Columns:
- `ID smallserial NOT NULL`
- `Name varchar(127) NOT NULL` (e.g. "Clothing", "Electronics", "Camp")

Constraints:
- `PK_Item_Categories PRIMARY KEY (ID)`
- `UN_Item_Categories UNIQUE (Name)`


### Storage_Locations
Enum of item storage locations (for when item is not in use).

Columns:
- `ID smallserial NOT NULL`
- `Name varchar(127) NOT NULL` (e.g. "Middle TV Cabinet", "Bottom Desk Drawer")

Constraints:
- `PK_Storage_Locations PRIMARY KEY (ID)`
- `UN_Storage_Locations UNIQUE (Name)`


### Items
Enum of individual items.

Columns:
- `ID serial NOT NULL`
- `Item_Category_ID smallint NOT NULL`
- `Name varchar(127) NOT NULL` (e.g. "Comb", "Lantern, Gas", "Socks")
- `Storage_Location_ID smallint NOT NULL`
- `Is_Container bool NOT NULL`

Constraints:
- `PK_Items PRIMARY KEY (ID)`
- `UN_Items UNIQUE (Name)` (should not include Item_Category_ID)
- `FK_Items_Item_Category_ID FOREIGN KEY (Item_Category_ID) REFERENCES Item_Categories.ID`
- `FK_Items_Storage_Location_ID FOREIGN KEY (Storage_Location_ID) REFERENCES Storage_Locations.ID`


### Statuses
Enum of statuses.

Columns:
- `ID smallserial NOT NULL`
- `Name varchar(127) NOT NULL` (e.g. "Missing", "In Storage Location", "Packed")
- `Color char(6) NOT NULL`  (hex color code)
- `Shortcut char(1) NOT NULL`

Constraints:
- `PK_Statuses PRIMARY KEY (ID)`
- `UN_Statuses UNIQUE (Name)`
- `UN_Statuses_Shortcut UNIQUE (Shortcut)`


### List_Contents
Actual contents of each list.

Columns:
- `ID serial NOT NULL`
- `List_ID smallint NOT NULL`
- `Container_ID smallint`  (null indicates top level for list)
- `Item_ID int NOT NULL`
- `Quantity int NOT NULL`
- `Unit varchar(127)`  (for when Quantity is not a simple count (e.g. "kg", "pairs", etc.))
- `Status_ID smallint NOT NULL`  (serves as the default status if list is a template)
- `Notes varchar(255)`

Constraints:
- `PK_Items PRIMARY KEY (ID)`
- `UN_Items UNIQUE (List_ID, Container_ID, Item_ID)`
- `FK_List_Contents_List_ID FOREIGN KEY (List_ID) REFERENCES Lists.ID`
- `FK_List_Contents_Container_ID FOREIGN KEY (Container_ID) REFERENCES Items.ID`
- `FK_List_Contents_Item_ID FOREIGN KEY (Item_ID) REFERENCES Items.ID`
- `FK_List_Contents_Status_ID FOREIGN KEY (Status_ID) REFERENCES Status.ID`



## General UI
See the adjacent `planner` and `squirrel` repos for UI elements and CSS guidance.  I'm very happy with how those look,
so this tool should generally match them.  The same tech stack should work here (other than DB -- `planner` uses
SQLite).

The objective of this tool is to have a single master "library" of items that lists can draw from.  We'll need one page
to manage this.

The user should be able to create a new list either from scratch or from the intersection of any number of other lists
("templates" or not).  The only purpose of the "template" flag is to indicate that that list is intended to be used as
a template.

### Lists
A single "list" is comprised of items.  Any item can be a container, which allows arbitrarily deep nesting.  E.g. for a
"Road Trip" list, "Car" might be the top-level container with various backpacks and bins as children.  Each of these in
turn has its own children, some of which (maybe a hygiene kit in a suitcase) have their own children.

The default view should be a hierarchical list with children indented and parents ("containers") collapsible.  Ensure
the expand-all and collapse-all functionality found in `planner` is included.

Each list should have an additional treemap view with items color-coded by status.

In both the list and treemap views, the arrow keys should navigate, and pressing the key assigned to `Shortcut` should
assign that status to the selected item.  Containers should *not* automatically change status based on their children.
