import { describe, expect, it } from "vitest";
import { inferTable, tableToMarkdown, tableToTSV } from "./table-extraction";

describe("table extraction", () => {
  it("keeps wide table columns separate and ignores a trailing pagination row", () => {
    const width = 1739;
    const height = 272;
    const block = (text: string, x: number, y: number, blockWidth: number, blockHeight: number) => ({
      text,
      x: x / width,
      y: y / height,
      width: blockWidth / width,
      height: blockHeight / height,
    });

    const table = inferTable([
      block("发生时间", 38, 54, 67, 28),
      block("对象", 240, 56, 39, 25),
      block("诊断场景", 467, 54, 67, 28),
      block("级别", 717, 56, 38, 25),
      block("诊断摘要", 813, 50, 68, 29),
      block("状态", 1338, 56, 39, 25),
      block("最近更新", 1445, 54, 67, 28),
      block("操作", 1648, 56, 37, 25),
      block("2026-09-01 12:15:00", 41, 114, 147, 22),
      block("低压凝汽器", 243, 114, 79, 23),
      block("清洁系数下降", 467, 101, 95, 28),
      block("凝汽器换热", 469, 127, 68, 23),
      block("严重", 720, 112, 43, 27),
      block("凝汽器稳定清洁系数低于运行关注线", 815, 102, 234, 27),
      block("当前异常", 1342, 114, 66, 23),
      block("2026-09-01 14:40:00", 1447, 114, 145, 22),
      block("共1条", 1440, 179, 54, 23),
      block("10条/页", 1623, 177, 71, 28),
    ], { width, height });

    expect(table).toEqual({
      cells: [
        ["发生时间", "对象", "诊断场景", "级别", "诊断摘要", "状态", "最近更新", "操作"],
        [
          "2026-09-01 12:15:00",
          "低压凝汽器",
          "清洁系数下降凝汽器换热",
          "严重",
          "凝汽器稳定清洁系数低于运行关注线",
          "当前异常",
          "2026-09-01 14:40:00",
          "",
        ],
      ],
      rows: 2,
      columns: 8,
    });
  });

  it("clusters slightly misaligned OCR blocks into rows and columns", () => {
    const table = inferTable([
      { text: "Name", x: 0.04, y: 0.08, width: 0.18, height: 0.06 },
      { text: "Score", x: 0.53, y: 0.075, width: 0.16, height: 0.06 },
      { text: "Alice", x: 0.03, y: 0.28, width: 0.2, height: 0.06 },
      { text: "92", x: 0.52, y: 0.285, width: 0.08, height: 0.06 },
      { text: "Bob", x: 0.045, y: 0.49, width: 0.14, height: 0.06 },
      { text: "88", x: 0.535, y: 0.495, width: 0.08, height: 0.06 },
    ]);
    expect(table).toEqual({
      cells: [["Name", "Score"], ["Alice", "92"], ["Bob", "88"]],
      rows: 3,
      columns: 2,
    });
  });

  it("merges multiple OCR fragments assigned to the same cell", () => {
    expect(
      inferTable([
        { text: "合", x: 0.05, y: 0.1, width: 0.05, height: 0.05 },
        { text: "计", x: 0.055, y: 0.105, width: 0.05, height: 0.05 },
        { text: "100", x: 0.55, y: 0.1, width: 0.1, height: 0.05 },
        { text: "税", x: 0.05, y: 0.3, width: 0.05, height: 0.05 },
        { text: "6", x: 0.55, y: 0.3, width: 0.05, height: 0.05 },
      ]).cells,
    ).toEqual([["合计", "100"], ["税", "6"]]);
  });

  it("exports safe TSV and Markdown", () => {
    const cells = [["Item", "A|B"], ["line\nbreak", "10\t20"]];
    expect(tableToTSV(cells)).toBe("Item\tA|B\nline break\t10 20");
    expect(tableToMarkdown(cells)).toBe(
      "| Item | A\\|B |\n| --- | --- |\n| line<br>break | 10\t20 |",
    );
  });

  it("rejects unbounded grids and ignores invalid blocks", () => {
    expect(inferTable([{ text: "bad", x: Number.NaN, y: 0, width: 1, height: 1 }])).toEqual({cells: [], rows: 0, columns: 0});
    expect(() => inferTable(Array.from({length: 31}, (_, index) => ({text:String(index),x:index/31,y:0,width:0.001,height:0.001})))).toThrow("TABLE_TOO_LARGE");
  });
});
