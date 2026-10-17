-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 0c3d1a116fa6
-- step: create table Post

CREATE TABLE "Post__Post" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "slug" TEXT NOT NULL UNIQUE, "body" TEXT NOT NULL, "published" INTEGER NOT NULL CHECK ("published" IN (0, 1)), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
