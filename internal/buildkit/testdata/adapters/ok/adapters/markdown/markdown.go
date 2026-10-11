package markdown

import "app"

// A constructor may return any value implementing the interface; its result is Go's to check.
func Markdown() app.Cap__Markdown__Markdown { return renderer{} }

type renderer struct{}
