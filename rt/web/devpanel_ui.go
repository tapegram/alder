package web

import "strconv"

// panelUI is the dev panel's own behaviour and style beyond the design system's <al-devpanel>:
// the Errors tab and the strip's button; the resize handle on the panel's top edge (the open
// section's height, 120 px to 80% of the viewport, as --_ph); a button, the tab itself and
// Ctrl+Shift+D that minimise it to a small tab in the bottom-right corner (data-min, out of the
// page's flow); and the state (open, minimised, height, last tab) kept per browser in
// localStorage, every access in try/catch, so the panel works when storage throws. It is written once into a full document, after the panel, in dev mode only, so
// production output carries none of it. The panel's host element is kept by every morph
// (data-ignore-morph), so its attributes (open, data-tab, data-min) and its --_ph property
// survive; the script re-adds what an inner patch of the panel removes.
//
// The style is added to the document's head by the script, not written beside it: a body morph
// would drop a <style> in the body, which the panel's host element, kept by data-ignore-morph,
// does not protect.
var panelUI = `<script id="_alder_ui">` + panelUIJS + `</script>`

const panelUICSS = `al-devpanel>nav>button[data-errors]{color:color-mix(in srgb,var(--destructive,#b91c1c) 60%,var(--foreground,#000));font-weight:600}` +
	`al-devpanel>nav>button[data-errors] [data-badge]{display:inline-block;box-sizing:border-box;min-inline-size:1.4em;margin-inline-start:.25em;padding:0 .35em;border-radius:999px;background:var(--destructive,#b91c1c);color:var(--color-white,#fff);text-align:center}` +
	`al-devpanel>section#_alder_tab_errors ._alder-errs{display:block}` +
	`al-devpanel>section#_alder_tab_errors>h3{margin:0 0 .5rem;font-size:inherit;font-weight:600;line-height:inherit}` +
	`al-devpanel>nav{padding-inline-end:2.5rem}` +
	`al-devpanel[open][data-sized]>section:not([hidden]){box-sizing:border-box;block-size:clamp(120px,var(--_ph,50dvh),80dvh);max-block-size:none}` +
	`al-devpanel>[data-grip]{position:absolute;inset:-4px 0 auto 0;block-size:8px;cursor:ns-resize;touch-action:none;z-index:1}` +
	`al-devpanel:not([open])>[data-grip]{display:none}` +
	`al-devpanel>[data-grip]:hover,al-devpanel>[data-grip]:focus-visible{background:var(--ring,currentColor);opacity:.4;outline:none}` +
	`al-devpanel>[data-minbtn]{position:absolute;inset:.15rem .5rem auto auto;margin:0;padding:.1rem .5rem;border:0;border-radius:var(--radius-sm,4px);background:transparent;color:var(--muted-foreground,currentColor);font:inherit;cursor:pointer}` +
	`al-devpanel>[data-minbtn]:hover{background:var(--accent,rgba(127,127,127,.2));color:var(--accent-foreground,currentColor)}` +
	`al-devpanel[data-min]{display:flex;align-items:center;gap:.5rem;position:fixed;inset:auto .75rem .75rem auto;inline-size:auto;z-index:2147482000;border:1px solid var(--border,#888);border-radius:var(--radius-md,6px);box-shadow:0 2px 10px rgba(0,0,0,.3);cursor:pointer}` +
	`al-devpanel[data-min]>section,al-devpanel[data-min]>[data-grip],al-devpanel[data-min]>[data-minbtn],al-devpanel[data-min]>nav>:not([data-ms]){display:none}` +
	`al-devpanel[data-min]>nav{flex-wrap:nowrap;margin:0;padding:.25rem .25rem .25rem .6rem}` +
	`al-devpanel[data-min]:hover,al-devpanel[data-min]:focus-visible{background:var(--accent,rgba(127,127,127,.2))}` +
	`al-devpanel[data-min][data-errors]::after{content:attr(data-errors);margin-inline-end:.4rem;padding:0 .4em;border-radius:999px;background:var(--destructive,#b91c1c);color:var(--color-white,#fff);font-weight:600}`

