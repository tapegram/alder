package rt

// NavTag says what a Nav does.
type NavTag uint8

const (
	NavRerender NavTag = iota // render the current page again (with its fresh load)
	NavRedirect               // redirect to Page
	NavReject                 // re-render the submitting page with Text as its error
	NavNoticed                // Text is a toast shown with the answer Next
)

// Nav is the Prelude `Nav`, the result of an `update`: Rerender, Redirect page or Reject text.
// Only the field its Tag names is meaningful.
type Nav struct {
	Tag  NavTag
	Page Page
	Text string
	Next *Nav // NavNoticed: the answer the notice rides on
}

// Rerender is the Prelude `Rerender`.
func Rerender() Nav { return Nav{Tag: NavRerender} }

// Redirect is the Prelude `Redirect p`.
func Redirect(p Page) Nav { return Nav{Tag: NavRedirect, Page: p} }

// Reject is the Prelude `Reject t`.
func Reject(t string) Nav { return Nav{Tag: NavReject, Text: t} }

// Noticed is the Prelude `Noticed text nav`: nav's answer with a toast showing text.
func Noticed(t string, n Nav) Nav { return Nav{Tag: NavNoticed, Text: t, Next: &n} }

// Unnotice is n without its notices and the text of the outermost one ("" for none).
func (n Nav) Unnotice() (Nav, string) {
	text := ""
	for n.Tag == NavNoticed {
		if text == "" {
			text = n.Text
		}
		n = *n.Next
	}
	return n, text
}
