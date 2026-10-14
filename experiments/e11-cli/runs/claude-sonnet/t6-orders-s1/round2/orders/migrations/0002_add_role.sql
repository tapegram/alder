-- alder migration 0002_add_role
-- from 0e812c25c044
-- to f0a894d79d48
-- step: rebuild table User: User.role: new column filled with 'Staff'

CREATE TABLE "User__User__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "created" TEXT NOT NULL, "role" TEXT NOT NULL CHECK ("role" IN ('Staff', 'Manager'))) STRICT;
INSERT INTO "User__User__alder__new" ("id", "email", "name", "passwordHash", "created", "role") SELECT "id", "email", "name", "passwordHash", "created", 'Staff' FROM "User__User";
DROP TABLE "User__User";
ALTER TABLE "User__User__alder__new" RENAME TO "User__User";
