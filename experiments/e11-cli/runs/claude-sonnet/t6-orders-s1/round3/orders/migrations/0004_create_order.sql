-- alder migration 0004_create_order
-- from 6fa7c425f55e
-- to f8d76d27e4de
-- step: create table Order
-- step: add index on Order.owner

CREATE TABLE "Order__Order" ("id" TEXT NOT NULL PRIMARY KEY, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Order__Order__owner" ON "Order__Order"("owner");
