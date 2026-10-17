-- alder migration 0004_create_invite
-- from 187cc545774c
-- to 9ed332038898
-- step: create table Invite
-- step: add index on Invite.team

CREATE TABLE "Invite__Invite" ("id" TEXT NOT NULL PRIMARY KEY, "team" TEXT NOT NULL REFERENCES "Team__Team"("id"), "email" TEXT NOT NULL, "status" TEXT NOT NULL CHECK ("status" IN ('Pending', 'Accepted', 'Declined'))) STRICT;
CREATE INDEX IF NOT EXISTS "Invite__Invite__team" ON "Invite__Invite"("team");
