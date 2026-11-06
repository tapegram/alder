-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 4efb988064dc
-- step: create table Post

CREATE TABLE "Post__Post" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
