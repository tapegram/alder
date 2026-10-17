-- alder migration 0002_create_room
-- from 0e812c25c044
-- to 86fc1e65dae4
-- step: create table Room

CREATE TABLE "Room__Room" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL) STRICT;
