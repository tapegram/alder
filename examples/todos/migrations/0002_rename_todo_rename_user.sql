-- alder migration 0002_rename_todo_rename_user
-- from 769953ebeb00
-- to 8004ef3b7282
-- step: rename table Todo to Todo
-- step: rename table User to User
-- step: drop index on Todo.owner
-- step: add index on Todo.owner

ALTER TABLE "Todo__Todo" RENAME TO "Table__Todo__Todo";
ALTER TABLE "User__User" RENAME TO "Table__User__User";
DROP INDEX IF EXISTS "Todo__Todo__owner";
CREATE INDEX IF NOT EXISTS "Table__Todo__Todo__owner" ON "Table__Todo__Todo"("owner");
