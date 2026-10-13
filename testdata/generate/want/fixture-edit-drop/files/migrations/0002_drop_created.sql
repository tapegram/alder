-- alder migration 0002_drop_created
-- from 0e16e2731724
-- to 783f99f421cf
-- step: drop column Tag.created
-- DROPS DATA: column Tag.created

ALTER TABLE "Tag__Tag" DROP COLUMN "created";
