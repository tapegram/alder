-- alder migration 0005_add_status
-- from f8d76d27e4de
-- to bdaf3f1b8e82
-- step: rebuild table Order: Order.status: new column filled with 'Open'

CREATE TABLE "Order__Order__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL, "status" TEXT NOT NULL CHECK ("status" IN ('Open', 'Paid', 'Cancelled'))) STRICT;
INSERT INTO "Order__Order__alder__new" ("id", "owner", "created", "status") SELECT "id", "owner", "created", 'Open' FROM "Order__Order";
DROP TABLE "Order__Order";
ALTER TABLE "Order__Order__alder__new" RENAME TO "Order__Order";
CREATE INDEX IF NOT EXISTS "Order__Order__owner" ON "Order__Order"("owner");
