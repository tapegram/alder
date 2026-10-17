-- alder migration 0003_create_member
-- from 0cc45b67cebe
-- to 187cc545774c
-- step: create table Member
-- step: add index on Member.team
-- step: add index on Member.user

CREATE TABLE "Member__Member" ("id" TEXT NOT NULL PRIMARY KEY, "team" TEXT NOT NULL REFERENCES "Team__Team"("id"), "user" TEXT NOT NULL REFERENCES "User__User"("id")) STRICT;
CREATE INDEX IF NOT EXISTS "Member__Member__team" ON "Member__Member"("team");
CREATE INDEX IF NOT EXISTS "Member__Member__user" ON "Member__Member"("user");
