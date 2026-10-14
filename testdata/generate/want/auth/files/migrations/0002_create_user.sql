-- alder migration 0002_create_user
-- from 2e82b15d5f85
-- to 2531c4d23c36
-- step: create table User

CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
