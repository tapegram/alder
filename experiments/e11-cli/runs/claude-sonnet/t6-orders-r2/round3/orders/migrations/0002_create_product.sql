-- alder migration 0002_create_product
-- from 786383c21eab
-- to e5c2ef7886ee
-- step: create table Product

CREATE TABLE "Product__Product" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "price" INTEGER NOT NULL, "active" INTEGER NOT NULL CHECK ("active" IN (0, 1))) STRICT;
