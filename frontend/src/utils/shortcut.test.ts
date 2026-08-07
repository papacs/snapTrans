import { describe, expect, it } from "vitest";
import { shortcutKeyFromKeyboardEvent } from "./shortcut";

function event(partial: Partial<KeyboardEvent>): KeyboardEvent {
  return {
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    metaKey: false,
    key: "",
    code: "",
    ...partial
  } as KeyboardEvent;
}

describe("shortcutKeyFromKeyboardEvent", () => {
  it("builds simple modifier+letter combinations", () => {
    expect(shortcutKeyFromKeyboardEvent(event({ ctrlKey: true, key: "q", code: "KeyQ" }))).toBe("Ctrl+Q");
    expect(shortcutKeyFromKeyboardEvent(event({ altKey: true, key: "Q", code: "KeyQ" }))).toBe("Alt+Q");
    expect(shortcutKeyFromKeyboardEvent(event({ ctrlKey: true, shiftKey: true, key: "s", code: "KeyS" }))).toBe("Ctrl+Shift+S");
  });

  it("handles digits and function keys", () => {
    expect(shortcutKeyFromKeyboardEvent(event({ ctrlKey: true, key: "1", code: "Digit1" }))).toBe("Ctrl+1");
    expect(shortcutKeyFromKeyboardEvent(event({ altKey: true, key: "F12", code: "F12" }))).toBe("Alt+F12");
  });

  it("handles space and other named keys", () => {
    expect(shortcutKeyFromKeyboardEvent(event({ altKey: true, key: " ", code: "Space" }))).toBe("Alt+Space");
    expect(shortcutKeyFromKeyboardEvent(event({ ctrlKey: true, key: "Tab", code: "Tab" }))).toBe("Ctrl+Tab");
    expect(shortcutKeyFromKeyboardEvent(event({ ctrlKey: true, key: "Enter", code: "Enter" }))).toBe("Ctrl+Enter");
  });

  it("rejects events without a modifier", () => {
    expect(shortcutKeyFromKeyboardEvent(event({ key: "q", code: "KeyQ" }))).toBeNull();
  });

  it("rejects modifier-only key presses", () => {
    expect(shortcutKeyFromKeyboardEvent(event({ ctrlKey: true, key: "Control", code: "ControlLeft" }))).toBeNull();
    expect(shortcutKeyFromKeyboardEvent(event({ altKey: true, key: "Alt", code: "AltLeft" }))).toBeNull();
  });
});
