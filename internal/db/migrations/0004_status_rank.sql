-- Packrat schema, version 4: statuses are ranked.
--
-- Rank orders the statuses from the worst (1) to the best: the order they
-- are shown in (the legend, the menus, the chips that say what is inside a
-- container) and sorted by. It is an order and nothing more: the numbers
-- need not be consecutive (deleting a status leaves a gap).
--
-- Existing statuses are ranked by a guess that suits the seeded three:
-- "Missing" is the worst, "Packed" the best, anything else in between in the
-- order it was made. Settings > Statuses puts them in any other order.
--
-- The constraint is deferred so that a reordering can pass through states in
-- which two statuses share a rank, as long as none do when it commits.

ALTER TABLE Statuses ADD COLUMN Rank smallint;

UPDATE Statuses s SET Rank = r.Rank
FROM (
    SELECT ID, row_number() OVER (
        ORDER BY CASE lower(Name) WHEN 'missing' THEN 0 WHEN 'packed' THEN 2 ELSE 1 END, ID
    ) AS Rank
    FROM Statuses
) r
WHERE r.ID = s.ID;

ALTER TABLE Statuses
    ALTER COLUMN Rank SET NOT NULL,
    ADD CONSTRAINT UN_Statuses_Rank UNIQUE (Rank) DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT CK_Statuses_Rank CHECK (Rank > 0);
