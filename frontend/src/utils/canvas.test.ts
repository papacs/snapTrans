import { afterEach, expect, it, vi } from "vitest";
import { encodeTemporaryPNG } from "./canvas";

afterEach(() => vi.restoreAllMocks());
it.each([false, true])("releases temporary pixels after PNG encoding (failure=%s)", failure => {
  const canvas = document.createElement("canvas");
  canvas.width = 3840; canvas.height = 2160;
  vi.spyOn(canvas, "toDataURL").mockImplementation(() => {
    if (failure) throw new Error("encoding failed");
    return "data:image/png;base64,encoded";
  });
  if (failure) expect(() => encodeTemporaryPNG(canvas)).toThrow("encoding failed");
  else expect(encodeTemporaryPNG(canvas)).toBe("data:image/png;base64,encoded");
  expect(canvas.width * canvas.height).toBe(0);
});
