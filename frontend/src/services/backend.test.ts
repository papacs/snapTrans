import { afterEach, describe, expect, it, vi } from "vitest";
import { copyImageDataUrl, copyText, defaultConfig, saveConfig, testConnection, loadConfig, clearHistory, getHistory, getCodexStatus, loginCodex, translateRegion } from "./backend";

afterEach(() => { delete window.go; delete window.runtime; vi.restoreAllMocks(); vi.unstubAllGlobals(); });
describe("native boundary", () => {
 it("sends an OCR retry as a request override without changing saved settings", async () => {
  const translate = vi.fn(async () => {});
  window.go = {desktop:{App:{TranslateRegion:translate}}} as unknown as Window["go"];
  const region = {x:80,y:60,width:320,height:100};
  await translateRegion(region,"to-zh",7,"ocr");
  expect(translate).toHaveBeenCalledWith({...region,inputMode:"ocr"},"to-zh",7);
  expect(region).not.toHaveProperty("inputMode");
 });
 it("requires the desktop for real connection tests and subscription auth", async () => {
  await expect(testConnection({...defaultConfig,provider:"codex"})).rejects.toThrow("desktop app");
  await expect(getCodexStatus("")).rejects.toThrow("desktop app");
  await expect(loginCodex("")).rejects.toThrow("desktop app");
 });
 it("sends the draft and autostart together to the desktop", async () => {
  const save = vi.fn(async () => {});
  const test = vi.fn(async () => {});
  window.go = {desktop:{App:{SaveSettings:save, TestConnection:test}}} as unknown as Window["go"];
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
  expect(cfg.provider).toBe("openai-compatible");expect(cfg.codexModel).toBe("");
  expect(cfg.directImageInput).toBe(false);
  expect(cfg.fastTranslation).toBe(true);
  expect(cfg.recordHistory).toBe(true);
  localStorage.removeItem("snaptrans.config");
 });
 it("enables screenshot upload only for an explicit boolean setting",async()=>{
  for (const value of [true,false,"false","true",null]) {
   localStorage.setItem("snaptrans.config",JSON.stringify({directImageInput:value}));
   expect((await loadConfig()).directImageInput).toBe(value === true);
  }
  localStorage.removeItem("snaptrans.config");
 });
 it("preserves disabled history recording across browser saves",async()=>{
  const previous = localStorage.getItem("snaptrans.history");
  localStorage.setItem("snaptrans.history",JSON.stringify([{id:"existing"}]));
  await saveConfig({...defaultConfig,recordHistory:false});
  expect((await loadConfig()).recordHistory).toBe(false);
  expect((await getHistory()).map(e=>e.id)).toEqual(["existing"]);
  localStorage.removeItem("snaptrans.config");
  if(previous === null)localStorage.removeItem("snaptrans.history");else localStorage.setItem("snaptrans.history",previous);
 });
 it("clears recent history but keeps explicit saves",async()=>{
  localStorage.setItem("snaptrans.history",JSON.stringify([{id:"recent"},{id:"saved",favorite:true},{id:"card",kind:"learning"}]));
  await clearHistory();expect((await getHistory()).map(e=>e.id)).toEqual(["saved","card"]);localStorage.removeItem("snaptrans.history");
 });
});
