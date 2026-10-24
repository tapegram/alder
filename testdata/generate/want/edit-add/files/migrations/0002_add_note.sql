-- alder migration 0002_add_note
-- from 9b787a621da5
-- to 3271191c30ad
-- step: add column Entry.note

ALTER TABLE "Table__Entry__Entry" ADD COLUMN "note" TEXT;
