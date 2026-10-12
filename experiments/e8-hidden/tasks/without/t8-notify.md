# Task T8: email the author about a comment

Change the blog app shown after this task (its `app/` as it is now) so
that when someone comments on a post, the post's author gets an email:

- Sending email is a capability, `Mailer`, declared in a new module
  `app/Mail.alder` with the `fake` given below. Its Go adapter is not yours
  to write.
- When a comment is stored, one email is sent through `Mailer.send` to the
  email address of the post's author (the `User` the post's `author` field
  names). Its subject is exactly `New comment on ` followed by the post's
  title (`New comment on Hello world`); its body contains the commenter's
  name and the comment's text.
- A comment the form rejects (an empty one, say) is not stored and sends no
  email.

**Names.** Use exactly these names; anything not listed here is your choice.
Every name the app already has (modules, pages, forms, messages, fields)
stays as it is, and the tables stay as they are (the fake's `Sent` table
apart).

- `app/Mail.alder` is module `Mail`, and declares exactly this (it may add
  imports and further declarations):

  ```alder
  type SentId = brand Text

  capability Mailer {
    send : { to: Text.Email, subject: Text, body: Text } -> Task Unit,
  }

  fake Mailer {
    table Sent {
      id: SentId @id,
      to: Text.Email,
      subject: Text,
      body: Text,
    }

    send args = {
      let _ <- Db.insert Sent { to: args.to, subject: args.subject, body: args.body }
      Task.succeed ()
    }
  }
  ```

- `app/App.alder`: the session stays exactly `session { user: Maybe User.UserId }`.
- `app/Posts/Show.alder` (page `Posts.Show`, `/posts/{slug}`): the comment
  form stays `form NewComment = { author: Comment.Name, body: Comment.Body }`,
  posted by `msg AddComment form NewComment`, which still answers
  `Redirect (Posts.Show { slug: … })` (the same post) after storing the
  comment and sending the email.

Output only the files you add or change: not the files you leave as they are,
not the standard library, nothing outside `app/`. Give each in full, preceded by
a line `--- file: app/<path> ---`. No explanation, only files.
