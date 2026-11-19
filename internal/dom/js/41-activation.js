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
  if (state.trusted || state.type !== "click" || !(event instanceof MouseEvent)) return null;
  const node = activatable(path[0]) ? path[0] : state.bubbles ? path.slice(1).find(activatable) : undefined;
  if (!node) return null;
  const back = preActivated(node);
  return (stands) => {
    if (stands) activated(node);
    else if (back) back();
  };
};
