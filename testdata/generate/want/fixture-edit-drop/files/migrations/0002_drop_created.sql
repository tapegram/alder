-- alder migration 0002_drop_created
-- from df651374c5bd
-- to f19aa63d4e5f
-- step: drop column Tag.created
-- DROPS DATA: column Tag.created

ALTER TABLE "Table__Tag__Tag" DROP COLUMN "created";
