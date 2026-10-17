-- alder migration 0002_silo_org
-- from 1e805e781e3e
-- to 1e805e781e3e
-- step: move table Activity into silo org
-- DROPS DATA: shared rows of Activity: it moves into silo org, and the rows the shared database holds of it stay there, unread

CREATE TABLE IF NOT EXISTS "Activity__Activity" ("id" TEXT NOT NULL PRIMARY KEY, "text" TEXT NOT NULL, "org" TEXT NOT NULL REFERENCES "Org__Org"("id") ON DELETE CASCADE, "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Activity__Activity__org" ON "Activity__Activity"("org");
