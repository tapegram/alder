-- alder migration 0002_add_summary
-- from 0e16e2731724
-- to 4eaa62b8ec7e
-- step: add column Post.summary

ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;
