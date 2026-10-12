-- alder migration 0002_change_post
-- from f2e7075fb0c3
-- to 3fa31c8ee016
-- step: rebuild table Post: Post.published: new column filled with 0; Post: column order changed (ALTER TABLE only appends)

CREATE TABLE "Post__Post__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "slug" TEXT NOT NULL UNIQUE, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "published" INTEGER NOT NULL CHECK ("published" IN (0, 1)), "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT;
INSERT INTO "Post__Post__alder__new" ("id", "slug", "title", "body", "published", "author", "created") SELECT "id", "slug", "title", "body", 0, "author", "created" FROM "Post__Post";
DROP TABLE "Post__Post";
ALTER TABLE "Post__Post__alder__new" RENAME TO "Post__Post";
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
