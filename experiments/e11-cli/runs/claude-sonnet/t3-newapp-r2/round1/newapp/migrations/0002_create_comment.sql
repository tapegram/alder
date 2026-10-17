-- alder migration 0002_create_comment
-- from 0c3d1a116fa6
-- to 1651618c61bc
-- step: create table Comment
-- step: add index on Comment.post

CREATE TABLE "Comment__Comment" ("id" TEXT NOT NULL PRIMARY KEY, "post" TEXT NOT NULL REFERENCES "Post__Post"("id") ON DELETE CASCADE, "author" TEXT NOT NULL, "body" TEXT NOT NULL, "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post");
