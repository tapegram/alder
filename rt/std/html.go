package std

import "alder/rt/html"

// HtmlNone is `Html.none`.
func HtmlNone() html.Node { return html.None() }

// HtmlText is `Html.text`: escaped when rendered.
func HtmlText(s string) html.Node { return html.Text(s) }

// HtmlFragment is `Html.fragment`: the nodes side by side, with no wrapper element.
func HtmlFragment(ns []html.Node) html.Node { return html.Nodes(ns...) }

// HtmlUnsafe is `Html.unsafe`: raw markup, never escaped.
func HtmlUnsafe(s string) html.Node { return html.Raw(s) }
