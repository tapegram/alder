-- alder migration 0002_create_post
-- from 0e812c25c044
-- to c0817625f138
-- step: create table Post
-- step: add index on Post.author

CREATE TABLE "Post__Post" ("id" TEXT NOT NULL PRIMARY KEY, "slug" TEXT NOT NULL UNIQUE, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "published" INTEGER NOT NULL CHECK ("published" IN (0, 1)), "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
