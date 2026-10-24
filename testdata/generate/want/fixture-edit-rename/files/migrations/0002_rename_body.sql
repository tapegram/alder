-- alder migration 0002_rename_body
-- from df651374c5bd
-- to 4022c41a7d3b
-- step: rename column Post.body to text

ALTER TABLE "Table__Post__Post" RENAME COLUMN "body" TO "text";
