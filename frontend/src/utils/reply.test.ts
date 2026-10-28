import { describe, expect, it } from "vitest";
import { replyPanelPosition } from "./reply";

describe("reply panel placement", () => {
  it("stays beside the source when there is room", () => {
    expect(replyPanelPosition({x:100,y:80,width:300,height:200},{width:1280,height:900}).left).toBe(416);
  });
  it("keeps the complete panel on screen near edges and on small displays", () => {
    for (const viewport of [{width:1920,height:1080},{width:640,height:480},{width:360,height:640}]) {
      const panel=replyPanelPosition({x:viewport.width-100,y:viewport.height-30,width:100,height:20},viewport);
      expect(panel.left).toBeGreaterThanOrEqual(16);expect(panel.top).toBeGreaterThanOrEqual(16);
      expect(panel.left+panel.width).toBeLessThanOrEqual(viewport.width-16);
      expect(panel.top+panel.height).toBeLessThanOrEqual(viewport.height-16);
    }
  });
});
