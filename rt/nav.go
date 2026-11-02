package rt

// NavTag says what a Nav does.
type NavTag uint8

const (
	NavRerender NavTag = iota // render the current page again (with its fresh load)
	NavRedirect               // redirect to Page
	NavReject                 // re-render the submitting page with Text as its error
	NavNoticed                // Text is a toast of kind Kind shown with the answer Next
)

// NoticeKind is the variant of a Noticed answer's toast (Notice.with is info).
type NoticeKind string

const (
	NoticeInfo    NoticeKind = "info"
	NoticeSuccess NoticeKind = "success"
	NoticeWarning NoticeKind = "warning"
	NoticeError   NoticeKind = "error"
)

// Nav is the Prelude `Nav`, the result of an `update`: Rerender, Redirect page or Reject text.
// Only the field its Tag names is meaningful.
type Nav struct {
	Tag  NavTag
	Page Page
	Text string
	Kind NoticeKind // NavNoticed: the toast's variant
	Next *Nav       // NavNoticed: the answer the notice rides on
}

// Rerender is the Prelude `Rerender`.
func Rerender() Nav { return Nav{Tag: NavRerender} }

// Redirect is the Prelude `Redirect p`.
func Redirect(p Page) Nav { return Nav{Tag: NavRedirect, Page: p} }

// Reject is the Prelude `Reject t`.
func Reject(t string) Nav { return Nav{Tag: NavReject, Text: t} }

// Noticed is the Prelude `Noticed text nav`: nav's answer with a toast of kind k showing text.
func Noticed(k NoticeKind, t string, n Nav) Nav {
	return Nav{Tag: NavNoticed, Text: t, Kind: k, Next: &n}
}

// Unnotice is n without its notices and the text and kind of the outermost one ("" and info
// for none).
func (n Nav) Unnotice() (Nav, string, NoticeKind) {
	text, kind := "", NoticeInfo
	for n.Tag == NavNoticed {
		if text == "" {
			text, kind = n.Text, n.Kind
		}
		n = *n.Next
	}
	return n, text, kind
}
