-- alder migration 0002_create_blog_post
-- from df651374c5bd
-- to 3d3fd6c69d2f
-- step: create table BlogPost
-- step: add index on BlogPost.owner
-- step: add index on BlogPost.post
-- step: add index on BlogPost.reviewer

CREATE TABLE "Table__BlogPost__BlogPost" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "owner" TEXT NOT NULL REFERENCES "Table__User__User"("id"), "reviewer" TEXT REFERENCES "Table__User__User"("id"), "post" TEXT REFERENCES "Table__Post__Post"("id"), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Table__BlogPost__BlogPost__owner" ON "Table__BlogPost__BlogPost"("owner");
CREATE INDEX IF NOT EXISTS "Table__BlogPost__BlogPost__post" ON "Table__BlogPost__BlogPost"("post");
CREATE INDEX IF NOT EXISTS "Table__BlogPost__BlogPost__reviewer" ON "Table__BlogPost__BlogPost"("reviewer");
