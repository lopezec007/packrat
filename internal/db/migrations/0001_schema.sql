-- Packrat schema, version 1.
--
-- Naming follows spec.md (Title_Case tables and columns; PK_/UN_/FK_ constraint
-- names). The identifiers are deliberately NOT double-quoted, so PostgreSQL
-- folds them to lower case in the catalogue: `SELECT Name FROM Items` and
-- `select name from items` are the same query, and psql's \d shows the
-- lower-case names. (Quoting them would make every query need the quotes.)
--
-- Where this file departs from spec.md the line is marked "-- [+]" (an
-- addition the requirements need) or "-- [~]" (a correction of what looks
-- like a typo). README.md lists every one with its reason.


-- ---------------------------------------------------------------------------
-- Lists: the packing/equipment lists
-- ---------------------------------------------------------------------------
CREATE TABLE Lists (
    ID          smallserial  NOT NULL,
    Name        varchar(127) NOT NULL,               -- e.g. "Park Day", "Paris 2027"
    Is_Template bool         NOT NULL,
    CONSTRAINT PK_Lists PRIMARY KEY (ID),
    CONSTRAINT UN_Lists UNIQUE (Name)
);


-- ---------------------------------------------------------------------------
-- The library: categories, storage locations and the items themselves
-- ---------------------------------------------------------------------------
CREATE TABLE Item_Categories (
    ID   smallserial  NOT NULL,
    Name varchar(127) NOT NULL,                      -- e.g. "Clothing", "Electronics", "Camp"
    CONSTRAINT PK_Item_Categories PRIMARY KEY (ID),
    CONSTRAINT UN_Item_Categories UNIQUE (Name)
);

-- Where an item lives when it is not in use.
CREATE TABLE Storage_Locations (
    ID   smallserial  NOT NULL,
    Name varchar(127) NOT NULL,                      -- e.g. "Middle TV Cabinet", "Bottom Desk Drawer"
    CONSTRAINT PK_Storage_Locations PRIMARY KEY (ID),
    CONSTRAINT UN_Storage_Locations UNIQUE (Name)
);

CREATE TABLE Items (
    ID                  serial       NOT NULL,
    Item_Category_ID    smallint     NOT NULL,
    Name                varchar(127) NOT NULL,       -- e.g. "Comb", "Lantern, Gas", "Socks"
    Storage_Location_ID smallint     NOT NULL,
    Is_Container        bool         NOT NULL,       -- other items can be packed inside it
    CONSTRAINT PK_Items PRIMARY KEY (ID),
    CONSTRAINT UN_Items UNIQUE (Name),               -- across all categories
    CONSTRAINT FK_Items_Item_Category_ID FOREIGN KEY (Item_Category_ID) REFERENCES Item_Categories (ID),
    CONSTRAINT FK_Items_Storage_Location_ID FOREIGN KEY (Storage_Location_ID) REFERENCES Storage_Locations (ID)
);
CREATE INDEX IX_Items_Item_Category_ID ON Items (Item_Category_ID);          -- [+]
CREATE INDEX IX_Items_Storage_Location_ID ON Items (Storage_Location_ID);    -- [+]


-- ---------------------------------------------------------------------------
-- Statuses: where an item on a list stands ("Missing", "Packed", ...)
-- ---------------------------------------------------------------------------
CREATE TABLE Statuses (
    ID       smallserial  NOT NULL,
    Name     varchar(127) NOT NULL,
    Color    char(6)      NOT NULL,                  -- hex colour code without the "#": "2e8540"
    Shortcut char(1)      NOT NULL,                  -- the key that assigns this status
    CONSTRAINT PK_Statuses PRIMARY KEY (ID),
    CONSTRAINT UN_Statuses UNIQUE (Name),
    CONSTRAINT UN_Statuses_Shortcut UNIQUE (Shortcut),
    CONSTRAINT CK_Statuses_Color CHECK (Color ~ '^[0-9a-f]{6}$'),                              -- [+] lower-case hex
    -- [+] one visible character, stored in lower case: the key is matched
    -- whatever the state of Shift or Caps Lock, so "p" and "P" are one shortcut.
    CONSTRAINT CK_Statuses_Shortcut CHECK (Shortcut ~ '^\S$' AND Shortcut = lower(Shortcut))
);


-- ---------------------------------------------------------------------------
-- List_Contents: what is on each list, and inside what
-- ---------------------------------------------------------------------------
-- A row places an item on a list, either at the top level (Container_ID null)
-- or inside a container. The container is named by its item (as spec.md has
-- it), so the tree is only well defined while each container item is on a
-- list at most once and containers do not hold one another in a circle; the
-- store (internal/store/contents.go) enforces both in the transaction that
-- changes a list.
CREATE TABLE List_Contents (
    ID           serial   NOT NULL,
    List_ID      smallint NOT NULL,
    Container_ID int,                                -- [~] int, not smallint: it refers to Items.ID (serial). Null = top level of the list
    Item_ID      int      NOT NULL,
    Quantity     int      NOT NULL,
    Unit         varchar(127),                       -- when Quantity is not a simple count: "kg", "pairs"
    Status_ID    smallint NOT NULL,                  -- on a template: the status a list made from it starts with
    Notes        varchar(255),
    CONSTRAINT PK_List_Contents PRIMARY KEY (ID),    -- [~] spec.md repeats the name PK_Items here
    -- [~] spec.md repeats the name UN_Items here. NULLS NOT DISTINCT so that the
    -- rule also holds at the top level (where Container_ID is null).
    CONSTRAINT UN_List_Contents UNIQUE NULLS NOT DISTINCT (List_ID, Container_ID, Item_ID),
    CONSTRAINT FK_List_Contents_List_ID FOREIGN KEY (List_ID) REFERENCES Lists (ID),
    CONSTRAINT FK_List_Contents_Container_ID FOREIGN KEY (Container_ID) REFERENCES Items (ID),
    CONSTRAINT FK_List_Contents_Item_ID FOREIGN KEY (Item_ID) REFERENCES Items (ID),
    CONSTRAINT FK_List_Contents_Status_ID FOREIGN KEY (Status_ID) REFERENCES Statuses (ID),   -- [~] spec.md says "Status.ID"
    CONSTRAINT CK_List_Contents_Quantity CHECK (Quantity > 0),                                 -- [+]
    CONSTRAINT CK_List_Contents_Container CHECK (Container_ID <> Item_ID)                      -- [+] nothing is inside itself
);
CREATE INDEX IX_List_Contents_Item_ID ON List_Contents (Item_ID);            -- [+]
CREATE INDEX IX_List_Contents_Container_ID ON List_Contents (Container_ID);  -- [+]
CREATE INDEX IX_List_Contents_Status_ID ON List_Contents (Status_ID);        -- [+]
