-- alder migration 0002_add_summary
-- from df651374c5bd
-- to 7be41661a585
-- step: rebuild table Post: Post.summary: new column filled with ''

CREATE TABLE "Table__Post__Post__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "status" TEXT NOT NULL CHECK ("status" IN ('Draft', 'Published')), "featured" INTEGER NOT NULL CHECK ("featured" IN (0, 1)), "author" TEXT NOT NULL REFERENCES "Table__User__User"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL, "summary" TEXT NOT NULL) STRICT;
INSERT INTO "Table__Post__Post__alder__new" ("id", "title", "body", "status", "featured", "author", "created", "updated", "summary") SELECT "id", "title", "body", "status", "featured", "author", "created", "updated", '' FROM "Table__Post__Post";
DROP TABLE "Table__Post__Post";
ALTER TABLE "Table__Post__Post__alder__new" RENAME TO "Table__Post__Post";
CREATE INDEX IF NOT EXISTS "Table__Post__Post__author" ON "Table__Post__Post"("author");
