-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 1c3a8d499e87
-- step: create table Book
-- step: create table Loan
-- step: add index on Loan.book

CREATE TABLE "Table__Book__Book" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "author" TEXT NOT NULL) STRICT;
CREATE TABLE "Table__Loan__Loan" ("id" TEXT NOT NULL PRIMARY KEY, "book" TEXT NOT NULL REFERENCES "Table__Book__Book"("id")) STRICT;
CREATE INDEX IF NOT EXISTS "Table__Loan__Loan__book" ON "Table__Loan__Loan"("book");
-- The shelf starts with five books, ids 1 to 5: five is the fewest lookups the dev panel calls an
-- N+1, and alder-tui's live test opens /books/1.
INSERT INTO "Table__Book__Book" ("id", "title", "author") VALUES ('1', 'Middlemarch', 'George Eliot');
INSERT INTO "Table__Book__Book" ("id", "title", "author") VALUES ('2', 'Bleak House', 'Charles Dickens');
INSERT INTO "Table__Book__Book" ("id", "title", "author") VALUES ('3', 'Persuasion', 'Jane Austen');
INSERT INTO "Table__Book__Book" ("id", "title", "author") VALUES ('4', 'The Leopard', 'Giuseppe Tomasi di Lampedusa');
INSERT INTO "Table__Book__Book" ("id", "title", "author") VALUES ('5', 'Moby-Dick', 'Herman Melville');
