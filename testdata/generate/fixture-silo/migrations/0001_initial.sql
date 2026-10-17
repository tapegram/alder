-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 1e805e781e3e
-- step: create table Activity
-- step: create table Issue
-- step: create table Membership
-- step: create table Org
-- step: create table Project
-- step: create table User
-- step: add index on Activity.org
-- step: add index on Issue.assignee
-- step: add index on Issue.org
-- step: add index on Issue.project
-- step: add index on Membership.org
-- step: add index on Membership.user
-- step: add index on Project.org

CREATE TABLE "Activity__Activity" ("id" TEXT NOT NULL PRIMARY KEY, "text" TEXT NOT NULL, "org" TEXT NOT NULL REFERENCES "Org__Org"("id") ON DELETE CASCADE, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Issue__Issue" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "project" TEXT NOT NULL REFERENCES "Project__Project"("id") ON DELETE CASCADE, "org" TEXT NOT NULL REFERENCES "Org__Org"("id"), "status" TEXT NOT NULL CHECK ("status" IN ('Open', 'Closed')), "assignee" TEXT REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Membership__Membership" ("id" TEXT NOT NULL PRIMARY KEY, "user" TEXT NOT NULL REFERENCES "User__User"("id") ON DELETE CASCADE, "org" TEXT NOT NULL REFERENCES "Org__Org"("id") ON DELETE CASCADE, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Org__Org" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
CREATE TABLE "Project__Project" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "archived" INTEGER NOT NULL CHECK ("archived" IN (0, 1)), "org" TEXT NOT NULL REFERENCES "Org__Org"("id"), "created" TEXT NOT NULL) STRICT;
CREATE TABLE "User__User" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "role" TEXT NOT NULL CHECK ("role" IN ('Member', 'Admin')), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Activity__Activity__org" ON "Activity__Activity"("org");
CREATE INDEX IF NOT EXISTS "Issue__Issue__assignee" ON "Issue__Issue"("assignee");
CREATE INDEX IF NOT EXISTS "Issue__Issue__org" ON "Issue__Issue"("org");
CREATE INDEX IF NOT EXISTS "Issue__Issue__project" ON "Issue__Issue"("project");
CREATE INDEX IF NOT EXISTS "Membership__Membership__org" ON "Membership__Membership"("org");
CREATE INDEX IF NOT EXISTS "Membership__Membership__user" ON "Membership__Membership"("user");
CREATE INDEX IF NOT EXISTS "Project__Project__org" ON "Project__Project"("org");
