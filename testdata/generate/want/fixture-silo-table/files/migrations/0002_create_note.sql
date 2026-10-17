-- alder migration 0002_create_note
-- from 1e805e781e3e
-- to 5b60eb6ae5ef
-- step: create table Note
-- step: add index on Note.org
-- step: add index on Note.project

CREATE TABLE "Note__Note" ("id" TEXT NOT NULL PRIMARY KEY, "text" TEXT NOT NULL, "project" TEXT NOT NULL REFERENCES "Project__Project"("id") ON DELETE CASCADE, "org" TEXT NOT NULL REFERENCES "Org__Org"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Note__Note__org" ON "Note__Note"("org");
CREATE INDEX IF NOT EXISTS "Note__Note__project" ON "Note__Note"("project");
