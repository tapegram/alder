-- alder migration 0003_create_user
-- from 1651618c61bc
-- to 0fe5e5396ea3
-- step: create table User

CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
