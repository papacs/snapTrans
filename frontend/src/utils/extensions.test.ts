import { describe, it, expect } from "vitest";
import {
  comparePixels,
  detectSensitiveBlocks,
  filterLibrary,
  libraryMarkdown,
  mergeTextLines,
} from "./extensions";
import { defaultFeatures, normalizeFeatures } from "./features";
import type { HistoryEntry } from "../services/backend";
describe("feature migration", () => {
  it("keeps experiments off and preserves explicit disables", () => {
    expect(normalizeFeatures()).toEqual(defaultFeatures);
    expect(normalizeFeatures({ pin: false, shareCards: true })).toMatchObject({
      pin: false,
      shareCards: true,
      textExtraction: true,
      tableExtraction: true,
      memeExplanation: false,
      learningCards: false,
      imageCompare: false,
    });
  });
});
describe("text formatting", () => {
  it("merges CJK without inserted spaces, preserves paragraph boundaries", () => {
    expect(mergeTextLines("中\n文\n字")).toBe("中文字");
    expect(mergeTextLines("第一行\n第二行\n第三行\n\nHello\r\nworld")).toBe(
      "第一行第二行第三行\n\nHello world",
    );
  });
});
describe("redaction geometry", () => {
  it("covers full matching blocks with padding and clamps to image bounds", () => {
    const matches = detectSensitiveBlocks(
      [
        {
          text: "email me: person@example.org",
          x: 0,
          y: 0.2,
          width: 0.8,
          height: 0.1,
        },
        { text: "电话 138 1234 5678", x: 0.8, y: 0.9, width: 0.3, height: 0.2 },
        { text: "version 123456789", x: 0.1, y: 0.1, width: 0.1, height: 0.1 },
        { text: "bad@example.org", x: NaN, y: 0, width: 0.1, height: 0.1 },
      ],
      1000,
      500,
    );
    expect(matches).toHaveLength(2);
    expect(matches[0].rect).toEqual({ x: 0, y: 97, width: 803, height: 57 });
    expect(matches[1].rect).toEqual({ x: 797, y: 447, width: 203, height: 53 });
  });
});
describe("comparison", () => {
  it("ignores small differences and highlights changed pixels", () => {
    const before = new Uint8ClampedArray([0, 0, 0, 255, 100, 100, 100, 255]);
    const after = new Uint8ClampedArray([12, 0, 0, 255, 200, 100, 100, 255]);
    const result = comparePixels(before, after);
    expect(result.changed).toBe(1);
    expect([...result.pixels]).toEqual([12, 0, 0, 255, 255, 48, 90, 255]);
    expect([...after]).toEqual([12, 0, 0, 255, 200, 100, 100, 255]);
    expect(() => comparePixels(before, new Uint8ClampedArray(4))).toThrow();
  });
});
describe("saved library", () => {
  const entries: HistoryEntry[] = [
    {
      id: "1",
      source: "HELLO",
      translation: "你好",
      direction: "to-zh",
      timestamp: "",
      favorite: true,
    },
    {
      id: "2",
      source: "world",
      translation: "世界",
      example: "hello there",
      direction: "to-zh",
      timestamp: "",
      kind: "learning",
    },
  ];
  it("searches source, translation and examples with independent filters", () => {
    expect(filterLibrary(entries, " hello ", "all")).toHaveLength(2);
    expect(
      filterLibrary(entries, "你好", "favorites").map((e) => e.id),
    ).toEqual(["1"]);
    expect(
      filterLibrary(entries, "hello", "learning").map((e) => e.id),
    ).toEqual(["2"]);
  });
  it("exports examples and neutralizes HTML in source text", () => {
    expect(
      libraryMarkdown([{ ...entries[1], source: "<script>bad</script>" }]),
    ).toContain("&lt;script&gt;");
    expect(libraryMarkdown(entries)).toContain("> hello there");
  });
});
