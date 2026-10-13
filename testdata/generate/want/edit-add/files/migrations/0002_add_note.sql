-- alder migration 0002_add_note
-- from 2e82b15d5f85
-- to d3a322f805e3
-- step: add column Entry.note

ALTER TABLE "Entry__Entry" ADD COLUMN "note" TEXT;
