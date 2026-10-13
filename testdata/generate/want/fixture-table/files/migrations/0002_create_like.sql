-- alder migration 0002_create_like
-- from 0e16e2731724
-- to 4129986c43e2
-- step: create table Like
-- step: add index on Like.owner
-- step: add index on Like.post

CREATE TABLE "Like__Like" ("id" TEXT NOT NULL PRIMARY KEY, "post" TEXT NOT NULL REFERENCES "Post__Post"("id") ON DELETE CASCADE, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Like__Like__owner" ON "Like__Like"("owner");
CREATE INDEX IF NOT EXISTS "Like__Like__post" ON "Like__Like"("post");