var panelUIJS = `(function(){if(window._alderUI)return;window._alderUI=1;
var css=document.createElement("style");css.id="_alder_ui_css";css.textContent=` + strconv.Quote(panelUICSS) + `;document.head.appendChild(css);
var P="_alder_panel",E="_alder_tab_errors",K="_alder_panel_ui",last="",tab="",drag=null;
function panel(){return document.getElementById(P)}
function store(f){try{return f(window.localStorage)}catch(e){return null}}
function load(){return store(function(s){return JSON.parse(s.getItem(K)||"{}")})||{}}
function height(p){var v=parseFloat(p.style.getPropertyValue("--_ph"));return v>0?v:0}
function save(){var p=panel();if(!p)return;var t=p.getAttribute("data-tab")||"";if(t&&t!==E)tab=t;
var v=JSON.stringify({open:p.hasAttribute("open"),min:p.hasAttribute("data-min"),h:height(p),tab:tab});
store(function(s){if(s.getItem(K)!==v)s.setItem(K,v)})}
function setH(p,h){h=Math.round(Math.max(120,Math.min(h,innerHeight*0.8)));p.style.setProperty("--_ph",h+"px");p.setAttribute("data-sized","");return h}
function setMin(p,on){p.toggleAttribute("data-min",on);
if(on){p.setAttribute("role","button");p.setAttribute("tabindex","0");p.setAttribute("aria-label","Restore the dev panel")}
else{p.removeAttribute("role");p.removeAttribute("tabindex");p.removeAttribute("aria-label")}save()}
function selectTab(p,id){p.setAttribute("data-tab",id);
var b=[].filter.call(p.querySelectorAll(":scope>nav>button"),function(x){return x.getAttribute("aria-controls")===id})[0];
if(b&&b.getAttribute("aria-selected")!=="true"){var o=p.hasAttribute("open");b.click();p.toggleAttribute("open",o)}}
function pickTab(p,id){selectTab(p,id);p.setAttribute("open","")}
function chrome(p){if(!p.querySelector(":scope>[data-grip]")){var g=document.createElement("div");g.setAttribute("data-grip","");g.setAttribute("role","separator");
g.setAttribute("aria-orientation","horizontal");g.setAttribute("aria-label","Resize the dev panel");g.tabIndex=0;p.prepend(g)}
if(!p.querySelector(":scope>[data-minbtn]")){var m=document.createElement("button");m.type="button";m.setAttribute("data-minbtn","");
m.setAttribute("aria-label","Minimise the dev panel (Ctrl+Shift+D)");m.title="Minimise (Ctrl+Shift+D)";m.textContent="\u2013";p.append(m)}}
function errs(){var p=panel(),nav=p&&p.querySelector(":scope>nav");if(!nav)return;chrome(p);
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
if(p){var st=load();chrome(p);
if(st.h>0)setH(p,st.h);
if(st.tab){tab=st.tab;selectTab(p,st.tab)}
p.toggleAttribute("open",!!st.open);
if(st.min)setMin(p,true);
new MutationObserver(errs).observe(p,{childList:true});
new MutationObserver(save).observe(p,{attributes:true,attributeFilter:["open","data-tab","data-min","data-sized"]})}
errs()}
document.addEventListener("click",function(ev){var p=panel(),t=ev.target.closest&&ev.target.closest("[data-open-errors]");
if(!p)return;
if(t){if(p.hasAttribute("data-min"))setMin(p,false);pickTab(p,E);return}
if(ev.target.closest("[data-minbtn]")){setMin(p,true);return}
if(p.hasAttribute("data-min")&&p.contains(ev.target))setMin(p,false)});
document.addEventListener("pointerdown",function(ev){var g=ev.target.closest&&ev.target.closest("[data-grip]"),p=panel();if(!g||!p||ev.button)return;
var sec=p.querySelector(":scope>section:not([hidden])");drag={p:p,y:ev.clientY,h:sec?sec.getBoundingClientRect().height:0};
ev.preventDefault();try{g.setPointerCapture(ev.pointerId)}catch(e){}});
document.addEventListener("pointermove",function(ev){if(drag)setH(drag.p,drag.h+drag.y-ev.clientY)});
function end(){if(drag){drag=null;save()}}
document.addEventListener("pointerup",end);document.addEventListener("pointercancel",end);
document.addEventListener("keydown",function(ev){var p=panel();if(!p)return;
if(ev.ctrlKey&&ev.shiftKey&&!ev.altKey&&!ev.metaKey&&(ev.key==="D"||ev.key==="d")){ev.preventDefault();setMin(p,!p.hasAttribute("data-min"));return}
if(p.hasAttribute("data-min")&&ev.target===p&&(ev.key==="Enter"||ev.key===" ")){ev.preventDefault();setMin(p,false);return}
if(ev.target.closest&&ev.target.closest("[data-grip]")&&(ev.key==="ArrowUp"||ev.key==="ArrowDown")){ev.preventDefault();
var sec=p.querySelector(":scope>section:not([hidden])"),h=height(p)||(sec?sec.getBoundingClientRect().height:120);
setH(p,h+(ev.key==="ArrowUp"?24:-24));save()}});
if(document.readyState==="loading")document.addEventListener("DOMContentLoaded",watch);else watch()})()`
