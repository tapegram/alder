-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 0e16e2731724
-- step: create table Comment
-- step: create table Post
-- step: create table Tag
-- step: create table User
-- step: add index on Comment.author
-- step: add index on Comment.post
-- step: add index on Post.author
-- step: add index on Tag.post

CREATE TABLE "Comment__Comment" ("id" TEXT NOT NULL PRIMARY KEY, "post" TEXT NOT NULL REFERENCES "Post__Post"("id") ON DELETE CASCADE, "author" TEXT NOT NULL REFERENCES "User__User"("id"), "body" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Post__Post" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "status" TEXT NOT NULL CHECK ("status" IN ('Draft', 'Published')), "featured" INTEGER NOT NULL CHECK ("featured" IN (0, 1)), "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE TABLE "Tag__Tag" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL UNIQUE, "post" TEXT NOT NULL REFERENCES "Post__Post"("id"), "created" TEXT NOT NULL) STRICT;
CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "role" TEXT NOT NULL CHECK ("role" IN ('Reader', 'Admin')), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Comment__Comment__author" ON "Comment__Comment"("author");
CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post");
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
CREATE INDEX IF NOT EXISTS "Tag__Tag__post" ON "Tag__Tag"("post");
