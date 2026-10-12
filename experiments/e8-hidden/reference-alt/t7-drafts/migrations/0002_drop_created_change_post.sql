-- alder migration 0002_drop_created_change_post
-- from f2e7075fb0c3
-- to 4bc820094aba
-- step: drop column User.created
-- DROPS DATA: column User.created
-- step: rebuild table Post: Post.published: new column filled with 0

ALTER TABLE "User__User" DROP COLUMN "created";
CREATE TABLE "Post__Post__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "slug" TEXT NOT NULL UNIQUE, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL, "published" INTEGER NOT NULL CHECK ("published" IN (0, 1))) STRICT;
INSERT INTO "Post__Post__alder__new" ("id", "slug", "title", "body", "author", "created", "published") SELECT "id", "slug", "title", "body", "author", "created", 0 FROM "Post__Post";
DROP TABLE "Post__Post";
ALTER TABLE "Post__Post__alder__new" RENAME TO "Post__Post";
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
