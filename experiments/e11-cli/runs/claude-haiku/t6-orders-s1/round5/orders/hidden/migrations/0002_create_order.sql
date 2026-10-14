-- alder migration 0002_create_order
-- from 786383c21eab
-- to b77c7e572078
-- step: create table Order
-- step: add index on Order.owner

CREATE TABLE "Order__Order" ("id" TEXT NOT NULL PRIMARY KEY, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "status" TEXT NOT NULL CHECK ("status" IN ('Open', 'Paid', 'Cancelled')), "created" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Order__Order__owner" ON "Order__Order"("owner");
