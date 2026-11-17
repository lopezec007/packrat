-- Made-up data for development and the end-to-end tests: a small library and
-- a few lists. Loaded by `make demo` (cmd/devtool) into a scratch database
-- whose name ends in _dev, _test or _e2e; never into the real one.
--
-- Rows are written by name and joined to their IDs, so the file reads like
-- the lists it describes. The check at the end fails the load if a name is
-- misspelt (a row that joins to nothing would otherwise vanish silently).

INSERT INTO Item_Categories (Name) VALUES
    ('Bags'), ('Camp'), ('Clothing'), ('Documents'), ('Electronics'), ('Food & Kitchen'), ('Hygiene'), ('Vehicle');

INSERT INTO Storage_Locations (Name) VALUES
    ('Bathroom Cabinet'), ('Bedroom Dresser'), ('Bottom Desk Drawer'), ('Driveway'), ('Garage Shelf A'),
    ('Garage Shelf B'), ('Hall Closet'), ('Kitchen Pantry'), ('Middle TV Cabinet');

CREATE TEMP TABLE demo_items (name text, cat text, loc text, cont bool);
INSERT INTO demo_items VALUES
    ('Car',                 'Vehicle',        'Driveway',           true),
    ('Blue Backpack',       'Bags',           'Hall Closet',        true),
    ('Daypack',             'Bags',           'Hall Closet',        true),
    ('Suitcase, Large',     'Bags',           'Garage Shelf B',     true),
    ('Camp Bin',            'Bags',           'Garage Shelf A',     true),
    ('Cooler',              'Food & Kitchen', 'Garage Shelf A',     true),
    ('Hygiene Kit',         'Hygiene',        'Bathroom Cabinet',   true),
    ('First Aid Kit',       'Hygiene',        'Hall Closet',        false),
    ('Tent, 3-Person',      'Camp',           'Garage Shelf A',     false),
    ('Sleeping Bag',        'Camp',           'Garage Shelf A',     false),
    ('Sleeping Pad',        'Camp',           'Garage Shelf A',     false),
    ('Lantern, Gas',        'Camp',           'Garage Shelf A',     false),
    ('Camp Stove',          'Camp',           'Garage Shelf A',     false),
    ('Fuel Canister',       'Camp',           'Garage Shelf B',     false),
    ('Headlamp',            'Camp',           'Bottom Desk Drawer', false),
    ('Camp Chair',          'Camp',           'Garage Shelf B',     false),
    ('Picnic Blanket',      'Camp',           'Hall Closet',        false),
    ('Frisbee',             'Camp',           'Hall Closet',        false),
    ('Water',               'Food & Kitchen', 'Kitchen Pantry',     false),
    ('Water Bottle',        'Food & Kitchen', 'Kitchen Pantry',     false),
    ('Snacks',              'Food & Kitchen', 'Kitchen Pantry',     false),
    ('Coffee, Ground',      'Food & Kitchen', 'Kitchen Pantry',     false),
    ('Mess Kit',            'Food & Kitchen', 'Garage Shelf A',     false),
    ('Socks',               'Clothing',       'Bedroom Dresser',    false),
    ('T-Shirt',             'Clothing',       'Bedroom Dresser',    false),
    ('Trousers',            'Clothing',       'Bedroom Dresser',    false),
    ('Rain Jacket',         'Clothing',       'Hall Closet',        false),
    ('Down Jacket',         'Clothing',       'Hall Closet',        false),
    ('Wool Hat',            'Clothing',       'Hall Closet',        false),
    ('Gloves',              'Clothing',       'Hall Closet',        false),
    ('Thermal Base Layer',  'Clothing',       'Bedroom Dresser',    false),
    ('Sun Hat',             'Clothing',       'Hall Closet',        false),
    ('Hand Warmers',        'Camp',           'Garage Shelf B',     false),
    ('Toothbrush',          'Hygiene',        'Bathroom Cabinet',   false),
    ('Toothpaste',          'Hygiene',        'Bathroom Cabinet',   false),
    ('Comb',                'Hygiene',        'Bathroom Cabinet',   false),
    ('Sunscreen',           'Hygiene',        'Bathroom Cabinet',   false),
    ('Passport',            'Documents',      'Bottom Desk Drawer', false),
    ('Train Tickets',       'Documents',      'Bottom Desk Drawer', false),
    ('Phone Charger',       'Electronics',    'Middle TV Cabinet',  false),
    ('Power Adapter, EU',   'Electronics',    'Middle TV Cabinet',  false),
    ('Camera',              'Electronics',    'Middle TV Cabinet',  false),
    ('Power Bank',          'Electronics',    'Middle TV Cabinet',  false);

