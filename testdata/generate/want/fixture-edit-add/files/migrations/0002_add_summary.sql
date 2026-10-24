-- alder migration 0002_add_summary
-- from df651374c5bd
-- to def7851efede
-- step: add column Post.summary

ALTER TABLE "Table__Post__Post" ADD COLUMN "summary" TEXT;
