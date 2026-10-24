-- alder migration 0002_create_like
-- from df651374c5bd
-- to 9d3f2cdb661f
-- step: create table Like
-- step: add index on Like.owner
-- step: add index on Like.post

CREATE TABLE "Table__Like__Like" ("id" TEXT NOT NULL PRIMARY KEY, "post" TEXT NOT NULL REFERENCES "Table__Post__Post"("id") ON DELETE CASCADE, "owner" TEXT NOT NULL REFERENCES "Table__User__User"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Table__Like__Like__owner" ON "Table__Like__Like"("owner");
CREATE INDEX IF NOT EXISTS "Table__Like__Like__post" ON "Table__Like__Like"("post");