INSERT INTO Items (Name, Item_Category_ID, Storage_Location_ID, Is_Container)
SELECT d.name, c.ID, l.ID, d.cont
FROM demo_items d
JOIN Item_Categories c ON c.Name = d.cat
JOIN Storage_Locations l ON l.Name = d.loc;

INSERT INTO Lists (Name, Is_Template) VALUES
    ('Camping Weekend', true),
    ('Cold Weather Add-on', true),
    ('Park Day', false),
    ('Paris 2027', false);

CREATE TEMP TABLE demo_contents (list text, container text, item text, qty int, unit text, status text, notes text);
INSERT INTO demo_contents VALUES
    -- Camping Weekend (template): everything rides in the car.
    ('Camping Weekend', NULL,            'Car',                1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Car',           'Camp Bin',           1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Camp Bin',      'Tent, 3-Person',     1, NULL,    'In Storage Location', 'Check the pole bag is inside'),
    ('Camping Weekend', 'Camp Bin',      'Sleeping Bag',       2, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Camp Bin',      'Sleeping Pad',       2, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Camp Bin',      'Lantern, Gas',       1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Camp Bin',      'Camp Stove',         1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Camp Bin',      'Fuel Canister',      2, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Camp Bin',      'Headlamp',           2, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Camp Bin',      'Mess Kit',           1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Car',           'Cooler',             1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Cooler',        'Water',              8, 'L',     'In Storage Location', NULL),
    ('Camping Weekend', 'Cooler',        'Snacks',             1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Cooler',        'Coffee, Ground',     250, 'g',   'In Storage Location', NULL),
    ('Camping Weekend', 'Car',           'Blue Backpack',      1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Blue Backpack', 'Rain Jacket',        1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Blue Backpack', 'Socks',              3, 'pairs', 'In Storage Location', NULL),
    ('Camping Weekend', 'Blue Backpack', 'T-Shirt',            2, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Blue Backpack', 'Hygiene Kit',        1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Hygiene Kit',   'Toothbrush',         1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Hygiene Kit',   'Toothpaste',         1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Hygiene Kit',   'Comb',               1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Hygiene Kit',   'Sunscreen',          1, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Car',           'Camp Chair',         2, NULL,    'In Storage Location', NULL),
    ('Camping Weekend', 'Car',           'First Aid Kit',      1, NULL,    'In Storage Location', NULL),

    -- Cold Weather Add-on (template): loose items to combine with another list.
    ('Cold Weather Add-on', NULL, 'Down Jacket',        1, NULL,    'In Storage Location', NULL),
    ('Cold Weather Add-on', NULL, 'Wool Hat',           1, NULL,    'In Storage Location', NULL),
    ('Cold Weather Add-on', NULL, 'Gloves',             1, 'pairs', 'In Storage Location', NULL),
    ('Cold Weather Add-on', NULL, 'Thermal Base Layer', 2, NULL,    'In Storage Location', NULL),
    ('Cold Weather Add-on', NULL, 'Hand Warmers',       6, NULL,    'In Storage Location', NULL),
    ('Cold Weather Add-on', NULL, 'Socks',              2, 'pairs', 'In Storage Location', 'The thick ones'),

    -- Park Day: one bag, partly packed.
    ('Park Day', NULL,            'Blue Backpack',  1, NULL, 'Packed',              NULL),
    ('Park Day', 'Blue Backpack', 'Water Bottle',   2, NULL, 'Packed',              NULL),
    ('Park Day', 'Blue Backpack', 'Sunscreen',      1, NULL, 'Packed',              NULL),
    ('Park Day', 'Blue Backpack', 'Snacks',         1, NULL, 'In Storage Location', NULL),
    ('Park Day', 'Blue Backpack', 'Frisbee',        1, NULL, 'Missing',             'Last seen in the car boot'),
    ('Park Day', NULL,            'Picnic Blanket', 1, NULL, 'In Storage Location', NULL),
    ('Park Day', NULL,            'Sun Hat',        2, NULL, 'Packed',              NULL),

    -- Paris 2027: a suitcase and a day bag.
    ('Paris 2027', NULL,              'Suitcase, Large',   1, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Suitcase, Large', 'T-Shirt',           5, NULL,    'Packed',              NULL),
    ('Paris 2027', 'Suitcase, Large', 'Trousers',          2, NULL,    'Packed',              NULL),
    ('Paris 2027', 'Suitcase, Large', 'Socks',             6, 'pairs', 'Packed',              NULL),
    ('Paris 2027', 'Suitcase, Large', 'Rain Jacket',       1, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Suitcase, Large', 'Hygiene Kit',       1, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Hygiene Kit',     'Toothbrush',        1, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Hygiene Kit',     'Toothpaste',        1, NULL,    'In Storage Location', 'Travel size'),
    ('Paris 2027', 'Hygiene Kit',     'Comb',              1, NULL,    'Packed',              NULL),
    ('Paris 2027', NULL,              'Daypack',           1, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Daypack',         'Passport',          1, NULL,    'Missing',             'Renewal sent off in March'),
    ('Paris 2027', 'Daypack',         'Train Tickets',     2, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Daypack',         'Phone Charger',     1, NULL,    'Packed',              NULL),
    ('Paris 2027', 'Daypack',         'Power Adapter, EU', 2, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Daypack',         'Camera',            1, NULL,    'In Storage Location', NULL),
    ('Paris 2027', 'Daypack',         'Power Bank',        1, NULL,    'Packed',              NULL),
    ('Paris 2027', 'Daypack',         'Socks',             1, 'pairs', 'Packed',              'Spare pair for the flight');

INSERT INTO List_Contents (List_ID, Container_ID, Item_ID, Quantity, Unit, Status_ID, Notes)
SELECT l.ID, c.ID, i.ID, d.qty, d.unit, s.ID, d.notes
FROM demo_contents d
JOIN Lists l ON l.Name = d.list
JOIN Items i ON i.Name = d.item
JOIN Statuses s ON s.Name = d.status
LEFT JOIN Items c ON c.Name = d.container;

DO $$
BEGIN
    IF (SELECT count(*) FROM Items) <> (SELECT count(*) FROM demo_items) THEN
        RAISE EXCEPTION 'demo-data.sql: an item names a category or storage location that does not exist';
    END IF;
    IF (SELECT count(*) FROM List_Contents) <> (SELECT count(*) FROM demo_contents) THEN
        RAISE EXCEPTION 'demo-data.sql: a list entry names a list, item or status that does not exist';
    END IF;
    IF EXISTS (SELECT 1 FROM demo_contents d WHERE d.container IS NOT NULL
               AND NOT EXISTS (SELECT 1 FROM Items i WHERE i.Name = d.container AND i.Is_Container)) THEN
        RAISE EXCEPTION 'demo-data.sql: a list entry names a container that is not a container item';
    END IF;
END $$;

DROP TABLE demo_items;
DROP TABLE demo_contents;
