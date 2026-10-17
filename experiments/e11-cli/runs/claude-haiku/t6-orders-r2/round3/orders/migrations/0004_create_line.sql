-- alder migration 0004_create_line
-- from b387a9e450a2
-- to 1775a0e2afa8
-- step: create table Line
-- step: add index on Line.order
-- step: add index on Line.product

CREATE TABLE "Line__Line" ("id" TEXT NOT NULL PRIMARY KEY, "order" TEXT NOT NULL REFERENCES "Order__Order"("id") ON DELETE CASCADE, "product" TEXT NOT NULL REFERENCES "Product__Product"("id"), "quantity" INTEGER NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Line__Line__order" ON "Line__Line"("order");
CREATE INDEX IF NOT EXISTS "Line__Line__product" ON "Line__Line"("product");
