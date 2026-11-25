package markup

import (
	"sort"
	"strings"
)

// HTML is the built-in HTML manifest.
var HTML = buildHTML()

// attrDocs describes every attribute by name; a tag lists the names it takes.
var attrDocs = map[string]string{
	"accesskey": "a keyboard shortcut key for the element", "autocapitalize": "how a virtual keyboard capitalizes typed text",
	"autofocus": "focus this element when the page loads", "class": "the element's CSS classes, separated by spaces",
	"contenteditable": "whether the user can edit the element's content", "dir": "the text direction: ltr, rtl or auto",
	"draggable": "whether the element can be dragged", "enterkeyhint": "the label of the virtual keyboard's enter key",
	"hidden": "hide the element", "id": "the element's unique identifier", "inert": "make the element and its children unable to be interacted with",
	"inputmode": "which virtual keyboard to show", "lang": "the language of the element's content", "nonce": "a one-time token for the content security policy",
	"popover": "make the element a popover shown on demand", "slot": "the named slot of the enclosing custom element this goes in",
	"spellcheck": "whether to check the element's spelling", "style": "inline CSS for the element", "tabindex": "the element's place in the tab order",
	"title": "advisory text, usually shown as a tooltip", "translate": "whether the content is translated with the page",
	"role": "the ARIA role of the element", "aria-label": "a text label for assistive technology", "itemscope": "mark the element as a microdata item",
	"href": "the address the link points to", "target": "where to show the linked page or form response", "rel": "how the linked resource relates to this page",
	"download": "download the linked resource instead of opening it", "src": "the address of the resource", "alt": "text shown when the image cannot be",
	"width": "the width in pixels", "height": "the height in pixels", "loading": "load now (eager) or when near the viewport (lazy)",
	"decoding": "how the browser decodes the image: sync, async or auto", "srcset": "a list of image sources for different screen sizes",
	"sizes": "the display width of the image for each screen size", "type": "the type of the control or resource", "name": "the name used when the form is submitted",
	"value": "the value of the control", "placeholder": "a hint shown while the control is empty", "required": "the control must be filled in before submitting",
	"disabled": "the control cannot be used or submitted", "checked": "the checkbox or radio button starts checked", "readonly": "the user cannot change the value",
	"min": "the smallest allowed value", "max": "the largest allowed value", "step": "the interval between allowed values", "pattern": "a regular expression the value must match",
	"autocomplete": "what the browser may fill in for the control", "multiple": "allow more than one value", "list": "the id of a datalist of suggestions",
	"maxlength": "the most characters allowed", "minlength": "the fewest characters allowed", "form": "the id of the form the control belongs to",
	"formaction": "the address to submit the form to, in place of the form's own", "formnovalidate": "submit without checking the form's constraints",
	"alpha": "let the color input choose an opacity", "action": "the address the form submits to", "method": "the HTTP method of the submission: get or post",
	"enctype": "how the form data is encoded", "novalidate": "submit without checking the form's constraints", "for": "the id of the control this labels or reports on",
	"size": "the number of rows shown, or the width in characters", "selected": "the option starts selected", "label": "the text shown for the option or track",
	"rows": "the number of visible text lines", "cols": "the visible width in characters", "wrap": "how the text wraps when submitted: soft or hard",
	"colspan": "how many columns the cell spans", "rowspan": "how many rows the cell spans", "headers": "the ids of the header cells for this cell", "scope": "which cells the header applies to",
	"media": "the media query the resource applies to", "content": "the value of the metadata", "charset": "the document's character encoding",
	"async": "run the script without blocking, as soon as it loads", "defer": "run the script after the document is parsed", "nomodule": "skip the script in browsers that support modules",
	"allow": "the permissions policy for the frame", "allowfullscreen": "let the frame go fullscreen", "controls": "show the player's controls", "autoplay": "start playing as soon as possible",
	"loop": "start again when finished", "muted": "start with the sound off", "playsinline": "play inside the page, not fullscreen", "poster": "an image shown until playback starts",
	"preload": "how much of the media to load ahead: none, metadata or auto", "open": "show the contents", "reversed": "count the list down", "start": "the number the list starts at",
	"datetime": "the date and time, in machine-readable form", "ismap": "the image is a server-side image map", "default": "use this track unless the user chooses another",
	"kind": "what the track is for: subtitles, captions, descriptions, chapters or metadata", "srclang": "the language of the track", "cite": "the address of the quoted source",
	"span": "how many columns the element covers", "usemap": "the name of the image map to use", "high": "the lower bound of the high range", "low": "the upper bound of the low range",
	"optimum": "the best value of the gauge", "shadowrootmode": "attach a shadow root to the parent, open or closed", "shadowrootclonable": "the shadow root is copied when the host is cloned",
	"shadowrootdelegatesfocus": "focus goes to the shadow root's first focusable element", "shadowrootserializable": "the shadow root can be serialized",
	"crossorigin": "how the request handles cross-origin sharing", "integrity": "a hash the fetched resource must match", "sandbox": "restrict what the framed page may do",
	"referrerpolicy": "how much referrer information to send", "hreflang": "the language of the linked page", "ping": "addresses to notify when the link is followed",
	"accept": "the file types the control accepts", "capture": "which camera to use to capture the input", "dirname": "the name of a field carrying the text direction",
	"formenctype": "how the form data is encoded, in place of the form's own", "formmethod": "the HTTP method, in place of the form's own", "formtarget": "where to show the response, in place of the form's own",
	"accept-charset": "the character encodings the server accepts", "as": "the kind of resource being preloaded",
	"http-equiv": "the HTTP header this metadata stands for", "data": "the address of the resource", "popovertarget": "the id of the popover this button controls",
	"popovertargetaction": "show, hide or toggle the popover", "blocking": "block rendering until the resource is fetched", "fetchpriority": "how important the fetch is: high, low or auto",
	"controlslist": "which controls to hide", "disablepictureinpicture": "stop picture-in-picture", "disableremoteplayback": "stop remote playback", "srcdoc": "the HTML of the framed document",
	"coords": "the shape's coordinates", "shape": "the shape of the area",
	// SVG (#306)
	"viewBox": "the drawing's coordinate system: min-x, min-y, width and height in user units", "preserveAspectRatio": "how the viewBox fits the element's box when their shapes differ, such as xMidYMid meet or none",
	"xmlns": "the XML namespace, http://www.w3.org/2000/svg (an inline <svg> needs none)", "x": "the x coordinate, in user units", "y": "the y coordinate, in user units",
	"x1": "the x coordinate of the line's start", "y1": "the y coordinate of the line's start", "x2": "the x coordinate of the line's end", "y2": "the y coordinate of the line's end",
	"cx": "the x coordinate of the centre", "cy": "the y coordinate of the centre", "r": "the radius", "rx": "the horizontal radius, or the corners' rounding", "ry": "the vertical radius, or the corners' rounding",
	"d": "the path's commands, such as M0 0 L10 10", "points": "the vertices as x,y pairs separated by spaces, such as 0,0 5,5",
	"fill": "the paint inside the shape: a color, currentColor or none", "stroke": "the paint of the outline: a color, currentColor or none", "stroke-width": "the width of the outline",
	"stroke-dasharray": "the lengths of the outline's dashes and gaps, such as 4 2", "stroke-linecap": "the shape of the outline's open ends: butt, round or square",
	"stroke-linejoin": "the shape of the outline's corners: miter, round or bevel", "opacity": "the element's opacity, from 0 to 1", "fill-opacity": "the fill's opacity, from 0 to 1",
	"stroke-opacity": "the outline's opacity, from 0 to 1", "transform": "a transformation, such as translate(10 0) or rotate(45)", "text-anchor": "where the text sits on its x: start, middle or end",
	"dominant-baseline": "the baseline the text sits on its y, such as middle or hanging", "font-size": "the size of the text",
}

