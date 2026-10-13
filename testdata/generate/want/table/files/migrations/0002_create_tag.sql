-- alder migration 0002_create_tag
-- from 2e82b15d5f85
-- to 3e9820702e78
-- step: create table Tag
-- step: add index on Tag.entry

CREATE TABLE "Tag__Tag" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL UNIQUE, "entry" TEXT NOT NULL REFERENCES "Entry__Entry"("id") ON DELETE CASCADE, "note" TEXT, "pinned" INTEGER NOT NULL CHECK ("pinned" IN (0, 1)), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Tag__Tag__entry" ON "Tag__Tag"("entry");
