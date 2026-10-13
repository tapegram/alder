-- alder migration 0002_rename_body
-- from 0e16e2731724
-- to 70f0e1ce57bd
-- step: rename column Post.body to text

ALTER TABLE "Post__Post" RENAME COLUMN "body" TO "text";
