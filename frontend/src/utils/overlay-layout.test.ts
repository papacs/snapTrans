import { describe, it, expect, vi } from "vitest";
import tweet from "./fixtures/tweet-ocr.json";
import {
  buildSourceRegions,
  layoutTranslations,
  wrapOverlayText,
  approximateTextWidth,
} from "./overlay-layout";
import { parseTranslationOutput } from "./translation";
import {
  markerInsetFromPixels,
  paintTranslationOverlay,
} from "./overlay-painter";
import type { OCRBlock } from "./selection";

const response = [
  "[1] Tibo@thsottiaux·8小时",
  "[2] Brent 的加入，让桌面应用的品质和细节体验",
  "[3] 都有了显著提升。",
  "[4] 日复一日，持续改进。",
  "[5] Brent Traut @btraut·9小时",
  "[6] 上周，我为 ChatGPT 桌面应用提交了一项更新，",
  "[7] 让长线程加载更快。不只快一点，而是快了很多。",
  "[8] 长线程加载速度提升超过 90%。",
  "[9] 长线程内存占用减少超过 90%。",
  "[10] Q 107",
  "[11] t7 51",
  "[12] 2,941",
  "[13] l.l 27万",
].join("\n");

describe("local screenshot translation layout", () => {
  it("keeps closing Chinese punctuation with its preceding character", () => {
    const lines = wrapOverlayText("一二三。四五六。七八九！", 16, 48);
    expect(lines.join("")).toBe("一二三。四五六。七八九！");
    expect(lines.every((line) => !/^[。！]/.test(line))).toBe(true);
    expect(lines.every((line) => approximateTextWidth(line, 16) <= 48)).toBe(
      true,
    );
  });
  it("groups wrapped body lines without moving or covering identity and counters", () => {
    const regions = buildSourceRegions(tweet.blocks, {
      width: 593,
      height: 314,
    });
    expect(regions.map((r) => r.indices)).toEqual([
      [1, 2],
      [3],
      [5, 6],
      [7],
      [8],
    ]);
    const layout = layoutTranslations(
      regions,
      tweet.blocks,
      parseTranslationOutput(response),
      "to-zh",
    );
    expect(layout.blocks).toHaveLength(5);
    expect(layout.blocks.every((b) => b.kind === "prose")).toBe(true);
    expect(layout.blocks.every((b) => !b.truncated)).toBe(true);
    expect(layout.blocks[0]!.bounds).toEqual({
      x: 53,
      y: 34,
      width: 503,
      height: 41,
    });
    expect(layout.blocks[2]!.bounds.x).toBe(65);
    expect(layout.blocks[2]!.bounds.width).toBe(488);
    for (const block of layout.blocks)
      expect(block.bounds.x + block.bounds.width).toBeLessThan(574);
    expect(layout.blocks.map((b) => b.fontSize)).toEqual([16, 16, 16, 16, 16]);
  });
  it("keeps geometry unchanged during streaming and does not serialize unrelated columns", () => {
    const source: OCRBlock[] = [
      {
        text: "A fairly long paragraph in the left column",
        x: 0.02,
        y: 0.1,
        width: 0.4,
        height: 0.1,
      },
      {
        text: "Independent text in the right column",
        x: 0.58,
        y: 0.12,
        width: 0.4,
        height: 0.1,
      },
      {
        text: "Next left paragraph should never move",
        x: 0.02,
        y: 0.4,
        width: 0.4,
        height: 0.1,
      },
    ];
    const regions = buildSourceRegions(source, { width: 600, height: 300 });
    const first = layoutTranslations(
      regions,
      source,
      parseTranslationOutput("[1] 短译文\n[2] 右侧文字\n[3] 下一段"),
      "to-zh",
    );
    const long = layoutTranslations(
      regions,
      source,
      parseTranslationOutput(
        "[1] " + "很长的翻译内容".repeat(90) + "\n[2] 右侧文字\n[3] 下一段",
      ),
      "to-zh",
    );
    expect(long.blocks.map((b) => b.bounds)).toEqual(
      first.blocks.map((b) => b.bounds),
    );
    expect(long.blocks[1]!.bounds.y).toBe(36);
    expect(long.blocks[2]!.bounds.y).toBe(120);
    expect(long.truncated).toBe(true);
    expect(long.blocks[0]!.lines.at(-1)).toMatch(/…$/);
    expect(long.blocks[0]!.fontSize).toBeGreaterThanOrEqual(10);
  });
  it("does not suppress similar but distinct percentage list items", () => {
    const regions = buildSourceRegions(tweet.blocks, {
      width: 593,
      height: 314,
    });
    const layout = layoutTranslations(
      regions,
      tweet.blocks,
      parseTranslationOutput(response),
      "to-zh",
    );
    expect(layout.blocks.some((b) => b.text.includes("加载速度"))).toBe(true);
    expect(layout.blocks.some((b) => b.text.includes("内存占用"))).toBe(true);
  });
  it("leaves untranslated regions visible and preserves source indices when OCR ordering differs", () => {
    const reordered = [tweet.blocks[8]!, tweet.blocks[3]!];
    const regions = buildSourceRegions(reordered, { width: 593, height: 314 });
    const layout = layoutTranslations(
      regions,
      reordered,
      parseTranslationOutput("[1] 内存占用"),
      "to-zh",
    );
    expect(layout.blocks).toHaveLength(1);
    expect(layout.blocks[0]!.bounds.y).toBe(247);
  });
  it("measures CJK at full glyph width and wraps English at word boundaries", () => {
    expect(wrapOverlayText("一二三四五六", 16, 48)).toEqual([
      "一二三",
      "四五六",
    ]);
    const lines = wrapOverlayText("Hello world stays readable", 16, 100);
    expect(lines).toEqual(["Hello world", "stays", "readable"]);
    for (const line of lines)
      expect(approximateTextWidth(line, 16)).toBeLessThanOrEqual(100);
  });
  it("preserves a filled leading check icon instead of painting over it", () => {
    const width = 30,
      height = 24,
      data = new Uint8ClampedArray(width * height * 4).fill(255);
    for (let y = 2; y < 21; y++)
      for (let x = 2; x < 21; x++) {
        const i = (y * width + x) * 4;
        data[i] = 120;
        data[i + 1] = 180;
        data[i + 2] = 75;
      }
    expect(markerInsetFromPixels({ data, width, height })).toBe(24);
    const insets = tweet.blocks.map((_, i) => (i === 7 || i === 8 ? 24 : 0));
    const regions = buildSourceRegions(
      tweet.blocks,
      { width: 593, height: 314 },
      insets,
    );
    expect(regions.find((r) => r.indices[0] === 7)!.bounds.x).toBe(92);
    data.fill(255);
    expect(markerInsetFromPixels({ data, width, height })).toBe(0);
  });
  it("uses selection-local coordinates even when the selection is offset on screen", () => {
    const rect = { x: 200, y: 120, width: 593, height: 314 };
    expect(buildSourceRegions(tweet.blocks, rect)).toEqual(
      buildSourceRegions(tweet.blocks, { width: 593, height: 314 }),
    );
  });
  it("keeps list continuation lines within the indented column", () => {
    const lines = wrapOverlayText(
      "1. 一二三四五六七八九十一二三四五六七八九十",
      16,
      110,
      approximateTextWidth,
      20,
    );
    expect(lines.length).toBeGreaterThan(2);
    lines.forEach((line, index) =>
      expect(
        approximateTextWidth(line, 16) + (index > 0 ? 20 : 0),
      ).toBeLessThanOrEqual(110),
    );
  });
  it("exports the measured lines with scaling and clipping without stretching text", () => {
    const source: OCRBlock[] = [
      {
        text: "This is a long source paragraph with wrapping",
        x: 0.1,
        y: 0.1,
        width: 0.7,
        height: 0.6,
      },
    ];
    const layout = layoutTranslations(
      buildSourceRegions(source, { width: 200, height: 100 }),
      source,
      parseTranslationOutput("[1] 1. 第一行文字和第二行文字需要正确缩进显示。"),
      "to-zh",
    );
    const block = layout.blocks[0]!;
    expect(block.lines.length).toBeGreaterThan(1);
    expect(block.indent).toBeGreaterThan(0);
    const context = {
      save: vi.fn(),
      restore: vi.fn(),
      scale: vi.fn(),
      beginPath: vi.fn(),
      rect: vi.fn(),
      clip: vi.fn(),
      fillRect: vi.fn(),
      fillText: vi.fn(),
    };
    paintTranslationOverlay(
      context as unknown as CanvasRenderingContext2D,
      [
        {
          ...block,
          palette: {
            backgroundColor: "#fff",
            color: "#111",
            textShadow: "none",
            boxShadow: "none",
          },
          foreground: "#111",
        },
      ],
      2,
      1.5,
    );
    expect(context.scale).toHaveBeenCalledWith(2, 1.5);
    expect(context.rect).toHaveBeenCalledWith(20, 10, 140, block.height);
    expect(context.clip).toHaveBeenCalledOnce();
    expect(context.fillText.mock.calls).toEqual(
      block.lines.map((line, index) => [
        line,
        21 + (index > 0 ? block.indent : 0),
        10 + block.lineHeight * (index + 0.5),
      ]),
    );
  });
});
