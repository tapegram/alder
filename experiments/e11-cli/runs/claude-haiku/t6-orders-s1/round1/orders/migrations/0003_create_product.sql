-- alder migration 0003_create_product
-- from b77c7e572078
-- to b387a9e450a2
-- step: create table Product

CREATE TABLE "Product__Product" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "price" INTEGER NOT NULL, "active" INTEGER NOT NULL CHECK ("active" IN (0, 1))) STRICT;
