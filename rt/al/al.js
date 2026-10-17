/*! alder-ui v0.3.3 | Apache-2.0 | built by scripts/build.py from components/*.js; do not edit. */

// ---- components/al-shim.js ----
{
// Polyfills for browsers without invoker commands (Chromium < 135, Safari < 26,
// Firefox < 140): <button commandfor="id" command="show-modal | close |
// toggle-popover | show-popover | hide-popover"> works as the platform's.
// Also re-announces closing as the bubbling al-close event on al-dialog,
// al-popover and al-menu. (popover="hint" needs no shim: al-tooltip.js shows
// and hides its tooltip on hover and focus, so an older browser treating
// "hint" as "manual" behaves the same.)
if (!("commandForElement" in HTMLButtonElement.prototype)) {
  document.addEventListener("click", (e) => {
    const b = e.target.closest("button[commandfor]");
    const t = b && !b.disabled && document.getElementById(b.getAttribute("commandfor"));
    if (!t) return;
    const c = b.getAttribute("command");
    if (c === "show-modal" && !t.open) t.showModal();
    else if (c === "close") t.close();
    else if (c === "toggle-popover") t.togglePopover();
    else if (c === "show-popover") t.showPopover();
    else if (c === "hide-popover") t.hidePopover();
  });
}
const closed = (e, sel) => {
  const host = e.target.closest && e.target.closest(sel);
  if (host) host.dispatchEvent(new CustomEvent("al-close", { bubbles: true }));
};
document.addEventListener("close", (e) => closed(e, "al-dialog"), true);
document.addEventListener("toggle", (e) => e.newState === "closed" && closed(e, "al-popover, al-menu"), true);
}

// ---- components/al-alert.js ----
{
// Optional enhancement for <al-alert dismissible>: adds a close button that
// removes the alert. Load with <script type="module" src="al-alert.js">.
// A DOM morph that replaces the alert's children removes the added button.
customElements.get("al-alert") || customElements.define("al-alert", class extends HTMLElement {
  connectedCallback() {
    if (!this.hasAttribute("dismissible") || this.querySelector(":scope > .al-alert-dismiss")) return;
    const b = Object.assign(document.createElement("button"), { type: "button", className: "al-alert-dismiss", textContent: "×" });
    b.setAttribute("aria-label", "Dismiss");
    b.addEventListener("click", () => this.remove());
    this.append(b);
  }
});
}

// ---- components/al-menu.js ----
{
// Optional enhancement for <al-menu popover id="…">: role="menu" and
// "menuitem", roving focus with the arrow keys, Home and End, focus on the
// first item when it opens, and closing on select. Fires al-select
// (detail.item) when an item is chosen. Load with <script type="module">.
customElements.get("al-menu") || customElements.define("al-menu", class extends HTMLElement {
  items() {
    return [...this.querySelectorAll(":scope > a, :scope > button")].filter((i) => i.getAttribute("aria-disabled") !== "true" && !i.disabled);
  }
  connectedCallback() {
    this.setAttribute("role", "menu");
    for (const i of this.querySelectorAll(":scope > a, :scope > button")) {
      i.setAttribute("role", "menuitem");
      i.tabIndex = -1;
    }
    this.addEventListener("toggle", (e) => e.newState === "open" && this.items()[0]?.focus());
    this.addEventListener("keydown", (e) => {
      const items = this.items(), at = items.indexOf(document.activeElement);
      const to = { ArrowDown: at + 1, ArrowUp: at - 1, Home: 0, End: items.length - 1 }[e.key];
      if (to === undefined) return;
      e.preventDefault();
      items[(to + items.length) % items.length]?.focus();
    });
    this.addEventListener("click", (e) => {
      const item = e.target.closest('[role="menuitem"]');
      if (!item || !this.contains(item)) return;
      this.dispatchEvent(new CustomEvent("al-select", { bubbles: true, detail: { item } }));
      this.hidePopover();
    });
  }
});
}

