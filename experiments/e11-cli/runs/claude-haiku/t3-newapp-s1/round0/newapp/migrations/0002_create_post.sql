-- alder migration 0002_create_post
-- from 0e812c25c044
-- to 2a9504a1b513
-- step: create table Post
-- step: add index on Post.author

CREATE TABLE "Post__Post" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "slug" TEXT NOT NULL UNIQUE, "body" TEXT NOT NULL, "published" INTEGER NOT NULL CHECK ("published" IN (0, 1)), "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
