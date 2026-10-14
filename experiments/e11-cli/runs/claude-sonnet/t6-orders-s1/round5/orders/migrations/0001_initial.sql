-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 0e812c25c044
-- step: create table User

CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
