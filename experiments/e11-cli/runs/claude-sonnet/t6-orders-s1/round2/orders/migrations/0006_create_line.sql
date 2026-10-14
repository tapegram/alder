-- alder migration 0006_create_line
-- from bdaf3f1b8e82
-- to 7446258a0f35
-- step: create table Line
-- step: add index on Line.order
-- step: add index on Line.product

CREATE TABLE "Line__Line" ("id" TEXT NOT NULL PRIMARY KEY, "order" TEXT NOT NULL REFERENCES "Order__Order"("id") ON DELETE CASCADE, "product" TEXT NOT NULL REFERENCES "Product__Product"("id"), "quantity" INTEGER NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Line__Line__order" ON "Line__Line"("order");
CREATE INDEX IF NOT EXISTS "Line__Line__product" ON "Line__Line"("product");
