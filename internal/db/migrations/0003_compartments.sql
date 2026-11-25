-- Packrat schema, version 3: compartments.
--
-- Many containers have compartments (a backpack's lid pocket, a car's boot).
-- A compartment belongs to one container item, and an entry that sits in a
-- container on a list can say which of the container's compartments it is
-- in. That is optional: with no compartment the entry is simply in the
-- container.
--
-- UN_List_Contents is unchanged: an item is still in a container at most
-- once on a list, whatever the compartment (not once per compartment).

CREATE TABLE Compartments (
    ID      serial       NOT NULL,
    Item_ID int          NOT NULL,                   -- the container the compartment is part of
    Name    varchar(127) NOT NULL,                   -- e.g. "Lid Pocket", "Main"
    CONSTRAINT PK_Compartments PRIMARY KEY (ID),
    CONSTRAINT UN_Compartments UNIQUE (Item_ID, Name),
    -- What FK_List_Contents_Compartment_ID points at: a compartment together
    -- with the container it belongs to.
    CONSTRAINT UN_Compartments_Item UNIQUE (ID, Item_ID),
    -- A compartment is part of its item and goes when the item does. (An item
    -- can only be deleted while it is on no list, so nothing is in its
    -- compartments by then.)
    CONSTRAINT FK_Compartments_Item_ID FOREIGN KEY (Item_ID) REFERENCES Items (ID) ON DELETE CASCADE
);

ALTER TABLE List_Contents
    ADD COLUMN Compartment_ID int,                   -- null = in the container, in no particular compartment
    -- The compartment must be one of the container the entry is in: the pair
    -- (Compartment_ID, Container_ID) has to exist in Compartments.
    ADD CONSTRAINT FK_List_Contents_Compartment_ID FOREIGN KEY (Compartment_ID, Container_ID) REFERENCES Compartments (ID, Item_ID),
    -- ...which a foreign key does not check while one of the two is null.
    ADD CONSTRAINT CK_List_Contents_Compartment CHECK (Compartment_ID IS NULL OR Container_ID IS NOT NULL);

CREATE INDEX IX_List_Contents_Compartment_ID ON List_Contents (Compartment_ID);
