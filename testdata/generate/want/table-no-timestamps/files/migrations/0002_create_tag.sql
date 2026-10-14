-- alder migration 0002_create_tag
-- from 2e82b15d5f85
-- to 3a2321805480
-- step: create table Tag

CREATE TABLE "Tag__Tag" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
