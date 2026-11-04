import { describe, expect, it } from "vitest";
import { rememberedReplyScene, replyPanelPosition } from "./reply";

describe("remembered reply scene", () => {
  it.each([null,"general","work","social","unknown"])("uses the same stored scene or default for %s", value => {
    const previous = sessionStorage.getItem("snaptrans.reply.scene");
    try {
      if(value === null) sessionStorage.removeItem("snaptrans.reply.scene");
      else sessionStorage.setItem("snaptrans.reply.scene",value);
      expect(rememberedReplyScene()).toBe(value === "work" || value === "social" ? value : "general");
    } finally {
      if(previous === null) sessionStorage.removeItem("snaptrans.reply.scene");
      else sessionStorage.setItem("snaptrans.reply.scene",previous);
    }
  });
});

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
