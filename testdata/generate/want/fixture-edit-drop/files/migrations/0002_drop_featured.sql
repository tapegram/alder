-- alder migration 0002_drop_featured
-- from 0e16e2731724
-- to 0e9afd5839be
-- step: drop column Post.featured
-- DROPS DATA: column Post.featured

ALTER TABLE "Post__Post" DROP COLUMN "featured";
