-- alder migration 0002_rename_comment_rename_post_rename_user
-- from b5992516745b
-- to 35c1cad14669
-- step: rename table Comment.Comment to Table.Comment.Comment
-- step: rename table Post.Post to Table.Post.Post
-- step: rename table User.User to Table.User.User
-- step: drop index on Comment.post
-- step: drop index on Post.author
-- step: add index on Comment.post
-- step: add index on Post.author

ALTER TABLE "Comment__Comment" RENAME TO "Table__Comment__Comment";
ALTER TABLE "Post__Post" RENAME TO "Table__Post__Post";
ALTER TABLE "User__User" RENAME TO "Table__User__User";
DROP INDEX IF EXISTS "Comment__Comment__post";
DROP INDEX IF EXISTS "Post__Post__author";
CREATE INDEX IF NOT EXISTS "Table__Comment__Comment__post" ON "Table__Comment__Comment"("post");
CREATE INDEX IF NOT EXISTS "Table__Post__Post__author" ON "Table__Post__Post"("author");
