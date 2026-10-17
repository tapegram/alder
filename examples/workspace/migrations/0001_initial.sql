-- alder migration 0001_initial
-- from e3b0c44298fc
-- to d6faca00abdb
-- step: create table Activity
-- step: create table Membership
-- step: create table Org
-- step: create table Project
-- step: create table Todo
-- step: create table User
-- step: add index on Activity.org
-- step: add index on Membership.org
-- step: add index on Membership.user
-- step: add index on Project.org
-- step: add index on Todo.org
-- step: add index on Todo.project

CREATE TABLE "Activity__Activity" ("id" TEXT NOT NULL PRIMARY KEY, "text" TEXT NOT NULL, "org" TEXT NOT NULL REFERENCES "Org__Org"("id") ON DELETE CASCADE, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Membership__Membership" ("id" TEXT NOT NULL PRIMARY KEY, "user" TEXT NOT NULL REFERENCES "User__User"("id") ON DELETE CASCADE, "org" TEXT NOT NULL REFERENCES "Org__Org"("id") ON DELETE CASCADE, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Org__Org" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Project__Project" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "org" TEXT NOT NULL REFERENCES "Org__Org"("id"), "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Todo__Todo" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "done" INTEGER NOT NULL CHECK ("done" IN (0, 1)), "project" TEXT NOT NULL REFERENCES "Project__Project"("id") ON DELETE CASCADE, "org" TEXT NOT NULL REFERENCES "Org__Org"("id"), "created" TEXT NOT NULL) STRICT;
CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "role" TEXT NOT NULL CHECK ("role" IN ('Member', 'Admin')), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Activity__Activity__org" ON "Activity__Activity"("org");
CREATE INDEX IF NOT EXISTS "Membership__Membership__org" ON "Membership__Membership"("org");
CREATE INDEX IF NOT EXISTS "Membership__Membership__user" ON "Membership__Membership"("user");
CREATE INDEX IF NOT EXISTS "Project__Project__org" ON "Project__Project"("org");
CREATE INDEX IF NOT EXISTS "Todo__Todo__org" ON "Todo__Todo"("org");
CREATE INDEX IF NOT EXISTS "Todo__Todo__project" ON "Todo__Todo"("project");
