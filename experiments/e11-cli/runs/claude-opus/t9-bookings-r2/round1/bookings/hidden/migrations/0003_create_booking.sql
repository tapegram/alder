-- alder migration 0003_create_booking
-- from 86fc1e65dae4
-- to e49c0d15da48
-- step: create table Booking
-- step: add index on Booking.owner
-- step: add index on Booking.room

CREATE TABLE "Booking__Booking" ("id" TEXT NOT NULL PRIMARY KEY, "room" TEXT NOT NULL REFERENCES "Room__Room"("id"), "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "from" TEXT NOT NULL, "to" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Booking__Booking__owner" ON "Booking__Booking"("owner");
CREATE INDEX IF NOT EXISTS "Booking__Booking__room" ON "Booking__Booking"("room");
