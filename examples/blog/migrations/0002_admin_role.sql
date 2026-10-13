-- alder migration 0002_admin_role
-- from 3fa31c8ee016
-- to b5992516745b
-- step: rebuild table User: User.role: Reader | Author -> Reader | Author | Admin

CREATE TABLE "User__User__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "email" TEXT NOT NULL UNIQUE, "name" TEXT NOT NULL, "passwordHash" TEXT NOT NULL, "role" TEXT NOT NULL CHECK ("role" IN ('Reader', 'Author', 'Admin')), "created" TEXT NOT NULL) STRICT;
INSERT INTO "User__User__alder__new" ("id", "email", "name", "passwordHash", "role", "created") SELECT "id", "email", "name", "passwordHash", "role", "created" FROM "User__User";
DROP TABLE "User__User";
ALTER TABLE "User__User__alder__new" RENAME TO "User__User";
