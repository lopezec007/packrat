-- Seed data: the statuses a new installation starts with, so that a list can
-- be filled in straight away (every List_Contents row needs a status). They
-- are ordinary rows: rename, recolour, re-key, add to or delete them in
-- Settings > Statuses.
--
-- The lowest-numbered status is the one a newly added item starts with.

INSERT INTO Statuses (Name, Color, Shortcut) VALUES
    ('In Storage Location', '8a94a3', 's'),
    ('Packed',              '2e8540', 'p'),
    ('Missing',             'd62828', 'm');