// svgPaint are the presentation attributes every drawing SVG element takes.
const svgPaint = "fill stroke stroke-width stroke-dasharray stroke-linecap stroke-linejoin opacity fill-opacity stroke-opacity transform text-anchor dominant-baseline font-size "

// svgSpecs are the SVG elements a view may write (#306), as tagSpecs: an SVG element takes only its
// own attributes, the global ones, `data-*` and `aria-*`.
var svgSpecs = []struct{ name, desc, attrs string }{
	{"circle", "an SVG circle", "cx cy r " + svgPaint},
	{"desc", "an SVG element's long description, for assistive technology", ""},
	{"ellipse", "an SVG ellipse", "cx cy rx ry " + svgPaint},
	{"g", "a group of SVG elements that share attributes", svgPaint},
	{"line", "an SVG straight line from (x1, y1) to (x2, y2)", "x1 y1 x2 y2 " + svgPaint},
	{"path", "an SVG shape drawn by its path commands", "d " + svgPaint},
	{"polygon", "an SVG closed shape through its points", "points " + svgPaint},
	{"polyline", "an SVG line through its points, such as a chart's series", "points " + svgPaint},
	{"rect", "an SVG rectangle", "x y width height rx ry " + svgPaint},
	{"svg", "an SVG drawing in the page", "viewBox preserveAspectRatio xmlns x y width height " + svgPaint},
	{"text", "SVG text drawn at (x, y)", "x y " + svgPaint},
	{"tspan", "a run of SVG text inside a <text>, which may move to its own x and y", "x y " + svgPaint},
}

