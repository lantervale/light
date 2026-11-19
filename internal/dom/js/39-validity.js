// ---- constraint validation: what a form control says of its value

// A control is validated when it is one a form sends and the user can
// change: validityOf names what is wrong with its value now. Nothing
// is kept but the message a script gave it; :valid, :invalid,
// :in-range and :out-of-range are told to light's selectors from the
// same rules, when a selector names one (10-core.js).
const VALIDITY = ["valueMissing", "typeMismatch", "patternMismatch", "tooLong", "tooShort", "rangeUnderflow",
  "rangeOverflow", "stepMismatch", "badInput", "customError"];
const customMessages = new WeakMap();
const validityStates = new WeakMap();
const validityOwners = new WeakMap();

const textTypes = new Set(["text", "search", "url", "tel", "email", "password"]);
const DAY = 86400000;
const clock = (h, m, s = "0", ms = "0") => (h < 24 && m < 60 && s < 60
  ? ((Number(h) * 60 + Number(m)) * 60 + Number(s)) * 1000 + Number(ms.padEnd(3, "0")) : NaN);
const dayOf = (y, m, d) => {
  const at = new Date(Date.UTC(2000, m - 1, Number(d)));
  at.setUTCFullYear(Number(y));
  return Number(y) > 0 && at.getUTCMonth() === m - 1 && at.getUTCDate() === Number(d) ? at.getTime() : NaN;
};
// numberOf reads a value as the number its type counts in: a number,
// the milliseconds of a date or a time, the months of a month.
const numberOf = {
  number: (v) => (/^-?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?$/.test(v) ? Number(v) : NaN),
  date(v) {
    const found = /^(\d{4,})-(\d\d)-(\d\d)$/.exec(v);
    return found ? dayOf(found[1], found[2], found[3]) : NaN;
  },
  month(v) {
    const found = /^(\d{4,})-(\d\d)$/.exec(v);
    return found && found[2] >= 1 && found[2] <= 12 && found[1] > 0 ? (found[1] - 1970) * 12 + (found[2] - 1) : NaN;
  },
  week(v) {
    const found = /^(\d{4,})-W(\d\d)$/.exec(v);
    if (!found || found[2] < 1 || found[2] > 53 || !(found[1] > 0)) return NaN;
    const fourth = dayOf(found[1], 1, 4);
    const monday = fourth - ((new Date(fourth).getUTCDay() + 6) % 7) * DAY;
    return monday + (found[2] - 1) * 7 * DAY;
  },
  time(v) {
    const found = /^(\d\d):(\d\d)(?::(\d\d)(?:\.(\d{1,3}))?)?$/.exec(v);
    return found ? clock(found[1], found[2], found[3], found[4]) : NaN;
  },
  "datetime-local"(v) {
    const found = /^(\d{4,})-(\d\d)-(\d\d)[T ](\d\d):(\d\d)(?::(\d\d)(?:\.(\d{1,3}))?)?$/.exec(v);
    return found ? dayOf(found[1], found[2], found[3]) + clock(found[4], found[5], found[6], found[7]) : NaN;
  },
};
// stepsOf is what one step of a type is worth, the step it has when it
// names none, and where its steps start.
const stepsOf = {
  number: [1, 1, 0], date: [DAY, 1, 0], month: [1, 1, 0], week: [7 * DAY, 1, -259200000], time: [1000, 60, 0],
  "datetime-local": [1000, 60, 0],
};
const EMAIL = /^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;

// validated reports whether a control is one constraint validation
// looks at.
const validated = (control) => {
  const tag = control[TAG];
  if (!(control instanceof HTMLElement)) return false;
  if (tag === "input") {
    if (/^(hidden|reset|button)$/.test(control.type)) return false;
  } else if (tag === "button") {
    if (control.type !== "submit") return false;
  } else if (tag !== "select" && tag !== "textarea") {
    return false;
  }
  if (tag !== "select" && tag !== "button" && ownAttr(control, "readonly") !== null) return false;
  return !control.matches(":disabled") && control.closest("datalist") === null;
};
const radioGroup = (radio) => {
  const name = ownAttr(radio, "name");
  if (!name) return [radio];
  const form = radio.closest("form");
  let root = form || radio;
  while (!form && root.parentNode) root = root.parentNode;
  return foundIn(root, "input").filter((other) => other.type === "radio" && ownAttr(other, "name") === name &&
    other.closest("form") === form);
};
const missing = (control, value) => {
  const tag = control[TAG];
  if (tag === "input" && control.type === "radio") {
    const group = radioGroup(control);
    return group.some((radio) => ownAttr(radio, "required") !== null) && !group.some((radio) => radio.checked);
  }
  if (ownAttr(control, "required") === null) return false;
  if (tag === "textarea") return value === "";
  if (tag === "select") {
    const options = itemsOf(control.options);
    const chosen = options[control.selectedIndex];
    if (!chosen) return true;
    const single = ownAttr(control, "multiple") === null && !(Number(ownAttr(control, "size")) > 1);
    return single && chosen === options[0] && chosen.value === "" && chosen.parentNode === control;
  }
  if (tag !== "input") return false;
  const type = control.type;
  if (type === "checkbox") return !control.checked;
  if (type === "file") return true;
  return (textTypes.has(type) || type in numberOf) && value === "";
};
const mistyped = (control, type, value) => {
  if (value === "") return false;
  if (type === "url") return !URL.canParse(value.trim());
  if (type !== "email") return false;
  const parts = ownAttr(control, "multiple") === null ? [value] : value.split(",").map((part) => part.trim());
  return !parts.every((part) => EMAIL.test(part));
};
const unpatterned = (control, type, value) => {
  const pattern = ownAttr(control, "pattern");
  if (pattern === null || value === "" || !textTypes.has(type)) return false;
  let wanted;
  try {
    wanted = new RegExp(`^(?:${pattern})$`, "v");
  } catch {
    return false;
  }
  const parts = type === "email" && ownAttr(control, "multiple") !== null ? value.split(",").map((p) => p.trim()) : [value];
  return !parts.every((part) => wanted.test(part));
};
// ranged are the least and the most a control takes, each NaN for
// none, and the number its value is.
const ranged = (control, type) => {
  const read = numberOf[type];
  const limit = (name) => {
    const said = ownAttr(control, name);
    return said === null ? NaN : read(said);
  };
  return [limit("min"), limit("max"), read];
};
const offStep = (control, type, at, least) => {
  const said = ownAttr(control, "step");
  if (said !== null && asciiLower(said) === "any") return false;
  const [scale, usual, start] = stepsOf[type];
  const asked = said === null ? NaN : numberOf.number(said.trim());
  const step = (asked > 0 ? asked : usual) * scale;
  const given = ownAttr(control, "value");
  let base = least;
  if (Number.isNaN(base)) base = given === null ? NaN : numberOf[type](given);
  if (Number.isNaN(base)) base = start;
  const steps = (at - base) / step;
  return Math.abs(steps - Math.round(steps)) > 1e-9 * Math.max(1, Math.abs(steps));
};

