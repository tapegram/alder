-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 737cce53101b
-- step: create table Todo
-- step: create table User
-- step: add index on Todo.owner

CREATE TABLE "Todo__Todo" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "done" INTEGER NOT NULL CHECK ("done" IN (0, 1)), "due" TEXT, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT;
CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Todo__Todo__owner" ON "Todo__Todo"("owner");
