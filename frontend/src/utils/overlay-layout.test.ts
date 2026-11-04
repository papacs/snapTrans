import { describe, it, expect, vi } from "vitest";
import tweet from "./fixtures/tweet-ocr.json";
import {
  buildSourceRegions,
  layoutTranslations,
  wrapOverlayText,
  approximateTextWidth,
  type SourceRegion,
  type OverlayBlock,
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
  it("fits short native selections without changing their left-aligned prose style", () => {
    const sources: OCRBlock[] = [{ text: "Hi 聪,", x: 0.1, y: 0.1, width: 0.43, height: 0.23 }];
    const regions = buildSourceRegions(sources, { width: 100, height: 100 }, [], true);
    const layout = layoutTranslations(regions, sources, parseTranslationOutput("[1] 嗨，聪，"), "to-zh");
    expect(layout.blocks[0]!.kind).toBe("prose");
    expect(layout.blocks[0]!.lines).toEqual(["嗨，聪，"]);
    expect(layout.truncated).toBe(false);
  });

  it("keeps a single short label inside a tight eight-pixel OCR height", () => {
    const sources: OCRBlock[] = [{ text: "NEXT", x: 0.1, y: 0.1, width: 0.33, height: 0.1 }];
    const layout = layoutTranslations(buildSourceRegions(sources, { width: 100, height: 80 }), sources, parseTranslationOutput("[1] 下一步"), "to-zh");
    const label = layout.blocks[0]!;
    expect(label.lines).toEqual(["下一步"]);
    expect(label.lineHeight).toBeLessThanOrEqual(label.height);
    expect(label.truncated).toBe(false);
  });

  it("uses the whole paragraph for unevenly distributed indexed translations", () => {
    const text = [
      "The rankings order every model by tokens sent through the OpenRouter API,",
      "split by task, so you can see what developers run for coding, for agents, or for",
      "extraction. Pick one near the top for the work you have in mind. Any slug on",
      "the page drops into the model field of the same request, on the same key.",
    ];
    const rows = [[27, 139, 513, 20], [26, 163, 517, 20], [26, 188, 509, 20], [26, 212, 495, 20]];
    const sources = rows.map(([x, y, width, height], index) => ({
      text: text[index]!, x: x! / 589, y: y! / 507, width: width! / 589, height: height! / 507,
    }));
    const translated = [
      "排名按通过 OpenRouter API 发送的 token 数量为每个模型排序，",
      "并按任务分类，因此你可以看到开发者在编程、智能体或",
      "信息提取任务中使用什么模型。为你要做的工作挑一个排名靠前的模型。页面上的任何模型标识都可以填入同一密钥下",
      "同一请求的 model 字段。",
    ];
    const parsed = parseTranslationOutput(translated.map((line, index) => `[${index + 1}] ${line}`).join("\n"));
    const layout = layoutTranslations(buildSourceRegions(sources, { width: 589, height: 507 }), sources, parsed, "to-zh");
    expect(layout.blocks).toHaveLength(1);
    expect(layout.blocks[0]!.text).toBe(translated.join(""));
    expect(layout.truncated).toBe(false);
    expect(layout.blocks[0]!.fontSize).toBeGreaterThanOrEqual(13);
  });
  it("shows complete greetings and short section labels inside their OCR boxes", () => {
    const size = { width: 589, height: 507 };
    const box = (text: string, x: number, y: number, width: number, height: number): OCRBlock => ({
      text, x: x / size.width, y: y / size.height, width: width / size.width, height: height / size.height,
    });
    const sources = [
      box("Hi 聪,", 25, 31, 43, 23),
      box("Pick a model from what developers actually run.", 27, 92, 492, 24),
      box("The rankings order every model by tokens sent through the OpenRouter API,", 27, 139, 513, 20),
      box("split by task, so you can see what developers run for coding, for agents, or for", 26, 163, 517, 20),
      box("extraction. Pick one near the top for the work you have in mind. Any slug on", 26, 188, 509, 20),
      box("the page drops into the model field of the same request, on the same key.", 26, 212, 495, 20),
      box("See this week's rankings", 49, 268, 159, 24),
      box("NEXT", 27, 366, 33, 16),
      box("Or let the router choose: typesafe/jev-router picks a model and a reasoning effort", 26, 392, 514, 20),
      box("for each request, billed at the price of the model that answers.", 27, 416, 387, 17),
    ];
    const regions = buildSourceRegions(sources, size);
    const layout = layoutTranslations(regions, sources, parseTranslationOutput("[1] 嗨，聪，\n[8] 下一步"), "to-zh");
    for (const [index, translation] of [[0, "嗨，聪，"], [7, "下一步"]] as const) {
      const label = layout.blocks.find(block => block.indices[0] === index)!;
      expect(label.truncated).toBe(false);
      expect(label.lines).toEqual([translation]);
      expect(label.fontSize).toBeGreaterThanOrEqual(8);
      expect(label.lines.length * label.lineHeight).toBeLessThanOrEqual(label.height);
      expect(approximateTextWidth(translation, label.fontSize)).toBeLessThanOrEqual(label.bounds.width - 2);
      expect(label.bounds).toEqual(regions.find(region => region.indices[0] === index)!.bounds);
    }
  });
  it("only measures changed regions while text streams and recalculates new geometry", () => {
    const sources: OCRBlock[] = Array.from({ length: 40 }, (_, index) => ({
      text: "A complete independent paragraph to translate.",
      x: 0, y: index / 40, width: 1, height: 0.02,
    }));
    const regions = buildSourceRegions(sources, { width: 500, height: 2000 });
    const output = sources.map((_, index) => `[${index + 1}] 这是已经完成的独立段落翻译。`).join("\n");
    const cache = new WeakMap<SourceRegion, OverlayBlock>();
    const measure = vi.fn(approximateTextWidth);
    const initial = layoutTranslations(regions, sources, parseTranslationOutput(output), "to-zh", measure, cache);
    expect(initial.blocks).toHaveLength(40);
    const initialMeasurements = measure.mock.calls.length;
    measure.mockClear();
    const streamed = layoutTranslations(regions, sources, parseTranslationOutput(output + "追加的译文"), "to-zh", measure, cache);
    expect(streamed.blocks[0]).toBe(initial.blocks[0]);
    expect(streamed.blocks[39]).not.toBe(initial.blocks[39]);
    expect(measure.mock.calls.length).toBeLessThan(initialMeasurements / 10);

    const resized = layoutTranslations(buildSourceRegions(sources, { width: 200, height: 2000 }), sources, parseTranslationOutput(output), "to-zh", measure, cache);
    expect(resized.blocks[0]).not.toBe(initial.blocks[0]);
    expect(resized.blocks[0]!.bounds.width).toBe(200);
  });
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
