-- alder migration 0002_create_note
-- from 1e805e781e3e
-- to 7f387839a537
-- step: create table Note
-- step: add index on Note.org
-- step: add index on Note.project

CREATE TABLE "Table__Note__Note" ("id" TEXT NOT NULL PRIMARY KEY, "text" TEXT NOT NULL, "project" TEXT NOT NULL REFERENCES "Project__Project"("id") ON DELETE CASCADE, "org" TEXT NOT NULL REFERENCES "Org__Org"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Table__Note__Note__org" ON "Table__Note__Note"("org");
CREATE INDEX IF NOT EXISTS "Table__Note__Note__project" ON "Table__Note__Note"("project");
