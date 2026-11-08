package generate

// MsgData is the data of msg.tmpl: the message, its parameters as declared (`id: Post.PostId`),
// the `click` value of its button (`Archive`, `Archive { id: ?archiveId }`) and the button's
// text.
type MsgData struct {
	Name   string
	Fields []string
	Click  string
	Label  string
	Class  string // the button's class, "" for none (a button in a loop has its message's name)
	ID     string // the button's id (--id), "" for none
	When   string // the condition the button is shown on, "" for always
}

// MsgOptions are `alder new msg`'s placement options.
type MsgOptions struct {
	InFor string // --in-for: the binder of the view's loop the button goes in
	When  string // --when: the Bool expression the button is shown on
	ID    string // --id: the button's id attribute
}

// NewMsg generates `alder new msg`: a write route on a route module (newRouteMsg: the route, its
// button at the end of the view and a test that presses it; --in-for, --when and --id place the
// button), or on App (newAppRouteMsg: a route without a button, sent from any page).
func NewMsg(p *Program, page, name string, fields []FieldSpec, o MsgOptions) ([]Edit, error) {
	if page == "App" {
		if o.InFor != "" || o.When != "" || o.ID != "" {
			return nil, Refusef("`alder new msg App` adds a route without a button: --in-for, --when and --id belong to a route module")
		}
		return newAppRouteMsg(p, name, fields)
	}
	if len(p.RoutesOf(page)) == 0 {
		return nil, notRouteModule(p, page)
	}
	return newRouteMsg(p, page, name, fields, o)
}

// validID reports whether s can be an element's id written by a generator: a letter, then
// letters, digits, `-` and `_`.
func validID(s string) bool {
	for i, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '_')
		if !ok {
			return false
		}
	}
	return s != ""
}
