# Task T14: a support board, filled in after it shows

Write a complete new Alder app (not the todo app) for a support team's
board of tickets, on one server:

- There are no users and no login: anyone who opens the board sees it.
- A ticket has a title, a status (pending, that is still open, or
  closed; a new ticket is pending) and a created time. Tickets are added
  outside the app (straight into the database); the app never adds one.
- `/board` shows at once, without waiting for its slower parts. After it
  shows, it fills in the five newest tickets, and a badge with the number
  of pending tickets, which it asks for again every ten seconds while the
  board is open (chapter 6, "A slow widget and a polling badge").
- `/board` itself lists every pending ticket with its title and a button
  that closes it; the board is then shown again, with the ticket closed.
- An `App` module and a `Layout` module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/App.alder`: module `App`, with exactly `session { }`, no `login`,
  and no routes.
- `app/Table/Ticket.alder`: module `Table.Ticket`, `type TicketId = brand Text`,
  `type Status = Pending | Closed`, and
  `table Ticket { id: TicketId @id, title: Text.NonEmpty, status: Status = Pending, created: Time @created }`.
  (A ticket that is still open is `Pending`: the constructor cannot be
  called `Open`, because `Board` has a route of that name.)
- `app/Board.alder`: route module `Board`, with
  `route Board = GET "/board" { }`, the page;
  `route Recent = GET "/board/recent" { }`, the five newest tickets; and
  `route Open = GET "/board/open" { }`, the number of pending tickets.
  None of the three requires anything.
  - `Board`'s view has a `<section id="recent" load={Recent {}}>` and a
    badge `<span id="open" load={Open {}} every="10s">`, and lists every
    pending ticket, each with a button that sends
    `route Close { id: Ticket.TicketId }`. These buttons are on the page
    as `Board` renders it, not inside `#recent` or `#open`.
  - `Recent`'s view is a `<section id="recent">` holding the five newest
    tickets (all of them when there are fewer; pending or closed alike),
    newest first, one `<li>` per ticket with the ticket's title as its
    whole text, and no other `<li>`: nothing for an empty list. Unlike
    chapter 6's `Dashboard`, whose `Recent` uses `<p>`, these are `<li>`.
  - `Open`'s view is the badge, `<span id="open" load={Open {}} every="10s">`,
    whose whole text is the number of pending tickets in digits (`3`).
    As in chapter 6's `Dashboard`, it writes the same `load` and `every`
    on its root, so the badge keeps polling after its first answer; the
    period is ten seconds, not the chapter's thirty.
  - `Close` closes the ticket (its status becomes `Closed`) and answers
    `Response.refresh`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test:
the badge counts the pending tickets, and counts again after one is added
and after one is closed; recent lists the five newest of six, newest first;
closing a ticket through its button refreshes the board and closes it.
`alder test` must pass.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
