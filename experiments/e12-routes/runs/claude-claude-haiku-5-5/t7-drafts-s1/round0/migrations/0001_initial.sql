-- alder migration 0001_initial
-- from e3b0c44298fc
-- to f2e7075fb0c3
-- step: create table Comment
-- step: create table Post
-- step: create table User
-- step: add index on Comment.post
-- step: add index on Post.author

CREATE TABLE "Comment__Comment" ("id" TEXT NOT NULL PRIMARY KEY, "post" TEXT NOT NULL REFERENCES "Post__Post"("id") ON DELETE CASCADE, "author" TEXT NOT NULL, "body" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Post__Post" ("id" TEXT NOT NULL PRIMARY KEY, "slug" TEXT NOT NULL UNIQUE, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT;
CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "role" TEXT NOT NULL CHECK ("role" IN ('Reader', 'Author')), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post");
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
