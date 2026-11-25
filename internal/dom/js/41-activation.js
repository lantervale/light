// ---- what a click a script sends does to the element it lands on

// A click a person makes is ember's to act on. One a script sends, by
// click() or dispatchEvent, is acted on here as the standard says: a
// checkbox or a radio is set before the listeners run and set back if
// one of them cancels the click, a submit button sends its form, a
// link is followed.
const checkable = (node) => node instanceof HTMLInputElement && (node.type === "checkbox" || node.type === "radio");
const activatable = (node) => node instanceof HTMLInputElement || node instanceof HTMLButtonElement ||
  (node instanceof HTMLElement && (node[TAG] === "a" || node[TAG] === "area") && node.hasAttribute("href"));
// preActivated sets a checkbox or a radio as a click does, and gives
// what sets it back.
const preActivated = (input) => {
  if (!checkable(input)) return null;
  if (input.type === "checkbox") {
    const was = input.checked;
    input.checked = !was;
    return () => { input.checked = was; };
  }
  const was = radioGroup(input).find((radio) => radio.checked) || null;
  input.checked = true;
  return () => {
    if (was && was !== input) was.checked = true;
    else if (!was) input.checked = false;
  };
};
const buttonActs = (button, type) => {
  const form = button.closest("form");
  if (!form || !button.isConnected || button.disabled) return;
  if (type === "submit" || type === "image") form.requestSubmit(button);
  else if (type === "reset") form.reset();
};
const activated = (node) => {
  if (checkable(node)) {
    if (!node.isConnected) return;
    dispatch(node, new Event("input", { bubbles: true, composed: true }));
    dispatch(node, new Event("change", { bubbles: true }));
  } else if (node instanceof HTMLInputElement || node instanceof HTMLButtonElement) buttonActs(node, node.type);
  else if (!scripted.test(node.getAttribute("href"))) host("activate", node[REF]);
};
// light runs no javascript: address, and goes to none.
const scripted = /^[\t\n\f\r ]*j[\t\n\r]*a[\t\n\r]*v[\t\n\r]*a[\t\n\r]*s[\t\n\r]*c[\t\n\r]*r[\t\n\r]*i[\t\n\r]*p[\t\n\r]*t[\t\n\r]*:/i;
// activation is asked by dispatch of each event before it is sent: for
// a click a script made it gives what to run once the listeners have,
// told whether the click stands.
activation = (event, path) => {
  const state = eventStates.get(event);
  if (state.trusted && activating(state.type, event)) [activatedAt, everActivated] = [now(), true];
  if (state.trusted || state.type !== "click" || !(event instanceof MouseEvent)) return null;
  const node = activatable(path[0]) ? path[0] : state.bubbles ? path.slice(1).find(activatable) : undefined;
  if (!node) return null;
  const back = preActivated(node);
  return (stands) => {
    if (stands) activated(node);
    else if (back) back();
  };
};

// ---- user activation

// A person's key or press activates the page: for transientActivation
// after it a script may ask, once, for what only a person may
// (showPicker), as Chrome counts it.
const transientActivation = 5000;
let activatedAt = -Infinity, everActivated = false;
const activating = (type, event) => (type === "keydown" ? event.key !== "Escape"
  : type === "mousedown" || type === "touchend" ||
    (type === "pointerdown" || type === "pointerup") && (event.pointerType === "mouse") === (type === "pointerdown"));
const activeNow = () => now() - activatedAt < transientActivation;
const userActivationKey = Symbol("userActivation");
class UserActivation {
  constructor(key) { if (key !== userActivationKey) illegal(); }
  get hasBeenActive() { return everActivated; }
  get isActive() { return activeNow(); }
}
define(window, { UserActivation });
navigator.userActivation = new UserActivation(userActivationKey);

// showPicker opens no picker of light's, but asks what a browser asks
// first, and takes the activation it needs.
const readOnlyApplies = /^(text|search|url|tel|email|password|date|month|week|time|datetime-local|number)$/;
const picking = (control, readOnly) => {
  if (control.matches(":disabled") || readOnly && control.hasAttribute("readonly")) {
    throw new DOMException("A control disabled or read-only shows no picker.", "InvalidStateError");
  }
  if (!activeNow()) throw new DOMException("showPicker() needs a person's activation.", "NotAllowedError");
};
define(HTMLInputElement.prototype, {
  showPicker() {
    picking(this, readOnlyApplies.test(this.type));
    activatedAt = -Infinity;
  },
});
define(HTMLSelectElement.prototype, {
  showPicker() {
    picking(this, false);
    if (!host("laid", this[REF])) throw new DOMException("A select not shown shows no picker.", "NotSupportedError");
    activatedAt = -Infinity;
  },
});
