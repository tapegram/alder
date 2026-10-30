package check

// HookDoc is what the editor says about a page hook (hover, chapter 11): its required
// signature as Alder text and its role, one paragraph of markdown. ok is false for a name that
// is not a hook. checkPage reads the same table, so the two cannot drift.
func HookDoc(name string) (sig, role string, ok bool) {
	switch name {
	case "load":
		return "Params -> Session -> Query Model",
			"Page hook. Runs on every request before `view`: reads the page parameters and the session " +
				"and builds the `Model`. Required: `Params -> Session -> Query Model`.", true
	case "view":
		return "Model -> Session -> Html Msg",
			"Page hook. Renders the `Model` to HTML; `click` and `submit` in it send `Msg` values to " +
				"`update`. Required: `Model -> Session -> Html Msg`, or `File` for a file page (chapter 6).", true
	case "update":
		return "Msg -> Model -> Session -> Task Nav",
			"Page hook. Handles one `Msg` and answers with a `Task Nav`: `Rerender` runs `load` and " +
				"`view` again, `Redirect` goes to a page (the same page with new parameters is how a page " +
				"keeps state), `Reject` answers with an error. State lives in the page parameters, the " +
				"session or a table — `Model` is rebuilt by `load` on every request. " +
				"Required: `Msg -> Model -> Session -> Task Nav`.", true
	}
	return "", "", false
}

// hookSig is hook name's required signature as Alder text.
func hookSig(name string) string {
	sig, _, _ := HookDoc(name)
	return sig
}

// hookMessage is checkPage's E-message for hook name: "`load` must have type `…`".
func hookMessage(name string) string {
	sig, _, _ := HookDoc(name)
	return "`" + name + "` must have type `" + sig + "`"
}
