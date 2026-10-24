-- alder migration 0002_rename_activity_rename_membership
-- from d6faca00abdb
-- to ea6193c9b914
-- step: rename table Activity.Activity to Table.Activity.Activity
-- step: rename table Membership.Membership to Table.Membership.Membership
-- step: rename table Org.Org to Table.Org.Org
-- step: rename table Project.Project to Table.Project.Project
-- step: rename table Todo.Todo to Table.Todo.Todo
-- step: rename table User.User to Table.User.User
-- step: drop index on Activity.org
-- step: drop index on Membership.org
-- step: drop index on Membership.user
-- step: drop index on Project.org
-- step: drop index on Todo.org
-- step: drop index on Todo.project
-- step: add index on Activity.org
-- step: add index on Membership.org
-- step: add index on Membership.user
-- step: add index on Project.org
-- step: add index on Todo.org
-- step: add index on Todo.project

ALTER TABLE "Activity__Activity" RENAME TO "Table__Activity__Activity";
ALTER TABLE "Membership__Membership" RENAME TO "Table__Membership__Membership";
ALTER TABLE "Org__Org" RENAME TO "Table__Org__Org";
ALTER TABLE "Project__Project" RENAME TO "Table__Project__Project";
ALTER TABLE "Todo__Todo" RENAME TO "Table__Todo__Todo";
ALTER TABLE "User__User" RENAME TO "Table__User__User";
DROP INDEX IF EXISTS "Activity__Activity__org";
DROP INDEX IF EXISTS "Membership__Membership__org";
DROP INDEX IF EXISTS "Membership__Membership__user";
DROP INDEX IF EXISTS "Project__Project__org";
DROP INDEX IF EXISTS "Todo__Todo__org";
DROP INDEX IF EXISTS "Todo__Todo__project";
CREATE INDEX IF NOT EXISTS "Table__Activity__Activity__org" ON "Table__Activity__Activity"("org");
CREATE INDEX IF NOT EXISTS "Table__Membership__Membership__org" ON "Table__Membership__Membership"("org");
CREATE INDEX IF NOT EXISTS "Table__Membership__Membership__user" ON "Table__Membership__Membership"("user");
CREATE INDEX IF NOT EXISTS "Table__Project__Project__org" ON "Table__Project__Project"("org");
CREATE INDEX IF NOT EXISTS "Table__Todo__Todo__org" ON "Table__Todo__Todo"("org");
CREATE INDEX IF NOT EXISTS "Table__Todo__Todo__project" ON "Table__Todo__Todo"("project");
