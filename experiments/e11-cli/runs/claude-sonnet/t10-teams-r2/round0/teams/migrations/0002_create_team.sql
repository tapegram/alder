-- alder migration 0002_create_team
-- from 0e812c25c044
-- to 0cc45b67cebe
-- step: create table Team
-- step: add index on Team.owner

CREATE TABLE "Team__Team" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "owner" TEXT NOT NULL REFERENCES "User__User"("id")) STRICT;
CREATE INDEX IF NOT EXISTS "Team__Team__owner" ON "Team__Team"("owner");
