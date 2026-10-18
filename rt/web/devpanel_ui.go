package web

// panelUI is the dev panel's own behaviour and style beyond the design system's <al-devpanel>:
// the Errors tab, the strip's button, and (with the panel's chrome) the resize handle and the
// minimised tab. It is written once into a full document, after the panel, in dev mode only, so
// production output carries none of it. The panel's host element is kept by every morph
// (data-ignore-morph), so its attributes (open, data-tab, data-min) and its --_ph property
// survive; the script re-adds what an inner patch of the panel removes.
const panelUI = `<style id="_alder_ui_css">` + panelUICSS + `</style><script id="_alder_ui">` + panelUIJS + `</script>`

const panelUICSS = `al-devpanel>nav>button[data-errors]{color:var(--destructive,#b91c1c);font-weight:600}` +
	`al-devpanel>nav>button[data-errors] [data-badge]{display:inline-block;min-inline-size:1.4em;margin-inline-start:.25em;padding:0 .35em;border-radius:999px;background:var(--destructive,#b91c1c);color:var(--destructive-foreground,#fff);text-align:center}` +
	`al-devpanel>section#_alder_tab_errors ._alder-errs{display:block}`

const panelUIJS = `(function(){if(window._alderUI)return;window._alderUI=1;
var P="_alder_panel",E="_alder_tab_errors",last="";
function panel(){return document.getElementById(P)}
function selectTab(p,id){p.setAttribute("data-tab",id);
var b=[].filter.call(p.querySelectorAll(":scope>nav>button"),function(x){return x.getAttribute("aria-controls")===id})[0];
if(b&&b.getAttribute("aria-selected")!=="true"){var o=p.hasAttribute("open");b.click();p.toggleAttribute("open",o)}}
function pickTab(p,id){selectTab(p,id);p.setAttribute("open","")}
function errs(){var p=panel(),nav=p&&p.querySelector(":scope>nav");if(!nav)return;
var l=document.querySelector("#_alder_errors ._alder-errs"),n=l?Number(l.getAttribute("data-count")):0,
btn=nav.querySelector("button[data-errors]"),sec=p.querySelector(":scope>section#"+E);
if(!n){if(btn||sec){var was=p.getAttribute("data-tab")===E;if(btn)btn.remove();if(sec)sec.remove();p.removeAttribute("data-errors");
if(was){var f=nav.querySelector("button[aria-controls]");if(f)p.setAttribute("data-tab",f.getAttribute("aria-controls"))}}last="";return}
if(p.getAttribute("data-errors")!==String(n))p.setAttribute("data-errors",n);
if(!btn){btn=document.createElement("button");btn.type="button";btn.setAttribute("data-errors","");btn.setAttribute("aria-controls",E);btn.setAttribute("aria-selected","false");
btn.innerHTML='Errors <span data-badge></span>';nav.insertBefore(btn,nav.querySelector("button[aria-controls]"))}
var bd=btn.querySelector("[data-badge]");if(bd.textContent!==String(n))bd.textContent=n;
var sig=l.innerHTML;
if(!sec){sec=document.createElement("section");sec.id=E;sec.hidden=true;nav.after(sec)}
if(sec.getAttribute("data-sig")!==sig){sec.setAttribute("data-sig",sig);sec.innerHTML="<h3>Errors</h3>"+l.outerHTML}
if(sig!==last){last=sig;if(!p.hasAttribute("data-min"))pickTab(p,E)}
else if(p.getAttribute("data-tab")===E)selectTab(p,E)}
function watch(){var c=document.getElementById("_alder_errors"),p=panel();
if(c)new MutationObserver(errs).observe(c,{childList:true,subtree:true});
if(p)new MutationObserver(errs).observe(p,{childList:true});
errs()}
document.addEventListener("click",function(ev){var t=ev.target.closest&&ev.target.closest("[data-open-errors]"),p=panel();
if(t&&p){pickTab(p,E)}});
if(document.readyState==="loading")document.addEventListener("DOMContentLoaded",watch);else watch()})()`
