-- alder migration 0002_create_user
-- from 9b787a621da5
-- to e6770a5cb4f5
-- step: create table User

CREATE TABLE "Table__User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
