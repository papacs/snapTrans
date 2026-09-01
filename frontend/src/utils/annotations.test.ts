import { describe, expect, it, vi } from "vitest";

import { renderAnnotation, toolbarPosition } from "./annotations";

describe("toolbarPosition", () => {
  it("right-aligns the toolbar below the selection when space is available", () => {
    expect(
      toolbarPosition(
        { x: 120, y: 80, width: 600, height: 220 },
        { width: 1000, height: 700 },
        { width: 560, height: 48 }
      )
    ).toEqual({ x: 160, y: 312 });
  });

  it("moves the toolbar above a selection near the bottom edge", () => {
    expect(
      toolbarPosition(
        { x: 700, y: 500, width: 260, height: 170 },
        { width: 1000, height: 700 },
        { width: 240, height: 48 }
      )
    ).toEqual({ x: 700, y: 440 });
  });

  it("keeps the toolbar inside the right edge of the viewport", () => {
    expect(
      toolbarPosition(
        { x: 900, y: 80, width: 220, height: 220 },
        { width: 1000, height: 700 },
        { width: 360, height: 48 }
      )
    ).toEqual({ x: 632, y: 312 });
  });
});

describe("arrow rendering", () => {
  it("uses a single rounded shaft and a closed filled arrowhead", () => {
    const stroke = vi.fn();
    const fill = vi.fn();
    const closePath = vi.fn();
    const lineTo = vi.fn();
    const context = {
      beginPath: vi.fn(),
      closePath,
      fill,
      lineTo,
      moveTo: vi.fn(),
      restore: vi.fn(),
      save: vi.fn(),
      stroke
    } as unknown as CanvasRenderingContext2D;

    renderAnnotation(context, {
      tool: "arrow",
      start: { x: 10, y: 40 },
      end: { x: 110, y: 40 },
      color: "#ff4d4f",
      width: 3
    });

    expect(stroke).toHaveBeenCalledTimes(1);
    expect(closePath).toHaveBeenCalledTimes(1);
    expect(fill).toHaveBeenCalledTimes(1);
    const headBase = lineTo.mock.calls.slice(-2);
    expect(Math.abs(headBase[0][1] - headBase[1][1])).toBeGreaterThanOrEqual(18);
  });
});
