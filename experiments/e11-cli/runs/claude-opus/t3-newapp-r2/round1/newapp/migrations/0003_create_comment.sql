-- alder migration 0003_create_comment
-- from 2a9504a1b513
-- to 5fc35f50922b
-- step: create table Comment
-- step: add index on Comment.post

CREATE TABLE "Comment__Comment" ("id" TEXT NOT NULL PRIMARY KEY, "post" TEXT NOT NULL REFERENCES "Post__Post"("id") ON DELETE CASCADE, "author" TEXT NOT NULL, "body" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post");