// ---- components/al-tabs.js ----
{
// Optional enhancement for <al-tabs>. With no script the radio group selects.
// This adds the tabs ARIA pattern: a <div role="tablist"> of <button role="tab">
// (aria-selected, aria-controls, roving tabindex) built from the labels' text and
// put before the panels (the radio labels are hidden; the radios stay and drive
// the CSS), role="tabpanel" + id + aria-labelledby on each panel, click/arrows/
// `selected`, <al-tab name>, and al-select (detail.tab); al-tabs-keys.js adds the click
// and the arrow keys. A MutationObserver
// rebuilds all of it, and the chosen tab, after a DOM patch (a morph) undoes it.
let seq = 0;
customElements.get("al-tabs") || customElements.define("al-tabs", class extends HTMLElement {
  sync() {
    this.rs.forEach((r, i) => {
      this.list.children[i].setAttribute("aria-selected", String(r.checked));
      this.list.children[i].tabIndex = r.checked ? 0 : -1;
    });
  }
  pick(i) {
    this.rs[i].checked = true;
    this.rs[i].dispatchEvent(new Event("change", { bubbles: true }));
    this.list.children[i].focus();
  }
  build() {
    const tabs = [...this.querySelectorAll(":scope > al-tab")].filter((t) => t.querySelector("input[type=radio]"));
    const labels = tabs.map((t) => t.querySelector("input[type=radio]").closest("label"));
    const panels = tabs.map((t) => t.querySelector(":scope > al-panel"));
    if (this.list?.parentNode === this && labels.every((l) => l.hidden) && panels.every((p) => !p || p.id)) return;
    const first = !this.list, list = document.createElement("div");
    list.setAttribute("role", "tablist");
    this.rs = tabs.map((tab) => tab.querySelector("input[type=radio]"));
    this.rs.forEach((radio, i) => {
      const tab = tabs[i], panel = panels[i], b = list.appendChild(document.createElement("button"));
      if (!radio.name && tab.getAttribute("name")) radio.name = tab.getAttribute("name");
      if (first ? tab.hasAttribute("selected") : this.at === i) radio.checked = true;
      Object.assign(b, { type: "button", id: `al-tabs-${this.n}-${i}`, textContent: labels[i].textContent.trim() });
      b.setAttribute("role", "tab");
      labels[i].hidden = true;
      if (!panel) return;
      panel.id ||= `${b.id}-panel`;
      panel.setAttribute("role", "tabpanel");
      panel.setAttribute("aria-labelledby", b.id);
      b.setAttribute("aria-controls", panel.id);
    });
    this.list?.remove();
    this.prepend((this.list = list));
    this.sync();
  }
  connectedCallback() {
    if (this.n) return;
    this.n = ++seq;
    this.build();
    this.at = this.rs.findIndex((r) => r.checked);
    new MutationObserver(() => this.build()).observe(this, { childList: true, subtree: true, attributes: true, attributeFilter: ["hidden", "id"] });
    this.addEventListener("change", (e) => {
      this.at = this.rs.indexOf(e.target);
      this.sync();
      const tab = e.target.closest("al-tab");
      if (tab) this.dispatchEvent(new CustomEvent("al-select", { bubbles: true, detail: { tab } }));
    });
  }
});
}

// ---- components/al-tabs-keys.js ----
{
// Optional: the tab buttons al-tabs.js builds respond to a click and to the arrow
// keys, Home and End (each picks the tab: it checks the radio, so al-select fires).
// One listener on the document serves every <al-tabs>, rebuilt or not.
document.addEventListener("click", (e) => {
  const tabs = e.target.closest?.("al-tabs [role=tab]")?.closest("al-tabs");
  const i = tabs ? [...tabs.list.children].indexOf(e.target.closest("[role=tab]")) : -1;
  if (i >= 0) tabs.pick(i);
});
document.addEventListener("keydown", (e) => {
  const tabs = e.target.closest?.("al-tabs [role=tab]")?.closest("al-tabs");
  if (!tabs) return;
  const i = [...tabs.list.children].indexOf(e.target), last = tabs.rs.length - 1;
  const to = { ArrowRight: i < last ? i + 1 : 0, ArrowLeft: i > 0 ? i - 1 : last, Home: 0, End: last }[e.key];
  if (to === undefined) return;
  e.preventDefault();
  tabs.pick(to);
});
}

