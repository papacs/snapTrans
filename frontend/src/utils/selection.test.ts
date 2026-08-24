import { describe, expect, it } from "vitest";
import {
	clampPointToBounds,
	fitTranslationFontSize,
	fontSizeForOCRBlock,
  fontSizeForTranslationBlock,
  mapCssRectToImageRect,
  mapOCRBlockToSelection,
  normalizeResultBox,
  normalizeRect,
  ocrScaleForRect,
  sampleCanvasColor,
	sampleCanvasForegroundColor,
	selectionBadgePosition,
  shouldUseFlowTranslationLayout,
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

describe("capture pointer helpers", () => {
	it("clamps pointer coordinates to the capture surface", () => {
		expect(clampPointToBounds({ x: -40, y: 760 }, { width: 1280, height: 720 })).toEqual({
			x: 0,
			y: 720
		});
	});

	it("places the size badge above the selection when space is available", () => {
		expect(
			selectionBadgePosition(
				{ x: 120, y: 80, width: 320, height: 160 },
				{ width: 1280, height: 720 }
			)
		).toEqual({ x: 120, y: 48 });
	});

	it("moves the size badge below a selection near the top edge", () => {
		expect(
			selectionBadgePosition(
				{ x: 1240, y: 4, width: 32, height: 24 },
				{ width: 1280, height: 720 }
			)
		).toEqual({ x: 1184, y: 36 });
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

  it("uses per-display DPI scales when display metadata is present", () => {
    const displays = [
      { x: 0, y: 0, width: 1920, height: 1080, scale: 1.5 },
      { x: 1920, y: 0, width: 1280, height: 1080, scale: 1.0 }
    ];
    const cssSize = { width: 3200, height: 1080 };
    const imageSize = { width: 4160, height: 1620 };

    const withinPrimary = mapCssRectToImageRect(
      { x: 960, y: 270, width: 480, height: 270 },
      cssSize,
      imageSize,
      displays
    );

    expect(withinPrimary).toEqual({ x: 1440, y: 405, width: 720, height: 405 });

    const withinSecondary = mapCssRectToImageRect(
      { x: 2880, y: 270, width: 320, height: 270 },
      cssSize,
      imageSize,
      displays
    );

    expect(withinSecondary).toEqual({ x: 2880, y: 270, width: 320, height: 270 });
  });

  it("falls back to ratio mapping when display metadata is absent", () => {
    expect(
      mapCssRectToImageRect(
        { x: 100, y: 50, width: 300, height: 150 },
        { width: 1280, height: 720 },
        { width: 2560, height: 1440 },
        []
      )
    ).toEqual({ x: 200, y: 100, width: 600, height: 300 });
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

  it("shrinks wrapped translations until they fit the available region height", () => {
    const text = "\u8fd9\u662f\u4e00\u4e2a\u5185\u5bb9\u8f83\u5bc6\u96c6\u7684\u7ffb\u8bd1\u7ed3\u679c\uff0c\u9700\u8981\u540c\u65f6\u53c2\u8003\u9009\u533a\u7684\u5bbd\u5ea6\u548c\u9ad8\u5ea6\u6765\u81ea\u52a8\u7f29\u5c0f\u5b57\u4f53\uff0c\u907f\u514d\u5185\u5bb9\u88ab\u906e\u6321\u6216\u51fa\u73b0\u6eda\u52a8\u6761\u3002";
    const rect = { x: 0, y: 0, width: 240, height: 54 };
    const fontSize = fitTranslationFontSize(text, rect, 20, 1.3);
    const lineHeight = Math.round(fontSize * 1.3);
    const wrappedHeight = wrapTranslationText(text, fontSize, rect.width).length * lineHeight;

    expect(fontSize).toBeLessThan(20);
    expect(wrappedHeight).toBeLessThanOrEqual(rect.height);
  });
});

describe("shouldUseFlowTranslationLayout", () => {
  it("keeps dense navigation and form controls in their original rows", () => {
    const blocks = [
      { text: "All Models", x: 0.01, y: 0.04, width: 0.07, height: 0.08 },
      { text: "Add Model", x: 0.09, y: 0.04, width: 0.07, height: 0.08 },
      { text: "LLM Credentials", x: 0.17, y: 0.04, width: 0.12, height: 0.08 },
      { text: "Pass-Through Endpoints", x: 0.3, y: 0.04, width: 0.16, height: 0.08 },
      { text: "Health Status", x: 0.47, y: 0.04, width: 0.1, height: 0.08 },
      { text: "Model Retry Settings", x: 0.58, y: 0.04, width: 0.14, height: 0.08 },
      {
        text: "To access these models: Create a Virtual Key without selecting a team on the Virtual Keys page",
        x: 0.02,
        y: 0.58,
        width: 0.78,
        height: 0.12
      }
    ];

    expect(shouldUseFlowTranslationLayout(blocks)).toBe(false);
  });

  it("uses flow layout for paragraph-like rows", () => {
    const blocks = [
      {
        text: "The space of AI-assisted coding is evolving rapidly.",
        x: 0.02,
        y: 0.04,
        width: 0.82,
        height: 0.12
      },
      {
        text: "Each day, this pipeline reviews new discussions and records the results.",
        x: 0.02,
        y: 0.22,
        width: 0.88,
        height: 0.12
      }
    ];

    expect(shouldUseFlowTranslationLayout(blocks)).toBe(true);
  });

  it("keeps vertically stacked form labels and helper text in place", () => {
    const blocks = [
      { text: "LiteLLM", x: 0.38, y: 0.09, width: 0.12, height: 0.05 },
      { text: "Login", x: 0.41, y: 0.17, width: 0.07, height: 0.04 },
      { text: "Access your LiteLLM Admin UI.", x: 0.36, y: 0.23, width: 0.2, height: 0.03 },
      { text: "Default Credentials", x: 0.3, y: 0.31, width: 0.15, height: 0.04 },
      {
        text: "By default, Username is admin and Password is your set LiteLLM Proxy MASTER_KEY",
        x: 0.3,
        y: 0.37,
        width: 0.34,
        height: 0.07
      },
      {
        text: "Need to setup UI credentials or SSO? Check the documentation.",
        x: 0.3,
        y: 0.48,
        width: 0.3,
        height: 0.06
      },
      { text: "Username", x: 0.25, y: 0.58, width: 0.08, height: 0.03 },
      { text: "admin", x: 0.26, y: 0.63, width: 0.06, height: 0.03 },
      { text: "Password", x: 0.25, y: 0.7, width: 0.08, height: 0.03 },
      { text: "Login", x: 0.42, y: 0.8, width: 0.05, height: 0.04 },
      { text: "Login with SSO", x: 0.38, y: 0.89, width: 0.13, height: 0.04 }
    ];

    expect(shouldUseFlowTranslationLayout(blocks)).toBe(false);
  });
});

describe("translationPaletteForLuminance", () => {
  it("uses light text on dark screenshot regions", () => {
    const palette = translationPaletteForLuminance(0.12);

    expect(palette.color).toBe("#f8fafc");
    expect(palette.backgroundColor).toBe("rgba(31, 31, 31, 1)");
  });

  it("uses dark text on light screenshot regions", () => {
    const palette = translationPaletteForLuminance(0.82);

    expect(palette.color).toBe("#0f172a");
    expect(palette.backgroundColor).toBe("rgba(209, 209, 209, 1)");
  });
});

describe("translationPaletteForColor", () => {
  it("uses the sampled original background color for dark regions", () => {
    const palette = translationPaletteForColor({ red: 19, green: 25, blue: 39, luminance: 0.1 });

    expect(palette.backgroundColor).toBe("rgba(19, 25, 39, 1)");
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

  it("samples bright source text color separately from the dominant dark background", () => {
    const canvas = {
      width: 8,
      height: 1,
      getBoundingClientRect: () => ({ left: 0, top: 0, width: 8, height: 1 }),
      getContext: () => ({
        getImageData: () => ({
          data: new Uint8ClampedArray([
            28, 29, 36, 255,
            30, 31, 38, 255,
            29, 30, 37, 255,
            31, 32, 39, 255,
            255, 83, 22, 255,
            250, 91, 30, 255,
            30, 31, 38, 255,
            29, 30, 37, 255
          ])
        })
      })
    } as unknown as HTMLCanvasElement;

    const foreground = sampleCanvasForegroundColor(canvas, { x: 0, y: 0, width: 8, height: 1 });

    expect(foreground?.red).toBeGreaterThan(240);
    expect(foreground?.green).toBeGreaterThan(75);
    expect(foreground?.green).toBeLessThan(100);
    expect(foreground?.blue).toBeLessThan(40);
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
