-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 009362a101f6
-- step: create table Note
-- step: create table Tag
-- step: add index on Tag.note

CREATE TABLE "Note__Note" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Tag__Tag" ("id" TEXT NOT NULL PRIMARY KEY, "note" TEXT NOT NULL REFERENCES "Note__Note"("id") ON DELETE CASCADE, "label" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Tag__Tag__note" ON "Tag__Tag"("note");
