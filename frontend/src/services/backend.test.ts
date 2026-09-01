import { afterEach, describe, expect, it, vi } from "vitest";
import { copyImageDataUrl, copyText, defaultConfig, saveConfig, testConnection, loadConfig, clearHistory, getHistory } from "./backend";

afterEach(() => { delete window.go; delete window.runtime; vi.restoreAllMocks(); vi.unstubAllGlobals(); });
describe("native boundary", () => {
 it("sends the draft and autostart together to the desktop", async () => {
  const save = vi.fn(async () => {});
  const test = vi.fn(async () => {});
  window.go = {main:{App:{SaveSettings:save, TestConnection:test}}} as unknown as Window["go"];
  const draft = {...defaultConfig, apiKey:"test-key"};
  await saveConfig(draft, true);
  await testConnection(draft);
  expect(save).toHaveBeenCalledWith(draft,true);
  expect(test).toHaveBeenCalledWith(draft);
 });
 it("surfaces a native clipboard rejection", async () => {
  window.runtime = { ClipboardSetText:vi.fn(async()=>false) } as unknown as Window["runtime"];
  await expect(copyText("sample")).rejects.toThrow("Clipboard");
 });
 it("never substitutes base64 text for an unavailable image clipboard", async () => {
  const writeText = vi.fn();
  vi.stubGlobal("navigator", {clipboard:{writeText}});
  vi.stubGlobal("ClipboardItem", undefined);
  vi.stubGlobal("fetch", vi.fn(async()=>({blob:async()=>new Blob(["png"],{type:"image/png"})})));
  await expect(copyImageDataUrl("data:image/png;base64,test")).rejects.toThrow("Image clipboard unavailable");
  expect(writeText).not.toHaveBeenCalled();
 });
});


describe("extension storage",()=>{
 it("migrates old browser settings without enabling experiments",async()=>{
  localStorage.setItem("snaptrans.config",JSON.stringify({features:{pin:false}}));
  const cfg=await loadConfig();expect(cfg.features.pin).toBe(false);expect(cfg.features.textExtraction).toBe(true);expect(cfg.features.tableExtraction).toBe(true);expect(cfg.features.memeExplanation).toBe(false);
  localStorage.removeItem("snaptrans.config");
 });
 it("clears recent history but keeps explicit saves",async()=>{
  localStorage.setItem("snaptrans.history",JSON.stringify([{id:"recent"},{id:"saved",favorite:true},{id:"card",kind:"learning"}]));
  await clearHistory();expect((await getHistory()).map(e=>e.id)).toEqual(["saved","card"]);localStorage.removeItem("snaptrans.history");
 });
});
