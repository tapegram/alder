package main

import "alder/rt/datastar"

// The theme configurator's page (#38 phase 2, spec "The page"): its HTML, its own stylesheet,
// its script and the gallery, as Go strings, composed as rt/web/devpanel_ui.go composes the dev
// panel's. The page links the vendored al.css and al.js and Datastar's script through the
// proxy, so the gallery shows the components the app serves; it subscribes to /_dev/reload as
// of the generation the handler writes in for themeGenMark, so a rebuild reloads it.
//
// Two regions: the panel (themeCSS, neutral colours of its own, no theme token, so a working
// theme can never hide its buttons) and the preview (the gallery, or the app in an iframe). The
// working theme is applied as inline custom properties on <html> (and on the iframe's <html>),
// with `.dark` toggled by the Light/Dark switch; the script keeps it in sessionStorage with the
// committed theme it started from, every access in try/catch.

// themeGenMark is replaced by the build generation (url-escaped) when the page is served.
const themeGenMark = "{{alder-gen}}"

// themePage is GET /_dev/theme.
const themePage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>theme · alder run</title>
<link rel="stylesheet" href="/static/al/al.css">
<style>` + themeCSS + `</style>
<script type="module" src="/static/al/al.js"></script>
<script type="module" src="` + datastar.ScriptPath + `"></script>
</head><body>
<div id="_alder_errors" data-ignore-morph></div><div data-init="@get('/_dev/reload?b=` + themeGenMark + `', {openWhenHidden: true})"></div>
<aside id="tp" aria-label="Theme" data-ignore>
<header>
<h1>theme <span>·</span> alder-ui <span id="tp-version">…</span> <span>·</span> <span id="tp-file">app/App.alder</span></h1>
<div class="tp-bar">
<button type="button" id="scheme" data-scheme="light" aria-label="Scheme: light (switch to dark)"><span data-s="light">Light</span><span data-s="dark">Dark</span></button>
<span id="tp-status" class="tp-status" role="status"></span>
</div>
<div class="tp-bar">
<button type="button" id="reset" title="Back to the theme in App.alder">Reset</button>
<button type="button" id="import" aria-expanded="false" aria-controls="tp-import">Import…</button>
<button type="button" id="write" class="tp-primary" disabled>Write to App.alder</button>
</div>
</header>
<div id="tp-import" hidden>
<label for="import-css">Paste a shadcn or tweakcn theme (CSS)</label>
<textarea id="import-css" spellcheck="false" placeholder=":root { --primary: oklch(0.55 0.22 285); … }&#10;.dark { … }"></textarea>
<div class="tp-bar"><button type="button" id="import-go" class="tp-primary">Import</button><button type="button" id="import-cancel">Close</button></div>
<div id="tp-import-out"></div>
</div>
<div id="tp-msgs"></div>
<section class="tp-group" aria-labelledby="tp-root-h"><h2 id="tp-root-h">Theme-wide</h2><div id="tp-root"></div></section>
<section class="tp-group" aria-labelledby="tp-colours-h"><h2 id="tp-colours-h">Colours of the light scheme</h2><div id="tp-colours"></div></section>
<datalist id="tp-fonts">
<option value="ui-sans-serif, system-ui, sans-serif"></option>
<option value="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"></option>
<option value="Inter, ui-sans-serif, system-ui, sans-serif"></option>
<option value="Geist, ui-sans-serif, system-ui, sans-serif"></option>
<option value="Geist Mono, ui-monospace, monospace"></option>
<option value="ui-serif, Georgia, Cambria, Times New Roman, serif"></option>
</datalist>
</aside>
<div id="tp-preview" data-ignore>
<nav id="tp-tabs" role="tablist" aria-label="Preview">
<button type="button" id="tab-gallery" role="tab" aria-selected="true" aria-controls="gallery">Gallery</button>
<button type="button" id="tab-app" role="tab" aria-selected="false" aria-controls="app-pane">App</button>
</nav>
<div id="gallery" role="tabpanel" aria-labelledby="tab-gallery">` + themeGallery + `</div>
<div id="app-pane" role="tabpanel" aria-labelledby="tab-app" hidden><iframe id="app" title="The app"></iframe></div>
</div>
<script>` + themeJS + `</script>
</body></html>
`

// themeCSS is the page's own stylesheet: the panel and the preview's tabs in neutral colours
// (--tp-*) that follow prefers-color-scheme, the two regions' layout, and the gallery's layout.
// It names no theme token (TestPanelCSS_NoThemeTokens); the gallery's token-coloured parts are
// styled inline in its markup.
const themeCSS = `:root{--tp-bg:#fafafa;--tp-fg:#18181b;--tp-dim:#6b6b73;--tp-line:#dcdce0;--tp-field:#fff;--tp-hover:#ececef;--tp-acc:#2563eb;--tp-acc-fg:#fff;--tp-err:#b91c1c;--tp-hatch:#c4c4cc}
@media (prefers-color-scheme: dark){:root{--tp-bg:#18181b;--tp-fg:#f4f4f5;--tp-dim:#a1a1aa;--tp-line:#3f3f46;--tp-field:#27272a;--tp-hover:#3a3a40;--tp-acc:#3b82f6;--tp-acc-fg:#fff;--tp-err:#f87171;--tp-hatch:#52525b}}
html,body{margin:0}
body{display:grid;grid-template-columns:22rem minmax(0,1fr);min-block-size:100vh}
#tp,#tp-tabs{color-scheme:light;background:var(--tp-bg);color:var(--tp-fg);font:13px/1.4 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;letter-spacing:normal;text-align:start}
@media (prefers-color-scheme: dark){#tp,#tp-tabs{color-scheme:dark}}
#tp{position:sticky;inset-block-start:0;block-size:100vh;overflow:auto;box-sizing:border-box;padding-block-end:2rem;border-inline-end:1px solid var(--tp-line)}
#tp *,#tp-tabs *{box-sizing:border-box}
#tp header{position:sticky;inset-block-start:0;z-index:2;padding:.75rem 1rem;background:var(--tp-bg);border-block-end:1px solid var(--tp-line)}
#tp h1{margin:0;font-size:13px;font-weight:600;line-height:1.4;color:var(--tp-fg)}
#tp h1 span{font-weight:400;color:var(--tp-dim)}
#tp h2{margin:1.1rem 1rem .4rem;font-size:11px;font-weight:600;letter-spacing:.06em;text-transform:uppercase;color:var(--tp-dim)}
#tp button,#tp-tabs button{margin:0;padding:.3rem .65rem;font:inherit;line-height:1.3;color:var(--tp-fg);background:var(--tp-field);border:1px solid var(--tp-line);border-radius:6px;box-shadow:none;cursor:pointer}
#tp button:hover:not(:disabled),#tp-tabs button:hover{background:var(--tp-hover)}
#tp button:disabled{opacity:.45;cursor:default}
#tp button.tp-primary{color:var(--tp-acc-fg);background:var(--tp-acc);border-color:var(--tp-acc)}
#tp button.tp-primary:hover:not(:disabled){background:var(--tp-acc);filter:brightness(1.1)}
#tp :focus-visible,#tp-tabs :focus-visible{outline:2px solid var(--tp-acc);outline-offset:1px}
#tp input,#tp select,#tp textarea{margin:0;padding:.25rem .4rem;min-inline-size:0;font:inherit;color:var(--tp-fg);background:var(--tp-field);border:1px solid var(--tp-line);border-radius:5px;box-shadow:none}
#tp input::placeholder,#tp textarea::placeholder{color:var(--tp-dim);opacity:1}
#tp label{color:inherit;font:inherit}
.tp-bar{display:flex;flex-wrap:wrap;align-items:center;gap:.4rem;margin-block-start:.6rem}
#scheme{display:inline-flex;gap:2px;padding:2px!important}
#scheme span{padding:.15rem .6rem;border-radius:4px}
#scheme[data-scheme="light"] span[data-s="light"],#scheme[data-scheme="dark"] span[data-s="dark"]{color:var(--tp-acc-fg);background:var(--tp-acc)}
.tp-status{color:var(--tp-dim);font-size:12px}
.tp-status[data-busy]::before{content:"";display:inline-block;inline-size:.55rem;block-size:.55rem;margin-inline-end:.35rem;border-radius:50%;background:var(--tp-acc);animation:tp-pulse 1s ease-in-out infinite alternate}
@keyframes tp-pulse{from{opacity:.25}to{opacity:1}}
#tp-import{padding:.75rem 1rem;border-block-end:1px solid var(--tp-line)}
#tp-import label{display:block;margin-block-end:.35rem;color:var(--tp-dim)}
#tp-import textarea{display:block;inline-size:100%;min-block-size:9rem;resize:vertical;font:12px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace}
#tp-import-out p{margin:.6rem 0 .3rem}
#tp-import-out .tp-unplaced{justify-content:space-between;margin-block:.6rem .3rem}
#tp pre{margin:0;padding:.5rem;max-block-size:16rem;overflow:auto;white-space:pre-wrap;font:12px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--tp-fg);background:var(--tp-field);border:1px solid var(--tp-line);border-radius:5px}
#tp-msgs:not(:empty){margin:.75rem 1rem 0;padding:.5rem .6rem;color:var(--tp-err);border:1px solid var(--tp-err);border-radius:6px}
#tp-msgs p{margin:0}
#tp-msgs ul{margin:.35rem 0 0;padding-inline-start:1.1rem}
#tp-msgs code{font:11px ui-monospace,SFMono-Regular,Menlo,monospace}
.tp-row{padding:.3rem 1rem}
.tp-row:hover{background:var(--tp-hover)}
.tp-head{display:flex;align-items:center;gap:.4rem;min-block-size:1.5rem}
.tp-name{flex:1;min-inline-size:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px ui-monospace,SFMono-Regular,Menlo,monospace}
.tp-row.changed .tp-name::after{content:" ●";color:var(--tp-acc);font-size:9px;vertical-align:middle}
#tp .clear{padding:0 .4rem;line-height:1.3;color:var(--tp-dim);background:transparent;border-color:transparent}
#tp .clear:hover{color:var(--tp-fg);border-color:var(--tp-line)}
.tp-unset .clear{visibility:hidden}
.tp-sw{flex:none;inline-size:1.15rem;block-size:1.15rem;border:1px solid var(--tp-line);border-radius:4px}
.tp-unset .tp-sw{opacity:.4}
.tp-sw.tp-bad{background:repeating-linear-gradient(45deg,var(--tp-hatch) 0 2px,transparent 2px 6px)!important}
.tp-ctl{display:flex;gap:.35rem;margin-block-start:.2rem}
.tp-ctl input[type=text]{flex:1}
.tp-ctl input[type=number]{flex:1}
#tp .tp-pick{flex:none;inline-size:2rem;block-size:auto;padding:1px 2px;cursor:pointer}
.tp-pick.tp-nohex::-webkit-color-swatch{background:conic-gradient(#f43f5e,#eab308,#22c55e,#06b6d4,#6366f1,#d946ef,#f43f5e)!important}
.tp-pick.tp-nohex::-moz-color-swatch{background:conic-gradient(#f43f5e,#eab308,#22c55e,#06b6d4,#6366f1,#d946ef,#f43f5e)!important}
.tp-ok{display:grid;grid-template-columns:.9rem minmax(0,1fr) 2.6rem;align-items:center;gap:0 .4rem;margin-block-start:.25rem;font-size:11px;color:var(--tp-dim)}
.tp-ok[hidden]{display:none}
#tp .tp-ok input{padding:0;border:0;background:none;accent-color:var(--tp-acc)}
.tp-ok output{font:11px ui-monospace,SFMono-Regular,Menlo,monospace;text-align:end}
.tp-diag{margin-block-start:.2rem;font-size:12px;color:var(--tp-err)}
.tp-diag:empty{display:none}
#tp-preview{display:flex;flex-direction:column;min-inline-size:0}
#tp-tabs{position:sticky;inset-block-start:0;z-index:5;display:flex;gap:.3rem;padding:.5rem 1rem;border-block-end:1px solid var(--tp-line)}
#tp-tabs button[aria-selected="true"]{color:var(--tp-acc-fg);background:var(--tp-acc);border-color:var(--tp-acc)}
#gallery[hidden],#app-pane[hidden]{display:none}
#app-pane{display:flex;flex:1}
#app{flex:1;inline-size:100%;min-block-size:calc(100vh - 3rem);border:0}
@media (max-width: 60rem){body{grid-template-columns:minmax(0,1fr)}#tp{position:static;block-size:auto;max-block-size:none;border-inline-end:0;border-block-end:1px solid var(--tp-line)}#tp header{position:static}}
.g-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(19rem,1fr));gap:1rem;align-items:start;margin-block:1.5rem}
.g-grid>al-card{margin:0}
.g-tag{font-size:.95rem}
.g-float{display:flex;flex-wrap:wrap;align-items:flex-start;gap:1rem;padding-block:.75rem .25rem}
.g-float>:is(al-popover,al-menu,al-tooltip){position:relative;inset:auto;display:block;margin:0;translate:none;opacity:1}
.g-float>al-popover{inline-size:15rem}
.g-swatches{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:.5rem}
.g-swatches span{display:block;block-size:3rem}
.g-swatches small{display:block;margin-block-start:.2rem;text-align:center}
.g-sidebar{display:grid;gap:.15rem;max-inline-size:16rem;padding:.6rem}
.g-sidebar>b{padding:.25rem .6rem .5rem}
.g-sidebar>a{display:block;padding:.35rem .6rem;text-decoration:none}
`

// themeGallery is the preview's Gallery tab: one card per alder-ui element of the vendored
// manifest but al-devpanel and al-toasts (TestGallery_MatchesManifest), in its common variants,
// written as the renderer would render it (book/C-design-system.md); then the typography, the
// chart colours and a sidebar. al-popover, al-menu and al-tooltip are shown open in place
// (.g-float), not in the top layer.
const themeGallery = `<al-page width="wide"><main>
<h1>Gallery</h1>
<p>Every alder-ui element in the working theme. Change a token on the left: the gallery follows at once, and nothing is written until you press <b>Write to App.alder</b>.</p>
<div class="g-grid">

<al-card><h3 slot="header" class="g-tag"><code>al-button</code></h3><div><al-stack gap="sm">
<al-stack direction="row" gap="sm">
<al-button id="g-primary"><button type="button">Primary</button></al-button>
<al-button variant="secondary"><button type="button">Secondary</button></al-button>
<al-button variant="outline"><button type="button">Outline</button></al-button>
<al-button variant="ghost"><button type="button">Ghost</button></al-button>
<al-button variant="destructive"><button type="button">Destructive</button></al-button>
<al-button variant="link"><button type="button">Link</button></al-button>
</al-stack>
<al-stack direction="row" gap="sm">
<al-button size="xs"><button type="button">Extra small</button></al-button>
<al-button size="sm"><button type="button">Small</button></al-button>
<al-button size="md"><button type="button">Medium</button></al-button>
<al-button size="lg"><button type="button">Large</button></al-button>
</al-stack>
<al-stack direction="row" gap="sm">
<al-button size="icon-sm" variant="outline"><button type="button" aria-label="Add (small)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M12 5v14"/></svg></button></al-button>
<al-button size="icon" variant="outline"><button type="button" aria-label="Add"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M12 5v14"/></svg></button></al-button>
<al-button size="icon-lg" variant="outline"><button type="button" aria-label="Add (large)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M12 5v14"/></svg></button></al-button>
<al-button disabled><button type="button" disabled>Disabled</button></al-button>
</al-stack>
</al-stack></div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-input</code></h3><div><al-stack gap="sm">
<al-input label="Name" help="As it appears on your posts"><label>Name<input type="text" value="Ada Lovelace" aria-describedby="g-help-name"></label><small class="help" id="g-help-name">As it appears on your posts</small><ul class="errors"></ul></al-input>
<al-input label="Email" invalid><label>Email<input type="email" value="ada@" aria-invalid="true" aria-describedby="g-errors-email"></label><ul class="errors" id="g-errors-email"><li>must be an email address</li></ul></al-input>
<al-input label="Handle" placeholder="Not editable" disabled><label>Handle<input type="text" placeholder="Not editable" disabled></label><ul class="errors"></ul></al-input>
</al-stack></div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-textarea</code>, <code>al-select</code>, <code>al-checkbox</code></h3><div><al-stack gap="sm">
<al-textarea label="Bio"><label>Bio<textarea>Mathematician; wrote the first program.</textarea></label><ul class="errors"></ul></al-textarea>
<al-select label="Plan"><label>Plan<select><option>Free</option><option selected>Pro</option><option>Team</option></select></label><ul class="errors"></ul></al-select>
<al-checkbox label="Send me the newsletter"><label>Send me the newsletter<input type="checkbox" checked></label><ul class="errors"></ul></al-checkbox>
<al-checkbox label="Make my profile public"><label>Make my profile public<input type="checkbox"></label><ul class="errors"></ul></al-checkbox>
</al-stack></div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-table</code></h3><div>
<al-table striped><table><thead><tr><th>Title</th><th>Status</th><th>Comments</th></tr></thead><tbody>
<tr><td>Hello, world</td><td><al-badge>Published</al-badge></td><td>3</td></tr>
<tr><td>Drafts are fine</td><td><al-badge variant="secondary">Draft</al-badge></td><td>0</td></tr>
<tr><td>Old news</td><td><al-badge variant="outline">Archived</al-badge></td><td>12</td></tr>
</tbody></table></al-table>
</div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-card</code></h3><p slot="description">A bordered surface; its children are placed by slot.</p><div>
<al-card><h4 slot="header">Hello, world</h4><p slot="description">Posted on 12 October 2026</p><p>The first post. The body is every child without a slot.</p><p slot="footer">3 comments</p><div slot="actions"><al-button variant="outline" size="sm"><button type="button">Back</button></al-button><al-button size="sm"><button type="button">Edit</button></al-button></div></al-card>
</div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-badge</code></h3><div>
<al-stack direction="row" gap="sm"><al-badge>Published</al-badge><al-badge variant="secondary">Draft</al-badge><al-badge variant="outline">Archived</al-badge><al-badge variant="destructive">Failed</al-badge></al-stack>
</div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-alert</code></h3><div><al-stack gap="sm">
<al-alert heading="Heads up" role="status">Comments are moderated before they appear.</al-alert>
<al-alert variant="success" heading="Saved" role="status">Your post is live.</al-alert>
<al-alert variant="warning" heading="Unsaved changes" role="alert">Leaving this page discards your draft.</al-alert>
<al-alert variant="destructive" heading="Could not save" role="alert">The title must not be empty.</al-alert>
</al-stack></div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-toast</code></h3><div><al-stack gap="sm">
<al-toast variant="info" timeout="0">Signed. Thanks for visiting!</al-toast>
<al-toast variant="success" timeout="0">Saved</al-toast>
<al-toast variant="warning" timeout="0">Your session ends in 5 minutes</al-toast>
<al-toast variant="destructive" timeout="0">Could not reach the server</al-toast>
</al-stack></div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-tabs</code>, <code>al-tab</code>, <code>al-panel</code></h3><div><al-stack gap="md">
<al-tabs><al-tab name="g-view" selected><label><input type="radio" name="g-view" checked> Details</label><al-panel>The post's fields: its title, its body and when it was posted.</al-panel></al-tab><al-tab name="g-view"><label><input type="radio" name="g-view"> Comments</label><al-panel>Its three comments, newest first.</al-panel></al-tab></al-tabs>
<al-tabs variant="line"><al-tab name="g-line" selected><label><input type="radio" name="g-line" checked> Overview</label><al-panel>The <code>line</code> variant underlines the active tab.</al-panel></al-tab><al-tab name="g-line"><label><input type="radio" name="g-line"> Activity</label><al-panel>Nothing new this week.</al-panel></al-tab></al-tabs>
</al-stack></div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-dialog</code></h3><div>
<al-button variant="outline"><button type="button" commandfor="g-dialog" command="show-modal">Open the dialog</button></al-button>
<al-dialog heading="Delete this comment?" dismissible><dialog id="g-dialog" aria-labelledby="g-dialog-heading" closedby="any"><form method="dialog"><button type="submit" aria-label="Close Delete this comment?">×</button></form><h2 slot="header" id="g-dialog-heading">Delete this comment?</h2><p>Nothing else refers to it. This cannot be undone.</p><div slot="actions"><al-button variant="outline"><button type="button" commandfor="g-dialog" command="close">Cancel</button></al-button><al-button variant="destructive"><button type="button" commandfor="g-dialog" command="close">Delete</button></al-button></div></dialog></al-dialog>
</div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-popover</code>, <code>al-menu</code>, <code>al-tooltip</code></h3><div class="g-float">
<al-popover placement="bottom"><h4>Who can see this?</h4><p>Only people with the link.</p></al-popover>
<al-menu><a href="#g-new">New post</a><a href="#g-drafts">Drafts</a><hr><button type="button" data-variant="destructive">Sign out</button></al-menu>
<al-tooltip>Saves a draft</al-tooltip>
</div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-pagination</code></h3><div><al-stack gap="sm">
<al-pagination page="2" pages="5" role="navigation" aria-label="Pages"><a slot="prev" href="#g-page-1">Previous</a><a slot="next" href="#g-page-3">Next</a></al-pagination>
<al-pagination page="1" pages="5" role="navigation" aria-label="Pages, first"><span slot="prev" aria-disabled="true">Previous</span><a slot="next" href="#g-page-2">Next</a></al-pagination>
</al-stack></div></al-card>

<al-card><h3 slot="header" class="g-tag"><code>al-stack</code>, <code>al-page</code></h3><div><al-stack gap="sm">
<al-stack direction="row" gap="lg"><al-badge variant="outline">One</al-badge><al-badge variant="outline">Two</al-badge><al-badge variant="outline">Three</al-badge></al-stack>
<al-stack gap="sm"><al-badge variant="secondary">A column</al-badge><al-badge variant="secondary">with a gap</al-badge></al-stack>
<p>This gallery is itself inside an <code>&lt;al-page width="wide"&gt;</code>: its width is <code>widthWide</code>.</p>
</al-stack></div></al-card>

</div>

<h2>Typography</h2>
<h1>Heading one</h1>
<h2>Heading two</h2>
<h3>Heading three</h3>
<p>A paragraph of body text set in <code>fontSans</code>, with <a href="#g-link">a link</a> and some inline <code>code</code>. The colours are <code>foreground</code> on <code>background</code>; muted text such as this <small style="color:var(--muted-foreground)">uses mutedForeground</small>.</p>
<pre>view : Model -&gt; Html Msg
view model =
  &lt;al-button variant="outline"&gt;Save&lt;/al-button&gt;</pre>

<h2>Charts</h2>
<div class="g-swatches">
<div><span style="background:var(--chart-1);border-radius:var(--radius-md)"></span><small>chart1</small></div>
<div><span style="background:var(--chart-2);border-radius:var(--radius-md)"></span><small>chart2</small></div>
<div><span style="background:var(--chart-3);border-radius:var(--radius-md)"></span><small>chart3</small></div>
<div><span style="background:var(--chart-4);border-radius:var(--radius-md)"></span><small>chart4</small></div>
<div><span style="background:var(--chart-5);border-radius:var(--radius-md)"></span><small>chart5</small></div>
</div>

<h2>Sidebar</h2>
<nav class="g-sidebar" aria-label="Sample sidebar" style="background:var(--sidebar);color:var(--sidebar-foreground);border:1px solid var(--sidebar-border);border-radius:var(--radius-lg)">
<b>Acme</b>
<a href="#g-dashboard" style="background:var(--sidebar-primary);color:var(--sidebar-primary-foreground);border-radius:var(--radius-md)">Dashboard</a>
<a href="#g-posts" style="background:var(--sidebar-accent);color:var(--sidebar-accent-foreground);border-radius:var(--radius-md)">Posts (hovered)</a>
<a href="#g-settings" style="color:inherit;border-radius:var(--radius-md);outline:2px solid var(--sidebar-ring);outline-offset:-2px">Settings (focused)</a>
</nav>
</main></al-page>`

// themeJS is the page's behaviour: it reads /_dev/theme/state, builds a control per token from
// the server's table, keeps the working theme (sessionStorage, keyed "alder-theme:" + file,
// with the committed theme it started from), applies it as inline custom properties, writes it,
// imports a pasted export, and polls /state once a second while a build is in progress.
const themeJS = `(function(){"use strict";
var KEY="alder-theme:",BUCKETS=["root","light","dark"],UNITS=["rem","em","px","%","vw","vh","ch","lh"];
var st=null,committed=null,working=null,broken=false,scheme="light",busy=false,wrote=false,polling=false,appShown=false,rows={};
var html=document.documentElement;
function $(id){return document.getElementById(id)}
function el(tag,attrs,text){var e=document.createElement(tag);if(attrs)for(var k in attrs)e.setAttribute(k,attrs[k]);if(text!=null)e.textContent=text;return e}
function store(f){try{return f(window.sessionStorage)}catch(e){return null}}
try{if(matchMedia("(prefers-color-scheme: dark)").matches)scheme="dark"}catch(e){}

// themes: {root,light,dark}, each field to its text; compared in a canonical form
function norm(t){var o={root:{},light:{},dark:{}};if(t)BUCKETS.forEach(function(b){var m=t[b]||{};Object.keys(m).forEach(function(f){var v=m[f]==null?"":String(m[f]).trim();if(v)o[b][f]=v})});return o}
function canon(t){t=norm(t);var o={};BUCKETS.forEach(function(b){var m={};Object.keys(t[b]).sort().forEach(function(f){m[f]=t[b][f]});o[b]=m});return JSON.stringify(o)}
function same(a,b){return canon(a)===canon(b)}
function copy(t){return JSON.parse(canon(t))}
function key(){return KEY+(st?st.file:"")}
function save(){var v=JSON.stringify({committed:committed,working:working});store(function(s){s.setItem(key(),v)})}
function restore(){var raw=store(function(s){return s.getItem(key())});if(!raw)return null;
try{var o=JSON.parse(raw);if(o&&same(o.committed,committed))return norm(o.working)}catch(e){}
store(function(s){s.removeItem(key())});return null}

// a token's bucket, value, committed value and default for the scheme the switch shows
function bucket(t,s){return t.scope==="root"?"root":(s||scheme)}
function value(t){return working[bucket(t)][t.field]||""}
function defOf(t,s){var d=st.defaults[t.field]||{};return (t.scope==="root"||(s||scheme)==="light"?d.Light:d.Dark)||""}
function families(v){var out=[],cur="",q="";for(var i=0;i<v.length;i++){var c=v[i];if(q){if(c===q)q="";cur+=c}else if(c==="'"||c==='"'){q=c;cur+=c}else if(c===","){out.push(cur);cur=""}else cur+=c}out.push(cur);
return out.map(function(f){f=f.trim();if(f.length>1&&(f[0]==="'"||f[0]==='"')&&f[f.length-1]===f[0])f=f.slice(1,-1).trim();return f}).filter(function(f){return f!==""})}
function cssValue(t,v){if(t.kind!=="font stack")return v;return families(v).map(function(f){return /^-?[A-Za-z][\w-]*$/.test(f)?f:'"'+f.replace(/["\\]/g,"")+'"'}).join(", ")}

// the preview: inline custom properties on <html> (and the iframe's), .dark for the dark scheme
function applyTo(root){st.tokens.forEach(function(t){var b=bucket(t),v=working[b][t.field],c=committed[b][t.field];
if(v)root.style.setProperty(t.css,cssValue(t,v));else if(c)root.style.setProperty(t.css,cssValue(t,defOf(t)));else root.style.removeProperty(t.css)});
root.classList.toggle("dark",scheme==="dark")}
function apply(){if(!st)return;applyTo(html);if(appShown){try{var d=$("app").contentDocument;if(d&&d.documentElement)applyTo(d.documentElement)}catch(e){}}}

// the controls
function set(t,v){var b=bucket(t);v=(v||"").trim();if(v)working[b][t.field]=v;else delete working[b][t.field];
save();apply();Object.keys(rows).forEach(function(f){sync(rows[f])});buttons()}
function row(t){var r={t:t},e=el("div",{"class":"tp-row tp-"+t.kind.replace(" ","-"),"data-field":t.field,title:t.css}),id="tp-in-"+t.field;
var head=el("div",{"class":"tp-head"}),ctl=el("div",{"class":"tp-ctl"});r.el=e;
if(t.kind==="colour"){r.sw=el("span",{"class":"tp-sw","aria-hidden":"true"});head.appendChild(r.sw)}
head.appendChild(el("label",{"class":"tp-name","for":id},t.field));
r.clear=el("button",{type:"button","class":"clear","aria-label":"Clear "+t.field,title:"Clear: alder-ui's value"},"×");head.appendChild(r.clear);
r.clear.addEventListener("click",function(){set(t,"")});
e.appendChild(head);e.appendChild(ctl);
if(t.kind==="length"){
r.num=el("input",{type:"number",step:"any",id:id,"class":"tp-num"});r.unit=el("select",{"class":"tp-unit","aria-label":t.field+" unit"});
UNITS.forEach(function(u){r.unit.appendChild(el("option",{value:u},u))});
r.input=el("input",{type:"hidden","class":"value"});ctl.appendChild(r.num);ctl.appendChild(r.unit);ctl.appendChild(r.input);
var joined=function(){var n=r.num.value.trim();if(n==="")return "";return parseFloat(n)===0?"0":n+r.unit.value};
r.num.addEventListener("input",function(){set(t,joined())});
r.unit.addEventListener("change",function(){if(r.num.value.trim()!=="")set(t,joined())});
r.input.addEventListener("input",function(){set(t,r.input.value)});
}else{
r.input=el("input",{type:"text",id:id,"class":"value",spellcheck:"false",autocomplete:"off"});ctl.appendChild(r.input);
if(t.kind==="font stack")r.input.setAttribute("list","tp-fonts");
r.input.addEventListener("input",function(){set(t,r.input.value)});
if(t.kind==="font stack")r.input.addEventListener("change",function(){var v=families(r.input.value).join(", ");r.input.value=v;set(t,v)});
}
if(t.kind==="colour"){
r.pick=el("input",{type:"color","class":"tp-pick","aria-label":"Pick "+t.field,title:"Pick a colour (written as #rrggbb)"});ctl.appendChild(r.pick);
r.pick.addEventListener("input",function(){r.input.value=r.pick.value;set(t,r.pick.value)});
r.ok=el("div",{"class":"tp-ok",hidden:""});r.ranges=[];
[["L",0,1,.005],["C",0,.4,.005],["H",0,360,1]].forEach(function(p,i){var rid=id+"-"+p[0];
var rg=el("input",{type:"range",id:rid,min:p[1],max:p[2],step:p[3],"aria-label":t.field+" "+p[0]}),out=el("output",{"for":rid});
r.ok.appendChild(el("label",{"for":rid},p[0]));r.ok.appendChild(rg);r.ok.appendChild(out);r.ranges.push({rg:rg,out:out});
rg.addEventListener("input",function(){var v="oklch("+r.ranges.map(function(x){return x.rg.value}).join(" ")+")";r.input.value=v;set(t,v)})});
e.appendChild(r.ok)}
r.diag=el("div",{"class":"tp-diag"});e.appendChild(r.diag);return r}
var OKLCH=/^oklch\(\s*([0-9]*\.?[0-9]+)\s+([0-9]*\.?[0-9]+)\s+([0-9]*\.?[0-9]+)\s*\)$/i,LEN=/^(-?[0-9]*\.?[0-9]+)([a-z%]*)$/i;
function sync(r){var t=r.t,v=value(t),d=defOf(t),c=committed[bucket(t)][t.field]||"";
r.el.classList.toggle("tp-unset",!v);r.el.classList.toggle("changed",v!==c);
if(t.kind==="length"){var m=LEN.exec(v),dm=LEN.exec(d);r.input.value=v;
if(document.activeElement!==r.num)r.num.value=m?m[1]:"";r.num.placeholder=dm?dm[1]:d;
var u=m&&m[2]?m[2]:(m?r.unit.value:(dm&&dm[2]?dm[2]:"rem"));
if(u&&!UNITS.includes(u)&&!r.unit.querySelector('option[value="'+CSS.escape(u)+'"]'))r.unit.appendChild(el("option",{value:u},u));
if(u&&document.activeElement!==r.unit)r.unit.value=u;r.num.title=v?t.css+": "+v:"alder-ui: "+d;return}
if(document.activeElement!==r.input)r.input.value=v;r.input.placeholder=d;r.input.title=v?t.css:"alder-ui: "+d;
if(t.kind!=="colour")return;
r.sw.style.background="";if(v){r.sw.style.background=v;r.sw.classList.toggle("tp-bad",!CSS.supports("color",v))}else{r.sw.style.background="var("+t.css+")";r.sw.classList.remove("tp-bad")}
var hex=/^#([0-9a-f]{6})$/i.exec(v),h3=/^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(v);
if(hex)r.pick.value=v.toLowerCase();else if(h3)r.pick.value=("#"+h3[1]+h3[1]+h3[2]+h3[2]+h3[3]+h3[3]).toLowerCase();r.pick.classList.toggle("tp-nohex",!hex&&!h3);
var ok=OKLCH.exec(v);r.ok.hidden=!ok;if(ok)r.ranges.forEach(function(x,i){if(document.activeElement!==x.rg)x.rg.value=ok[i+1];x.out.textContent=ok[i+1]})}
function build(){var rb=$("tp-root"),cb=$("tp-colours");rb.textContent="";cb.textContent="";rows={};
st.tokens.forEach(function(t){var r=row(t);rows[t.field]=r;(t.scope==="root"?rb:cb).appendChild(r.el)})}
function syncAll(){Object.keys(rows).forEach(function(f){sync(rows[f]);[].forEach.call(rows[f].diag.children,function(p){if(p.hasAttribute("data-msg"))p.textContent=diagText(p)})});
$("tp-colours-h").textContent="Colours of the "+scheme+" scheme";var s=$("scheme");s.setAttribute("data-scheme",scheme);
s.setAttribute("aria-label","Scheme: "+scheme+" (switch to "+(scheme==="light"?"dark":"light")+")")}
function buttons(){var w=$("write");w.disabled=!st||broken||busy||st.building||same(working,committed);$("reset").disabled=!st||same(working,committed)}
function status(text,on){var s=$("tp-status");s.textContent=text||"";s.toggleAttribute("data-busy",!!on)}
function clearDiags(){$("tp-msgs").textContent="";Object.keys(rows).forEach(function(f){rows[f].diag.textContent=""})}
function message(text,list){var m=$("tp-msgs");m.textContent="";if(text)m.appendChild(el("p",null,text));if(list&&list.length){var ul=el("ul");list.forEach(function(x){ul.appendChild(x)});m.appendChild(ul)}}

// a diagnostic goes under the field its message names (the longest name, in backticks first)
function fieldOf(msg){var names=Object.keys(rows).sort(function(a,b){return b.length-a.length});var bt=String.fromCharCode(96);
for(var i=0;i<names.length;i++)if(msg.indexOf(bt+names[i]+bt)>=0)return names[i];
for(i=0;i<names.length;i++)if(new RegExp("\\b"+names[i]+"\\b").test(msg))return names[i];return ""}
// the scheme a colour field's diagnostic is about: the diagnostics are located on the whole
// declaration and come in record order (light, then dark), so it is the only scheme that sets the
// field, else the n-th that does when each has one, else the one whose value the browser refuses
function schemeOf(f,nth,count){var set=["light","dark"].filter(function(s){return working[s][f]});
if(set.length===1)return set[0];if(set.length===count)return set[nth]||"";
var bad=set.filter(function(s){var v=working[s][f];return !/^var\(/.test(v)&&!CSS.supports("color",v)});return bad.length===1?bad[0]:""}
function diagText(p){var s=p.getAttribute("data-scheme");return (s&&s!==scheme?s+": ":"")+p.getAttribute("data-msg")}
function showRefusal(a){clearDiags();var rest=[],ds=a.diagnostics||[],per={},seen={};
ds.forEach(function(d){var f=fieldOf(d.message||"");if(f)per[f]=(per[f]||0)+1});
ds.forEach(function(d){var f=fieldOf(d.message||"");
if(f){var p=el("div",{"data-msg":d.message||""});if(rows[f].t.scope==="scheme"){var n=seen[f]||0;seen[f]=n+1;var sc=schemeOf(f,n,per[f]);if(sc)p.setAttribute("data-scheme",sc)}
p.textContent=diagText(p);rows[f].diag.appendChild(p)}else{var li=el("li");li.appendChild(el("code",null,(d.file||"")+":"+d.line+":"+d.col+" "+(d.code||"")));li.appendChild(document.createTextNode(" "+d.message));rest.push(li)}});
message(a.error||"Nothing was written.",rest)}

function post(url,body){return fetch(url,{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)})
.then(function(r){return r.text().then(function(x){try{return {status:r.status,body:JSON.parse(x)}}catch(e){return {status:r.status,body:{error:x.trim()||("HTTP "+r.status)}}}})})}
function getState(){return fetch("/_dev/theme/state",{credentials:"same-origin",cache:"no-store"}).then(function(r){if(!r.ok)throw new Error("HTTP "+r.status);return r.json()})}
function sheetOK(){var l=document.querySelector('link[href="/static/al/al.css"]');try{return !!(l&&l.sheet&&l.sheet.cssRules.length)}catch(e){return false}}
// while a build runs: ask /state each second; when it ends, reload after a write or when the
// stylesheet did not load (no app was up), else take the state as it is now
function poll(){if(polling)return;polling=true;(function tick(){setTimeout(function(){getState().then(function(s){
if(s.building){st.building=true;buttons();return tick()}polling=false;if(wrote||!sheetOK()){location.reload();return}init(s)}).catch(function(){tick()})},1000)})()}

function init(s){st=s;$("tp-version").textContent=s.version;$("tp-file").textContent=s.file;$("write").textContent="Write to "+s.file.split("/").pop();
broken=!s.theme;committed=norm(s.theme);working=restore()||copy(committed);
if(!Object.keys(rows).length)build();syncAll();apply();buttons();
if(broken)message(s.error||(s.file+" cannot be read"));else if(!wrote)clearDiags();
if(s.building){status("building…",true);poll()}else status(same(working,committed)?"":"unsaved changes")}

$("scheme").addEventListener("click",function(){scheme=scheme==="light"?"dark":"light";syncAll();apply()});
$("reset").addEventListener("click",function(){working=copy(committed);save();syncAll();apply();buttons();clearDiags();status("")});
$("write").addEventListener("click",function(){busy=true;buttons();status("writing…",true);
post("/_dev/theme/write",{theme:working}).then(function(res){var a=res.body||{};
if(a.written){wrote=true;st.building=true;clearDiags();status("written: rebuilding…",true);poll()}
else if(a.unchanged){status(st.file+" already holds this theme");getState().then(init).catch(function(e){message("The theme state could not be read: "+e.message)})}
else{status("");showRefusal(a)}}).catch(function(e){status("");message("The write failed: "+e.message)}).then(function(){busy=false;buttons()})});
function toggleImport(open){var b=$("tp-import");b.hidden=!open;$("import").setAttribute("aria-expanded",String(open));if(open)$("import-css").focus()}
$("import").addEventListener("click",function(){toggleImport($("tp-import").hidden)});
$("import-cancel").addEventListener("click",function(){toggleImport(false)});
$("import-go").addEventListener("click",function(){var out=$("tp-import-out");out.textContent="";
post("/_dev/theme/import",{css:$("import-css").value}).then(function(res){var a=res.body||{};
if(!a.theme){out.appendChild(el("p",{"class":"tp-diag"},a.error||"Nothing could be imported."));return}
working=norm(a.theme);save();syncAll();apply();buttons();clearDiags();
var n=BUCKETS.reduce(function(k,b){return k+Object.keys(working[b]).length},0),sk=a.skipped||{n:0,kinds:[]};
out.appendChild(el("p",null,"Imported "+n+" value"+(n===1?"":"s")+(sk.n?"; skipped "+sk.n+" block"+(sk.n===1?"":"s")+" ("+(sk.kinds||[]).join(", ")+")":"")+". Write to keep them."));
var un=a.unplaced||[];if(!un.length)return;
var groups=[],by={};un.forEach(function(u){if(!by[u.block]){by[u.block]=[];groups.push(u.block)}by[u.block].push(u)});
var css=groups.map(function(g){return g+" {\n"+by[g].map(function(u){return "  /* "+u.reason.replace(/\*\//g,"* /")+" */\n  "+u.name+": "+u.value+";"}).join("\n")+"\n}"}).join("\n\n");
var cbtn=el("button",{type:"button",id:"import-copy"},"Copy"),head=el("div",{"class":"tp-bar tp-unplaced"});
head.appendChild(el("span",null,un.length+" not placed: paste them into static/app.css"));head.appendChild(cbtn);
cbtn.addEventListener("click",function(){var done=function(t){cbtn.textContent=t;setTimeout(function(){cbtn.textContent="Copy"},1500)};
try{navigator.clipboard.writeText(css).then(function(){done("Copied")},function(){done("Copy failed")})}catch(e){done("Copy failed")}});
out.appendChild(head);out.appendChild(el("pre",null,css))
}).catch(function(e){out.appendChild(el("p",{"class":"tp-diag"},"The import failed: "+e.message))})});

function tab(name){["gallery","app"].forEach(function(n){var on=n===name;$("tab-"+n).setAttribute("aria-selected",String(on));$(n==="app"?"app-pane":"gallery").hidden=!on});
if(name==="app"&&!appShown){appShown=true;$("app").src="/"}}
$("tab-gallery").addEventListener("click",function(){tab("gallery")});
$("tab-app").addEventListener("click",function(){tab("app")});
$("app").addEventListener("load",apply);
$("gallery").addEventListener("click",function(e){var a=e.target.closest&&e.target.closest("a[href]");if(a)e.preventDefault()});

getState().then(init).catch(function(e){message("The theme state could not be read: "+e.message);status("")});
})();`
