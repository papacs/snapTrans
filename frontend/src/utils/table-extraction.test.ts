import { describe, expect, it } from "vitest";
import { inferTable, tableToMarkdown, tableToTSV } from "./table-extraction";

describe("table extraction", () => {
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
