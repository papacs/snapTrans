const KEY_ALIASES: Record<string, string> = {
  " ": "Space",
  Spacebar: "Space",
  Enter: "Enter",
  Return: "Enter",
  Tab: "Tab",
  Escape: "Esc",
  Esc: "Esc",
  ArrowUp: "Up",
  ArrowDown: "Down",
  ArrowLeft: "Left",
  ArrowRight: "Right",
  PageUp: "PgUp",
  PageDown: "PgDn",
  Home: "Home",
  End: "End",
  Delete: "Del",
  Backspace: "Backspace",
  Insert: "Ins",
};

const MODIFIER_KEYS = new Set(["Control", "Alt", "Shift", "Meta", "Ctrl", "Win"]);

// shortcutKeyFromKeyboardEvent builds a shortcut string such as "Ctrl+Shift+S"
// from a keyboard event. It returns null when the event is not a usable
// shortcut (no modifier, or only modifier keys pressed).
export function shortcutKeyFromKeyboardEvent(event: KeyboardEvent): string | null {
  const parts: string[] = [];
  if (event.ctrlKey) {
    parts.push("Ctrl");
  }
  if (event.altKey) {
    parts.push("Alt");
  }
  if (event.shiftKey) {
    parts.push("Shift");
  }
  if (event.metaKey) {
    parts.push("Win");
  }
  if (parts.length === 0) {
    return null;
  }

  const key = event.key ?? "";
  let token: string | null = null;

  if (/^[a-zA-Z]$/.test(key)) {
    token = key.toUpperCase();
  } else if (/^[0-9]$/.test(key)) {
    token = key;
  } else if (/^F([1-9]|1[0-9]|20)$/i.test(key)) {
    token = key.toUpperCase();
  } else if (MODIFIER_KEYS.has(key)) {
    token = null;
  } else if (KEY_ALIASES[key]) {
    token = KEY_ALIASES[key];
  } else if (KEY_ALIASES[event.code]) {
    token = KEY_ALIASES[event.code];
  }

  if (!token) {
    return null;
  }
  return [...parts, token].join("+");
}