// ---- components/al-devpanel.js ----
{
// <al-devpanel>: a <nav> of buttons (aria-controls="<section id>") switches its
// <section>s; a button opens the panel, the selected one closes it again. The
// script hides the sections that are not selected and re-applies the selection
// after any DOM patch (a MutationObserver), so a server that patches the
// panel's inner content need not render `hidden` or `aria-selected`.
customElements.get("al-devpanel") || customElements.define("al-devpanel", class extends HTMLElement {
  buttons() {
    return [...this.querySelectorAll(":scope > nav > button")];
  }
  apply() {
    const bs = this.buttons(), b = bs.find((x) => x.getAttribute("aria-controls") === this.sel) || bs[0];
    if (!b) return;
    this.sel = b.getAttribute("aria-controls");
    for (const x of bs) if (x.getAttribute("aria-selected") !== String(x === b)) x.setAttribute("aria-selected", String(x === b));
    for (const s of this.querySelectorAll(":scope > section")) if (s.hidden !== (s.id !== this.sel)) s.hidden = s.id !== this.sel;
  }
  connectedCallback() {
    this.sel = this.buttons().find((x) => x.getAttribute("aria-selected") === "true")?.getAttribute("aria-controls");
    this.apply();
    new MutationObserver(() => this.apply()).observe(this, {
      childList: true, subtree: true, attributes: true, attributeFilter: ["hidden", "aria-selected"],
    });
    this.addEventListener("click", (e) => {
      const b = e.target.closest("nav > button");
      if (!b || !this.buttons().includes(b)) return;
      const close = this.hasAttribute("open") && b.getAttribute("aria-selected") === "true";
      this.sel = b.getAttribute("aria-controls");
      this.apply();
      this.toggleAttribute("open", !close);
    });
  }
});
}

// ---- components/al-toast.js ----
{
// Optional enhancement for <al-toast> inside <al-toasts popover="manual">:
// shows the region when a toast connects, sets role="status", closes the toast
// after `timeout` ms (default 5000; 0 keeps it) and hides an empty region.
// Fires al-close on the region when a toast closes. Load with <script type="module">.
customElements.get("al-toast") || customElements.define("al-toast", class extends HTMLElement {
  connectedCallback() {
    this.hasAttribute("role") || this.setAttribute("role", "status");
    const region = this.closest("al-toasts");
    if (region && region.hasAttribute("popover") && !region.matches(":popover-open")) region.showPopover();
    const ms = this.hasAttribute("timeout") ? Number(this.getAttribute("timeout")) : 5000;
    if (ms > 0) this.timer = setTimeout(() => this.close(), ms);
  }
  disconnectedCallback() {
    clearTimeout(this.timer);
  }
  close() {
    const region = this.closest("al-toasts");
    this.remove();
    if (!region) return;
    region.dispatchEvent(new CustomEvent("al-close", { bubbles: true, detail: { toast: this } }));
    if (!region.querySelector("al-toast") && region.matches(":popover-open")) region.hidePopover();
  }
});
}

// ---- components/al-tooltip.js ----
{
// Optional enhancement for <al-tooltip for="id">text</al-tooltip>: makes it a
// popover="hint" (the browser treats it as "manual" where "hint" is unknown;
// the behaviour is the same), anchors it to the trigger, wires
// aria-describedby, and shows it on hover and keyboard focus; Escape hides it.
// The setup is re-applied each time it is shown, so a morph that drops the
// attributes the script added (the server never writes them) does not break it.
customElements.get("al-tooltip") || customElements.define("al-tooltip", class extends HTMLElement {
  connectedCallback() {
    const trigger = document.getElementById(this.getAttribute("for"));
    if (!trigger || this.wired) return;
    this.wired = true;
    const tid = "al-tip-" + Math.random().toString(36).slice(2, 8);
    const wire = () => {
      this.id ||= tid;
      this.hasAttribute("popover") || this.setAttribute("popover", "hint");
      this.setAttribute("role", "tooltip");
      this.setAttribute("anchor", trigger.id);
      trigger.setAttribute("aria-describedby", this.id);
    };
    wire();
    const show = () => (wire(), this.matches(":popover-open") || this.showPopover());
    const hide = () => this.matches(":popover-open") && this.hidePopover();
    for (const [type, f] of [["pointerenter", show], ["focus", show], ["pointerleave", hide], ["blur", hide]]) trigger.addEventListener(type, f);
    trigger.addEventListener("keydown", (e) => e.key === "Escape" && hide());
  }
});
}
