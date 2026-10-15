/*! alder-ui v0.2.0 | Apache-2.0 | built by scripts/build.py from components/*.js; do not edit. */

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
// Optional enhancement for <al-tabs>. The radio group already gives selection
// and arrow keys with no script; this adds Home/End, the `selected` attribute
// (the server's pick), the group name from <al-tab name>, role="tabpanel",
// and the al-select event (detail.tab). Load with <script type="module">.
customElements.get("al-tabs") || customElements.define("al-tabs", class extends HTMLElement {
  radios() {
    return [...this.querySelectorAll(":scope > al-tab > label > input[type=radio]")];
  }
  connectedCallback() {
    for (const tab of this.querySelectorAll(":scope > al-tab")) {
      const radio = tab.querySelector("input[type=radio]");
      if (radio && !radio.name && tab.getAttribute("name")) radio.name = tab.getAttribute("name");
      if (radio && tab.hasAttribute("selected")) radio.checked = true;
      tab.querySelector(":scope > al-panel")?.setAttribute("role", "tabpanel");
    }
    this.addEventListener("keydown", (e) => {
      const rs = this.radios(), to = { Home: 0, End: rs.length - 1 }[e.key];
      if (to === undefined || !rs[to]) return;
      e.preventDefault();
      rs[to].checked = true;
      rs[to].focus();
      rs[to].dispatchEvent(new Event("change", { bubbles: true }));
    });
    this.addEventListener("change", (e) => {
      const tab = e.target.closest("al-tab");
      if (tab) this.dispatchEvent(new CustomEvent("al-select", { bubbles: true, detail: { tab } }));
    });
  }
});

// <al-devpanel>: a <nav> of buttons (aria-controls="<section id>") switches its
// <section>s; a button opens the panel, the selected one closes it again.
customElements.get("al-devpanel") || customElements.define("al-devpanel", class extends HTMLElement {
  connectedCallback() {
    const buttons = [...this.querySelectorAll(":scope > nav > button")];
    const show = (b) => {
      for (const x of buttons) x.setAttribute("aria-selected", String(x === b));
      for (const s of this.querySelectorAll(":scope > section")) s.hidden = s.id !== b.getAttribute("aria-controls");
    };
    if (buttons[0]) show(buttons[0]);
    this.addEventListener("click", (e) => {
      const b = e.target.closest("nav > button");
      if (!b || !buttons.includes(b)) return;
      const close = this.hasAttribute("open") && b.getAttribute("aria-selected") === "true";
      show(b);
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
customElements.get("al-tooltip") || customElements.define("al-tooltip", class extends HTMLElement {
  connectedCallback() {
    const trigger = document.getElementById(this.getAttribute("for"));
    if (!trigger || this.wired) return;
    this.wired = true;
    this.id ||= "al-tip-" + Math.random().toString(36).slice(2, 8);
    this.hasAttribute("popover") || this.setAttribute("popover", "hint");
    this.setAttribute("role", "tooltip");
    this.setAttribute("anchor", trigger.id);
    trigger.setAttribute("aria-describedby", this.id);
    const show = () => this.matches(":popover-open") || this.showPopover();
    const hide = () => this.matches(":popover-open") && this.hidePopover();
    for (const [type, f] of [["pointerenter", show], ["focus", show], ["pointerleave", hide], ["blur", hide]]) trigger.addEventListener(type, f);
    trigger.addEventListener("keydown", (e) => e.key === "Escape" && hide());
  }
});
}
