-- alder migration 0002_create_tag
-- from 9b787a621da5
-- to 48d2ebe47058
-- step: create table Tag
-- step: add index on Tag.entry

CREATE TABLE "Table__Tag__Tag" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL UNIQUE, "entry" TEXT NOT NULL REFERENCES "Table__Entry__Entry"("id") ON DELETE CASCADE, "note" TEXT, "pinned" INTEGER NOT NULL CHECK ("pinned" IN (0, 1)), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Table__Tag__Tag__entry" ON "Table__Tag__Tag"("entry");
