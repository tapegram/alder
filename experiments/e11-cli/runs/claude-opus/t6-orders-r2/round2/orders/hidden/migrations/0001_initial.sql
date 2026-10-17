-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 786383c21eab
-- step: create table User

CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "role" TEXT NOT NULL CHECK ("role" IN ('Staff', 'Manager')), "created" TEXT NOT NULL) STRICT;
