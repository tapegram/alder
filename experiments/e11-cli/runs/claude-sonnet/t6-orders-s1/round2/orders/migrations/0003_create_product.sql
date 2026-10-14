-- alder migration 0003_create_product
-- from f0a894d79d48
-- to 6fa7c425f55e
-- step: create table Product

CREATE TABLE "Product__Product" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "price" INTEGER NOT NULL, "active" INTEGER NOT NULL CHECK ("active" IN (0, 1))) STRICT;
