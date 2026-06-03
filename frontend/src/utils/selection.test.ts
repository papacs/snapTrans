import { describe, expect, it } from "vitest";
import {
  fontSizeForOCRBlock,
  fontSizeForTranslationBlock,
  mapCssRectToImageRect,
  mapOCRBlockToSelection,
  normalizeResultBox,
  normalizeRect,
  ocrScaleForRect,
  sampleCanvasColor,
  translationPaletteForColor,
  translationPaletteForLuminance,
  wrapTranslationText
} from "./selection";

describe("normalizeRect", () => {
  it("returns positive dimensions when dragging from bottom-right to top-left", () => {
    expect(normalizeRect({ x: 320, y: 240 }, { x: 120, y: 90 })).toEqual({
      x: 120,
      y: 90,
      width: 200,
      height: 150
    });
  });

  it("keeps zero-width drags normalized without negative values", () => {
    expect(normalizeRect({ x: 40, y: 80 }, { x: 40, y: 10 })).toEqual({
      x: 40,
      y: 10,
      width: 0,
      height: 70
    });
  });
});

describe("mapCssRectToImageRect", () => {
  it("maps CSS coordinates to source image pixels for high DPI screenshots", () => {
    expect(
      mapCssRectToImageRect(
        { x: 100, y: 50, width: 300, height: 150 },
        { width: 1280, height: 720 },
        { width: 2560, height: 1440 }
      )
    ).toEqual({ x: 200, y: 100, width: 600, height: 300 });
  });

  it("clamps selections that extend beyond the visible canvas", () => {
    expect(
      mapCssRectToImageRect(
        { x: 900, y: 500, width: 400, height: 300 },
        { width: 1000, height: 600 },
        { width: 2000, height: 1200 }
      )
    ).toEqual({ x: 1800, y: 1000, width: 200, height: 200 });
  });
});

describe("normalizeResultBox", () => {
  it("keeps the result box anchored to the selected region", () => {
    const box = normalizeResultBox(
      { x: 120, y: 220, width: 260, height: 80 },
      { width: 900, height: 700 }
    );

    expect(box).toEqual({ x: 120, y: 220, width: 260, height: 80 });
  });

  it("only clamps the result box when the selected region would leave the viewport", () => {
    const box = normalizeResultBox(
      { x: 380, y: 690, width: 90, height: 45 },
      { width: 433, height: 721 }
    );

    expect(box.x).toBeLessThanOrEqual(337);
    expect(box.y).toBeLessThanOrEqual(668);
    expect(box.width).toBeGreaterThanOrEqual(90);
    expect(box.height).toBeGreaterThanOrEqual(45);
  });
});

describe("ocrScaleForRect", () => {
  it("upscales small text crops before OCR", () => {
    expect(ocrScaleForRect({ x: 0, y: 0, width: 60, height: 18 })).toBeGreaterThan(1);
  });

  it("keeps large crops at native scale", () => {
    expect(ocrScaleForRect({ x: 0, y: 0, width: 640, height: 240 })).toBe(1);
  });
});

describe("mapOCRBlockToSelection", () => {
  it("maps normalized OCR block coordinates back into the selected CSS region", () => {
    expect(
      mapOCRBlockToSelection(
        { text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 },
        { x: 148, y: 26, width: 452, height: 52 }
      )
    ).toEqual({ x: 428, y: 36, width: 81, height: 19 });
  });

  it("sizes inline translations from the OCR block height", () => {
    expect(fontSizeForOCRBlock({ text: "Positive", x: 0, y: 0, width: 0.2, height: 0.36 }, 52)).toBe(18);
  });

  it("keeps short legend translations readable", () => {
    expect(fontSizeForTranslationBlock("\u6b63\u9762", { x: 0, y: 0, width: 81, height: 19 })).toBe(16);
  });

  it("reduces long title translations to fit their original text box", () => {
    expect(
      fontSizeForTranslationBlock("\u70ed\u95e8\u6a21\u578b\u7684\u603b\u63d0\u53ca\u6b21\u6570\u4e0e\u7528\u6237\u60c5\u611f\u5206\u6790", {
        x: 0,
        y: 0,
        width: 420,
        height: 24
      })
    ).toBeLessThanOrEqual(17);
  });
});

describe("translationPaletteForLuminance", () => {
  it("uses light text on dark screenshot regions", () => {
    const palette = translationPaletteForLuminance(0.12);

    expect(palette.color).toBe("#f8fafc");
    expect(palette.backgroundColor).toBe("rgba(31, 31, 31, 0.98)");
  });

  it("uses dark text on light screenshot regions", () => {
    const palette = translationPaletteForLuminance(0.82);

    expect(palette.color).toBe("#0f172a");
    expect(palette.backgroundColor).toBe("rgba(209, 209, 209, 0.98)");
  });
});

describe("translationPaletteForColor", () => {
  it("uses the sampled original background color for dark regions", () => {
    const palette = translationPaletteForColor({ red: 19, green: 25, blue: 39, luminance: 0.1 });

    expect(palette.backgroundColor).toBe("rgba(19, 25, 39, 0.98)");
    expect(palette.color).toBe("#f8fafc");
    expect(palette.boxShadow).toBe("none");
  });
});

describe("sampleCanvasColor", () => {
  it("uses the dominant background color instead of averaging bright text pixels into gray", () => {
    const canvas = {
      width: 4,
      height: 1,
      getBoundingClientRect: () => ({ left: 0, top: 0, width: 4, height: 1 }),
      getContext: () => ({
        getImageData: () => ({
          data: new Uint8ClampedArray([
            20, 24, 32, 255,
            22, 26, 34, 255,
            24, 28, 36, 255,
            250, 250, 250, 255
          ])
        })
      })
    } as unknown as HTMLCanvasElement;

    const sampled = sampleCanvasColor(canvas, { x: 0, y: 0, width: 4, height: 1 });

    expect(sampled?.red).toBeLessThan(30);
    expect(sampled?.green).toBeLessThan(34);
    expect(sampled?.blue).toBeLessThan(42);
  });
});

describe("wrapTranslationText", () => {
  it("keeps short labels on one line", () => {
    expect(wrapTranslationText("\u6b63\u9762", 16, 81)).toEqual(["\u6b63\u9762"]);
  });

  it("wraps long translations so they can be drawn completely inside narrow regions", () => {
    expect(wrapTranslationText("\u70ed\u95e8\u6a21\u578b\u7684\u603b\u63d0\u53ca\u6b21\u6570", 12, 72).length).toBeGreaterThan(1);
  });
});
