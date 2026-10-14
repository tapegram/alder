-- alder migration 0002_create_note
-- from 03b9bc832991
-- to 65ef7eb319a3
-- step: create table Note
-- step: add index on Note.owner

CREATE TABLE "Note__Note" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Note__Note__owner" ON "Note__Note"("owner");