const validityOf = (control) => {
  const flags = {};
  for (const name of VALIDITY) flags[name] = false;
  const tag = control[TAG];
  if (tag === "input" || tag === "select" || tag === "textarea") {
    const value = tag === "select" ? "" : control.value;
    const type = tag === "input" ? control.type : tag;
    flags.valueMissing = missing(control, value);
    if (tag === "input") {
      flags.typeMismatch = mistyped(control, type, value);
      flags.patternMismatch = unpatterned(control, type, value);
      if (type in stepsOf) {
        const [least, most, read] = ranged(control, type);
        const at = read(value);
        if (!Number.isNaN(at)) {
          flags.rangeUnderflow = at < least;
          flags.rangeOverflow = at > most && !(type === "time" && most < least);
          flags.stepMismatch = offStep(control, type, at, least);
        }
      }
    }
  }
  flags.customError = Boolean(customMessages.get(control));
  flags.valid = !VALIDITY.some((name) => flags[name]);
  return flags;
};

class ValidityState {
  constructor(key, control) {
    if (key !== token) illegal();
    validityOwners.set(this, control);
  }
}
for (const name of [...VALIDITY, "valid"]) {
  Object.defineProperty(ValidityState.prototype, name, {
    get() {
      const control = validityOwners.get(this);
      if (!control) throw new TypeError("Illegal invocation");
      return validityOf(control)[name];
    },
    enumerable: true, configurable: true,
  });
}

const validityMessages = {
  valueMissing: "Please fill out this field.", typeMismatch: "Please enter a value of the type asked for.",
  patternMismatch: "Please match the format asked for.", rangeUnderflow: "The value is below the least allowed.",
  rangeOverflow: "The value is above the most allowed.", stepMismatch: "The value is between two allowed ones.",
};
// checked reports whether a control's value satisfies it, and tells
// the control when it does not.
const checkedValid = (control) => {
  if (!validated(control) || validityOf(control).valid) return true;
  dispatch(control, new Event("invalid", { cancelable: true }));
  return false;
};
const validating = {
  validity: {
    get() {
      let state = validityStates.get(this);
      if (!state) validityStates.set(this, state = new ValidityState(token, this));
      return state;
    },
  },
  willValidate: { get() { return validated(this); } },
  validationMessage: {
    get() {
      if (!validated(this)) return "";
      const flags = validityOf(this);
      if (flags.customError) return customMessages.get(this);
      const wrong = VALIDITY.find((name) => flags[name]);
      return wrong ? validityMessages[wrong] : "";
    },
  },
};
const validatingMethods = {
  checkValidity() { return checkedValid(this); },
  reportValidity() { return checkedValid(this); },
  setCustomValidity(message) {
    if (!arguments.length) throw new TypeError("setCustomValidity takes a message.");
    customMessages.set(this, String(message));
  },
};
for (const Class of [HTMLInputElement, HTMLSelectElement, HTMLTextAreaElement, HTMLButtonElement, HTMLFieldSetElement,
  HTMLOutputElement, HTMLObjectElement]) {
  accessors(Class.prototype, validating);
  define(Class.prototype, validatingMethods);
}
const formValid = (form) => itemsOf(form.elements).map(checkedValid).every(Boolean);
define(HTMLFormElement.prototype, {
  checkValidity() { return formValid(this); },
  reportValidity() { return formValid(this); },
});
define(window, { ValidityState });

// validityAsked is a selector that names a state only these rules
// know; tellValidity gives light the controls of the tree of a node in
// each, before it matches one.
const validityAsked = /:(?:in)?valid|:(?:in|out-of)-range/i;
const tellValidity = (ref) => {
  let root = ref ? wrap(ref) : document;
  while (root.parentNode) root = root.parentNode;
  const controls = foundIn(root, "input, select, textarea, button");
  if (root instanceof HTMLElement && /^(input|select|textarea|button)$/.test(root[TAG])) controls.push(root);
  const told = [[], [], [], []];
  for (const control of controls) {
    if (!validated(control)) continue;
    const flags = validityOf(control);
    told[flags.valid ? 1 : 0].push(control[REF]);
    if (control[TAG] !== "input" || !(control.type in stepsOf)) continue;
    const [least, most] = ranged(control, control.type);
    if (Number.isNaN(least) && Number.isNaN(most)) continue;
    told[flags.rangeUnderflow || flags.rangeOverflow ? 2 : 3].push(control[REF]);
  }
  host("validity", ...told);
};
