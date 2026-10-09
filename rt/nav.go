package rt

// NavTag says what a Nav does.
type NavTag uint8

const (
	NavRerender NavTag = iota // render the current page again (with its fresh load)
	NavRedirect               // redirect to Page
	NavReject                 // re-render the submitting page with Text as its error
)

// Nav is the Prelude `Nav`, the result of an `update`: Rerender, Redirect page or Reject text.
// Only the field its Tag names is meaningful.
type Nav struct {
	Tag  NavTag
	Page Page
	Text string
}

// Rerender is the Prelude `Rerender`.
func Rerender() Nav { return Nav{Tag: NavRerender} }

// Redirect is the Prelude `Redirect p`.
func Redirect(p Page) Nav { return Nav{Tag: NavRedirect, Page: p} }

// Reject is the Prelude `Reject t`.
func Reject(t string) Nav { return Nav{Tag: NavReject, Text: t} }