var boolNames = strings.Fields(`allowfullscreen alpha async autofocus autoplay checked controls default defer disabled formnovalidate hidden inert ismap itemscope loop multiple muted nomodule novalidate open playsinline readonly required reversed selected shadowrootclonable shadowrootdelegatesfocus shadowrootserializable`)

// tagSpecs: name, description, then the tag's own attribute names.
var tagSpecs = []struct{ name, desc, attrs string }{
	{"a", "a link to another page, file or place on this page", "href target rel download hreflang ping referrerpolicy"},
	{"abbr", "an abbreviation or acronym", ""},
	{"address", "contact information for its nearest article or the page", ""},
	{"article", "a self-contained piece of content, such as a post", ""},
	{"aside", "content only loosely related to what is around it, such as a sidebar", ""},
	{"audio", "a sound player", "src controls autoplay loop muted preload crossorigin"},
	{"b", "text drawn to attention without extra importance, shown bold", ""},
	{"bdi", "text whose direction is isolated from its surroundings", ""},
	{"bdo", "text whose direction is overridden", ""},
	{"blockquote", "a long quotation set apart as a block", "cite"},
	{"body", "the visible content of the page", ""},
	{"br", "a line break", ""},
	{"button", "a clickable button", "type name value disabled form formaction formnovalidate formenctype formmethod formtarget popovertarget popovertargetaction"},
	{"canvas", "a surface for drawing with scripts", "width height"},
	{"caption", "the title of a table", ""},
	{"cite", "the title of a creative work being cited", ""},
	{"code", "a fragment of computer code", ""},
	{"col", "a column of a table, inside a colgroup", "span"},
	{"colgroup", "a group of table columns", "span"},
	{"data", "content with a machine-readable value", "value"},
	{"datalist", "a list of suggested values for an input", ""},
	{"dd", "the description of a term in a description list", ""},
	{"del", "text that was removed from the document", "cite datetime"},
	{"details", "a disclosure widget the user opens and closes", "open name"},
	{"dfn", "the term being defined", ""},
	{"dialog", "a dialog box or window", "open"},
	{"div", "a generic block container", ""},
	{"dl", "a description list of terms and their descriptions", ""},
	{"dt", "a term in a description list", ""},
	{"em", "text with stress emphasis, shown italic", ""},
	{"embed", "an external application or interactive content", "src type width height"},
	{"fieldset", "a group of related form controls", "disabled form name"},
	{"figcaption", "the caption of a figure", ""},
	{"figure", "self-contained content with an optional caption", ""},
	{"footer", "the footer of the page or of its nearest section", ""},
	{"form", "a form that collects input and submits it", "action method enctype novalidate target accept-charset autocomplete name"},
	{"h1", "a top-level heading", ""},
	{"h2", "a second-level heading", ""},
	{"h3", "a third-level heading", ""},
	{"h4", "a fourth-level heading", ""},
	{"h5", "a fifth-level heading", ""},
	{"h6", "a sixth-level heading", ""},
	{"head", "the page's metadata: title, links and scripts", ""},
	{"header", "the introduction of the page or of its nearest section", ""},
	{"hgroup", "a heading with its subheading or tagline", ""},
	{"hr", "a thematic break between paragraphs, drawn as a line", ""},
	{"html", "the root of the document", ""},
	{"i", "text in an alternate voice, shown italic", ""},
	{"iframe", "another page embedded in this one", "src title width height allow allowfullscreen loading sandbox srcdoc name referrerpolicy"},
	{"img", "an image", "src alt width height loading decoding srcset sizes ismap usemap crossorigin referrerpolicy fetchpriority"},
	{"input", "a form control the user types into or toggles", "type name value placeholder required disabled checked readonly min max step pattern autocomplete multiple list maxlength minlength form alpha formnovalidate accept capture dirname size formaction formenctype formmethod formtarget popovertarget popovertargetaction"},
	{"ins", "text that was added to the document", "cite datetime"},
	{"kbd", "text the user types on a keyboard", ""},
	{"label", "a caption for a form control", "for"},
	{"legend", "the caption of a fieldset", ""},
	{"li", "an item of a list", "value"},
	{"link", "a link to an external resource, such as a stylesheet", "rel href type media as crossorigin integrity hreflang referrerpolicy sizes blocking fetchpriority"},
	{"main", "the main content of the page", ""},
	{"map", "an image map of clickable areas", "name"},
	{"mark", "text highlighted for reference", ""},
	{"menu", "an unordered list of commands", ""},
	{"meta", "metadata that no other tag covers", "name content charset http-equiv media"},
	{"meter", "a scalar measurement within a known range", "value min max low high optimum"},
	{"nav", "a section of navigation links", ""},
	{"noscript", "content shown when scripts are off", ""},
	{"object", "an external resource, such as a plugin or an image", "data type name width height form usemap"},
	{"ol", "an ordered list", "reversed start type"},
	{"optgroup", "a group of options in a select", "disabled label"},
	{"option", "an option of a select or datalist", "value selected disabled label"},
	{"output", "the result of a calculation or user action", "for form name"},
	{"p", "a paragraph", ""},
	{"picture", "an image with alternative sources for different screens", ""},
	{"pre", "preformatted text, shown exactly as written", ""},
	{"progress", "how far a task has got", "value max"},
	{"q", "a short inline quotation", "cite"},
	{"rp", "fallback parentheses for browsers that do not show ruby", ""},
	{"rt", "the pronunciation of ruby text", ""},
	{"ruby", "an annotation over East Asian text", ""},
	{"s", "text that is no longer accurate, shown struck through", ""},
	{"samp", "sample output from a program", ""},
	{"script", "an embedded or external script", "src type async defer nomodule crossorigin integrity referrerpolicy blocking fetchpriority"},
	{"section", "a standalone section of the page, usually with a heading", ""},
	{"select", "a control that picks from a list of options", "name multiple required disabled size form autocomplete"},
	{"slot", "a placeholder inside a custom element for the content a page gives it", "name"},
	{"small", "side comments and fine print", ""},
	{"source", "a media resource for a picture, video or audio", "src type srcset media sizes width height"},
	{"span", "a generic inline container", ""},
	{"strong", "text of strong importance, shown bold", ""},
	{"style", "CSS for the page", "media blocking"},
	{"sub", "subscript text", ""},
	{"summary", "the visible heading of a details widget", ""},
	{"sup", "superscript text", ""},
	{"table", "tabular data in rows and columns", ""},
	{"tbody", "the body rows of a table", ""},
	{"td", "a data cell of a table", "colspan rowspan headers"},
	{"template", "markup that is not shown until a script or a shadow root uses it", "shadowrootmode shadowrootclonable shadowrootdelegatesfocus shadowrootserializable"},
	{"textarea", "a multi-line text control", "name rows cols placeholder required disabled readonly maxlength minlength wrap form autocomplete dirname"},
	{"tfoot", "the footer rows of a table", ""},
	{"th", "a header cell of a table", "colspan rowspan headers scope"},
	{"thead", "the header rows of a table", ""},
	{"time", "a date or time", "datetime"},
	{"title", "the title of the page, shown in the tab, or inside <svg> the drawing's accessible name", ""},
	{"tr", "a row of a table", ""},
	{"track", "a timed text track for a video or audio", "kind src srclang label default"},
	{"u", "text with an unarticulated annotation, shown underlined", ""},
	{"ul", "an unordered list", ""},
	{"var", "a variable in a mathematical expression or program", ""},
	{"video", "a video player", "src controls autoplay loop muted playsinline poster preload width height crossorigin"},
	{"wbr", "a place where the browser may break a long word", ""},
}

func buildHTML() *Manifest {
	bools := map[string]bool{}
	for _, b := range boolNames {
		bools[b] = true
	}
	attr := func(name string) Attr {
		d, ok := attrDocs[name]
		if !ok {
			panic("markup: no description for attribute " + name)
		}
		return Attr{Name: name, Description: d, Boolean: bools[name]}
	}
	m := &Manifest{Name: "html"}
	for _, n := range strings.Fields(`accesskey autocapitalize autofocus class contenteditable dir draggable enterkeyhint hidden id inert inputmode itemscope lang nonce popover slot spellcheck style tabindex title translate role aria-label`) {
		m.Global = append(m.Global, attr(n))
	}
	add := func(specs []struct{ name, desc, attrs string }, svg bool) {
		for _, s := range specs {
			t := Tag{Name: s.name, Description: s.desc, SVG: svg}
			for _, n := range strings.Fields(s.attrs) {
				t.Attrs = append(t.Attrs, attr(n))
			}
			m.Tags = append(m.Tags, t)
		}
	}
	add(tagSpecs, false)
	add(svgSpecs, true)
	sort.Slice(m.Tags, func(i, j int) bool { return m.Tags[i].Name < m.Tags[j].Name })
	return m
}
