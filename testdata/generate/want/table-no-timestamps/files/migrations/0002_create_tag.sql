-- alder migration 0002_create_tag
-- from 9b787a621da5
-- to 90b019401350
-- step: create table Tag

CREATE TABLE "Table__Tag__Tag" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
