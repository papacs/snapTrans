import { normalizeFeatures, type FeatureFlags } from "../utils/features";
export interface AppConfig {
  features: FeatureFlags;
  uiLanguage: "zh-CN" | "en";
  theme: "light" | "dark";
  shortcutKey: string;
  screenshotShortcutKey: string;
  apiKey: string;
  baseURL: string;
  model: string;
  rapidOCRPath: string;
  rapidOCRTimeoutSeconds: number;
  translationTimeoutSeconds: number;
  autoDirection: boolean;
  systemPrompt: string;
  glossary: string;
  persistentOCR: boolean;
  autoCopy: boolean;
}

export interface CapturePayload {
  image: string;
  width: number;
  height: number;
  originX?: number;
  originY?: number;
  displays?: DisplayInfo[];
  source?: "wails" | "browser";
  mode?: "translate" | "screenshot";
  scrollFrames?: number;
  selectedText?: NativeTextSelection;
  notice?: string;
}

export interface ScrollCaptureRegion {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface SelectionRegion {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface ManualScrollStatus {
  frames: number;
  width: number;
  height: number;
  error?: string;
}

export interface ScrollCaptureStepResult {
  previewImage: string;
  frames: number;
  width: number;
  height: number;
  appended: boolean;
  limitReached?: boolean;
}

export interface DisplayInfo {
  x: number;
  y: number;
  width: number;
  height: number;
  scale: number;
}

export interface WorkflowErrorPayload {
  generation?: number;
  stage: "capture" | "ocr" | "translation" | "config" | "unknown";
  message: string;
}

export interface GenerationEvent {
  generation: number;
}

export interface TranslationTokenEvent extends GenerationEvent {
  token: string;
}

export interface TranslationDirectionEvent extends GenerationEvent {
  direction: TranslationDirection;
}

export interface OCRResultEvent extends GenerationEvent {
  text: string;
  blocks: OCRBlockPayload[];
}

// Selection bounds are normalized to the full captured frame (0..1).
// Child text blocks are normalized to that selection, not the screen.
export interface NativeTextSelection extends SelectionRegion { id: string; blocks: TextBlockPayload[] }
export interface TextRegionsEvent extends OCRResultEvent { source: "ocr" | "selection" }
export type OCRBlockPayload = TextBlockPayload;
export interface TextBlockPayload {
  background?: string;
  foreground?: string;
  text: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface OCRResultPayload {
  text: string;
  blocks: OCRBlockPayload[];
}

import type { TranslationDirection as ResolvedDirection } from "../utils/translation";

export type TranslationDirection = ResolvedDirection | "auto";

export interface HistoryEntry {
  favorite?: boolean;
  kind?: "learning";
  example?: string;
  id: string;
  timestamp: string;
  source: string;
  translation: string;
  direction: TranslationDirection;
}

export interface EnvironmentStatus {
  ocrReady: boolean;
  ocrDetail: string;
  apiKeyConfigured: boolean;
  shortcut: string;
  screenshotShortcut: string;
}

type EventName =
  | "text-action"
  | "capture-start"
  | "ocr-start"
  | "ocr-result"
  | "text-regions"
  | "translation-direction"
  | "translation-start"
  | "translation-token"
  | "translation-done"
  | "workflow-error"
  | "settings-open";

type Listener<T> = (payload: T) => void;

const fallbackListeners = new Map<EventName, Set<Listener<unknown>>>();

export const defaultConfig: AppConfig = {
  features: normalizeFeatures(),
  uiLanguage: "zh-CN",
  theme: "light",
  shortcutKey: "Alt+Q",
  screenshotShortcutKey: "Alt+W",
  apiKey: "",
  baseURL: "https://api.deepseek.com",
  model: "deepseek-v4-flash",
  rapidOCRPath: "./RapidOCR-json_v0.2.0",
  rapidOCRTimeoutSeconds: 15,
  translationTimeoutSeconds: 60,
  autoDirection: true,
  systemPrompt: "",
  glossary: "",
  persistentOCR: true,
  autoCopy: false
};

export function hasWailsBackend(): boolean {
  return Boolean(window.go?.desktop?.App);
}

export function onBackendEvent<T>(eventName: EventName, callback: Listener<T>): () => void {
  const off = window.runtime?.EventsOn?.(eventName, callback as Listener<unknown>);
  if (typeof off === "function") {
    return off;
  }

  const listeners = fallbackListeners.get(eventName) ?? new Set<Listener<unknown>>();
  listeners.add(callback as Listener<unknown>);
  fallbackListeners.set(eventName, listeners);

  return () => {
    listeners.delete(callback as Listener<unknown>);
  };
}

export async function loadConfig(): Promise<AppConfig> {
  if (hasWailsBackend()) {
    const loaded = await window.go!.desktop!.App!.LoadConfig();
    return normalizeLoadedConfig(loaded);
  }

  const saved = localStorage.getItem("snaptrans.config");
  if (!saved) {
    return { ...defaultConfig };
  }

  try {
    return normalizeLoadedConfig(JSON.parse(saved));
  } catch {
    return { ...defaultConfig };
  }
}

function normalizeLoadedConfig(loaded: Partial<AppConfig> | null | undefined): AppConfig {
  return {
    ...defaultConfig,
    ...loaded,
    uiLanguage: loaded?.uiLanguage === "en" ? "en" : "zh-CN",
    theme: loaded?.theme === "dark" ? "dark" : "light",
    features: normalizeFeatures(loaded?.features)
  };
}

export async function frontendReady(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.FrontendReady();
  }
}

export async function saveConfig(config: AppConfig, autoStart?: boolean): Promise<void> {
  if (hasWailsBackend()) {
    if (autoStart === undefined) {
      await window.go!.desktop!.App!.SaveConfig(config);
    } else {
      await window.go!.desktop!.App!.SaveSettings(config, autoStart);
    }
    return;
  }

  localStorage.setItem("snaptrans.config", JSON.stringify(config));
  if (autoStart !== undefined) localStorage.setItem("snaptrans.autostart", String(autoStart));
}

export async function triggerCapture(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.TriggerCapture();
    return;
  }

  emitFallback("capture-start", createFallbackCapture("translate"));
}

export async function triggerScreenshot(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.TriggerScreenshot();
    return;
  }

  emitFallback("capture-start", createFallbackCapture("screenshot"));
}

export async function showCaptureWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.ShowCaptureWindow();
  }
}

let fallbackScrollRegion: ScrollCaptureRegion | null = null;
let fallbackScrollFrames = 1;

export async function beginScrollingScreenshot(region: ScrollCaptureRegion): Promise<ManualScrollStatus> {
  if (hasWailsBackend()) {
    return window.go!.desktop!.App!.BeginScrollingScreenshot(region);
  }

  fallbackScrollRegion = region;
  fallbackScrollFrames = 1;
  return { frames: 1, width: region.width, height: region.height };
}

export async function stepScrollingScreenshot(): Promise<ScrollCaptureStepResult> {
  if (hasWailsBackend()) {
    return window.go!.desktop!.App!.StepScrollingScreenshot();
  }
  if (!fallbackScrollRegion) {
    throw new Error("no scrolling capture is active");
  }
  return {
    previewImage: "",
    frames: fallbackScrollFrames,
    width: fallbackScrollRegion.width,
    height: fallbackScrollRegion.height,
    appended: false
  };
}

export async function finishScrollingScreenshot(): Promise<CapturePayload> {
  if (hasWailsBackend()) {
    return window.go!.desktop!.App!.FinishScrollingScreenshot();
  }
  if (!fallbackScrollRegion) {
    throw new Error("no scrolling capture is active");
  }
  const result = createFallbackScrollingCapture(fallbackScrollRegion);
  fallbackScrollRegion = null;
  return result;
}

export async function cancelScrollingScreenshot(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.CancelScrollingScreenshot();
  }
  fallbackScrollRegion = null;
}

export async function showSettingsWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.ShowSettingsWindow();
  }
}

export async function processImage(
  base64Crop: string,
  direction: TranslationDirection = "to-zh",
  generation = 0
): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.ProcessImage(base64Crop, direction, generation);
    return;
  }

  void streamFallbackTranslation(direction, generation);
}

export async function translateRegion(
  region: SelectionRegion,
  direction: TranslationDirection = "to-zh",
  generation = 0
): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.TranslateRegion(region, direction, generation);
    return;
  }

  void streamFallbackTranslation(direction, generation);
}

export async function translateSelection(id: string, direction: TranslationDirection, generation: number): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.TranslateSelection(id, direction, generation);
    return;
  }
  throw new Error("Native text selection is only available in the desktop app.");
}

export async function extractText(base64Image: string): Promise<OCRResultPayload> {
  if (hasWailsBackend()) {
    const result = await window.go!.desktop!.App!.ExtractText(base64Image);
    return {
      text: typeof result?.text === "string" ? result.text : "",
      blocks: Array.isArray(result?.blocks) ? result.blocks : []
    };
  }

  await delay(220);
  return {
    text: "Feature This month Last month\nTranslation 128 104\nScreenshot 86 72\nSaved 41 35",
    blocks: [
      { text: "Feature", x: 0.06, y: 0.08, width: 0.22, height: 0.1 },
      { text: "This month", x: 0.43, y: 0.08, width: 0.2, height: 0.1 },
      { text: "Last month", x: 0.72, y: 0.08, width: 0.2, height: 0.1 },
      { text: "Translation", x: 0.06, y: 0.31, width: 0.24, height: 0.1 },
      { text: "128", x: 0.43, y: 0.31, width: 0.1, height: 0.1 },
      { text: "104", x: 0.72, y: 0.31, width: 0.1, height: 0.1 },
      { text: "Screenshot", x: 0.06, y: 0.54, width: 0.24, height: 0.1 },
      { text: "86", x: 0.43, y: 0.54, width: 0.08, height: 0.1 },
      { text: "72", x: 0.72, y: 0.54, width: 0.08, height: 0.1 },
      { text: "Saved", x: 0.06, y: 0.77, width: 0.16, height: 0.1 },
      { text: "41", x: 0.43, y: 0.77, width: 0.08, height: 0.1 },
      { text: "35", x: 0.72, y: 0.77, width: 0.08, height: 0.1 }
    ]
  };
}

// Keep the settings form mounted so taskbar restore retains unsaved changes.
// Browser previews have no native window to minimize.
export function minimizeWindow(): void {
  window.runtime?.WindowMinimise?.();
}

export async function hideWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.HideWindow();
  }
}

export async function getHistory(): Promise<HistoryEntry[]> {
  if (hasWailsBackend()) {
    const entries = await window.go!.desktop!.App!.GetHistory();
    return Array.isArray(entries) ? entries : [];
  }

  const saved = localStorage.getItem("snaptrans.history");
  if (!saved) {
    return [];
  }
  try {
    const entries: unknown = JSON.parse(saved);
    return Array.isArray(entries) ? entries : [];
  } catch {
    return [];
  }
}

export async function clearHistory(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.ClearHistory();
    return;
  }

  const saved = (await getHistory()).filter(entry => entry.favorite || entry.kind === "learning");
  localStorage.setItem("snaptrans.history", JSON.stringify(saved));
}

export async function testConnection(config: AppConfig): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.TestConnection(config);
    return;
  }

  await delay(250);
}

export async function getEnvironmentStatus(): Promise<EnvironmentStatus> {
  if (hasWailsBackend()) {
    return window.go!.desktop!.App!.GetEnvironmentStatus();
  }

  return {
    ocrReady: false,
    ocrDetail: "Browser preview does not run local OCR.",
    apiKeyConfigured: false,
    shortcut: defaultConfig.shortcutKey,
    screenshotShortcut: defaultConfig.screenshotShortcutKey
  };
}

export async function setAutoStart(enabled: boolean): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.SetAutoStart(enabled);
  }
}

export async function isAutoStartEnabled(): Promise<boolean> {
  if (hasWailsBackend()) {
    return window.go!.desktop!.App!.IsAutoStartEnabled();
  }
  return localStorage.getItem("snaptrans.autostart") === "true";
}

export async function getVersion(): Promise<string> {
  if (hasWailsBackend()) {
    return window.go!.desktop!.App!.GetVersion();
  }
  return "browser-preview";
}

export async function openLogFolder(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.desktop!.App!.OpenLogFolder();
  }
}

export async function saveScreenshot(dataUrl: string): Promise<string> {
  if (hasWailsBackend()) {
    return window.go!.desktop!.App!.SaveScreenshot(dataUrl);
  }

  const link = document.createElement("a");
  link.href = dataUrl;
  link.download = `snapTrans-${new Date().toISOString().replace(/[:.]/g, "-")}.png`;
  link.click();
  return link.download;
}

export async function copyText(text: string): Promise<void> {
  if (window.runtime?.ClipboardSetText) {
    const success = await window.runtime.ClipboardSetText(text);
    if (success === false) throw new Error("Clipboard is busy; please retry.");
    return;
  }

  await navigator.clipboard.writeText(text);
}

export async function copyImageDataUrl(dataUrl: string): Promise<void> {
  const blob = await (await fetch(dataUrl)).blob();
  if (navigator.clipboard?.write && typeof ClipboardItem !== "undefined") {
    await navigator.clipboard.write([new ClipboardItem({ [blob.type || "image/png"]: blob })]);
    return;
  }

  throw new Error("当前环境无法复制图片，请使用保存图片。 / Image clipboard unavailable; please save the image.");
}

function emitFallback<T>(eventName: EventName, payload: T): void {
  const listeners = fallbackListeners.get(eventName);
  if (!listeners) {
    return;
  }

  for (const listener of listeners) {
    listener(payload);
  }
}

function createFallbackCapture(mode: "translate" | "screenshot"): CapturePayload {
  const canvas = document.createElement("canvas");
  const scale = Math.max(1, Math.min(window.devicePixelRatio || 1, 2));
  canvas.width = Math.max(720, Math.round(window.innerWidth * scale));
  canvas.height = Math.max(520, Math.round(window.innerHeight * scale));

  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("Canvas 2D context is unavailable");
  }

  const gradient = context.createLinearGradient(0, 0, canvas.width, canvas.height);
  gradient.addColorStop(0, "#0f172a");
  gradient.addColorStop(0.45, "#2563eb");
  gradient.addColorStop(1, "#16a34a");
  context.fillStyle = gradient;
  context.fillRect(0, 0, canvas.width, canvas.height);

  const cardWidth = Math.min(760, canvas.width - 96);
  const cardHeight = Math.min(320, canvas.height * 0.42);
  const cardX = Math.max(48, canvas.width * 0.12);
  const cardY = Math.max(70, canvas.height * 0.14);

  context.fillStyle = "rgba(255, 255, 255, 0.92)";
  context.fillRect(cardX, cardY, cardWidth, cardHeight);
  context.fillStyle = "#111827";
  context.font = `${Math.max(30, Math.round(canvas.width * 0.03))}px Segoe UI, sans-serif`;
  context.fillText("Instant screenshot translation", cardX + 48, cardY + 100);
  context.font = `${Math.max(22, Math.round(canvas.width * 0.022))}px Segoe UI, sans-serif`;
  context.fillText("Release the mouse to stream the result in place.", cardX + 48, cardY + 170);

  context.fillStyle = "rgba(255, 255, 255, 0.94)";
  const targetWidth = Math.min(600, canvas.width * 0.4);
  const targetHeight = Math.min(240, canvas.height * 0.26);
  const targetX = Math.max(48, canvas.width - targetWidth - 80);
  const targetY = Math.max(cardY + cardHeight + 48, canvas.height - targetHeight - 72);
  context.fillRect(targetX, targetY, targetWidth, targetHeight);
  const demoRows = [
    ["Feature", "This month", "Last month"],
    ["Translation", "128", "104"],
    ["Screenshot", "86", "72"],
    ["Saved", "41", "35"]
  ];
  const demoColumns = [0, 0.44, 0.72, 1];
  const demoRowHeight = targetHeight / demoRows.length;
  context.strokeStyle = "#cbd5e1";
  context.lineWidth = 2;
  context.font = `${Math.max(14, Math.round(targetWidth * 0.03))}px Segoe UI, sans-serif`;
  demoRows.forEach((row, rowIndex) => {
    if (rowIndex === 0) {
      context.fillStyle = "#ecfdf5";
      context.fillRect(targetX, targetY, targetWidth, demoRowHeight);
    }
    row.forEach((cell, columnIndex) => {
      context.fillStyle = rowIndex === 0 ? "#047857" : "#334155";
      context.fillText(
        cell,
        targetX + demoColumns[columnIndex]! * targetWidth + 16,
        targetY + rowIndex * demoRowHeight + demoRowHeight * 0.66
      );
    });
  });
  for (let row = 0; row <= demoRows.length; row++) {
    context.beginPath();
    context.moveTo(targetX, targetY + row * demoRowHeight);
    context.lineTo(targetX + targetWidth, targetY + row * demoRowHeight);
    context.stroke();
  }
  for (const column of demoColumns) {
    context.beginPath();
    context.moveTo(targetX + column * targetWidth, targetY);
    context.lineTo(targetX + column * targetWidth, targetY + targetHeight);
    context.stroke();
  }

  return {
    image: canvas.toDataURL("image/png"),
    width: canvas.width,
    height: canvas.height,
    originX: 0,
    originY: 0,
    source: "browser",
    mode
  };
}

function createFallbackScrollingCapture(region: ScrollCaptureRegion, frames = 3): CapturePayload {
  const width = Math.max(32, Math.round(region.width));
  const viewportHeight = Math.max(32, Math.round(region.height));
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = viewportHeight * frames;
  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("Canvas 2D context is unavailable");
  }

  context.fillStyle = "#f8fafc";
  context.fillRect(0, 0, canvas.width, canvas.height);
  for (let section = 0; section < 6; section += 1) {
    const top = section * (canvas.height / 6);
    context.fillStyle = section % 2 === 0 ? "#e2e8f0" : "#d1fae5";
    context.fillRect(0, top, canvas.width, canvas.height / 6);
    context.fillStyle = "#0f172a";
    context.font = `${Math.max(14, Math.round(width * 0.035))}px Segoe UI, sans-serif`;
    context.fillText(`Scrolling screenshot section ${section + 1}`, 20, top + 42);
  }

  return {
    image: canvas.toDataURL("image/png"),
    width: canvas.width,
    height: canvas.height,
    originX: region.x,
    originY: region.y,
    source: "browser",
    mode: "screenshot",
    scrollFrames: frames
  };
}

async function streamFallbackTranslation(direction: TranslationDirection, generation: number): Promise<void> {
  emitFallback("ocr-start", { generation });
  await delay(80);
  const resolvedDirection: "to-zh" | "to-en" = direction === "to-en" ? "to-en" : "to-zh";
  emitFallback("translation-direction", { generation, direction: resolvedDirection });
  emitFallback("ocr-result", {
    generation,
    text: "Neutral\nNegative\nPositive",
    blocks: [
      { text: "Neutral", x: 0.16, y: 0.2, width: 0.15, height: 0.36 },
      { text: "Negative", x: 0.39, y: 0.2, width: 0.17, height: 0.36 },
      { text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 }
    ]
  } satisfies OCRResultEvent);
  emitFallback("translation-start", { generation });

  const tokens =
    direction === "to-en"
      ? ["Neutral", "\n", "Negative", "\n", "Positive"]
      : ["\u4e2d\u6027", "\n", "\u8d1f\u9762", "\n", "\u6b63\u9762"];

  for (const token of tokens) {
    emitFallback("translation-token", { generation, token });
    await delay(35);
  }

  emitFallback("translation-done", { generation });
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}


export type TextAction = "explain" | "summarize" | "meme" | "learning";
export interface TextActionRequest { id: string; text: string; action: TextAction }
export interface TextActionEvent { id: string; token?: string; error?: string; done: boolean }
const fallbackActionTimers = new Map<string, number>();
export async function startTextAction(request: TextActionRequest): Promise<void> {
  if (hasWailsBackend()) { await window.go!.desktop!.App!.StartTextAction(request); return; }
  for (const id of fallbackActionTimers.keys()) await cancelTextAction(id);
  const timer = window.setTimeout(() => {
    fallbackActionTimers.delete(request.id);
    emitFallback("text-action", {id:request.id, token:"浏览器演示 / Browser preview — 桌面版会使用你配置的模型处理这段文字。此处未调用 API。", done:true} satisfies TextActionEvent);
  }, 250);
  fallbackActionTimers.set(request.id, timer);
}
export async function cancelTextAction(id: string): Promise<void> {
  if (hasWailsBackend()) { await window.go!.desktop!.App!.CancelTextAction(id); return; }
  window.clearTimeout(fallbackActionTimers.get(id)); fallbackActionTimers.delete(id);
}
export async function setHistoryFavorite(id: string, favorite: boolean): Promise<void> {
  if (hasWailsBackend()) {await window.go!.desktop!.App!.SetHistoryFavorite(id,favorite); return;}
  const entries = await getHistory(); const entry = entries.find(e=>e.id===id);
  if (!entry) throw new Error("History entry no longer exists");
  entry.favorite=favorite; localStorage.setItem("snaptrans.history",JSON.stringify(entries));
}
export async function saveLearningCard(source: string, meaning: string, example: string): Promise<void> {
  if (hasWailsBackend()) {await window.go!.desktop!.App!.SaveLearningCard(source,meaning,example); return;}
  if (!source.trim() || !meaning.trim()) throw new Error("Original and meaning are required");
  const entries=await getHistory();
  if(entries.some(e=>e.kind==="learning" && e.source===source && e.translation===meaning && e.example===example)) return;
  entries.unshift({id:crypto.randomUUID(),timestamp:new Date().toISOString(),source,translation:meaning,example,kind:"learning",favorite:true,direction:"auto"});
  localStorage.setItem("snaptrans.history",JSON.stringify(entries));
}
export async function deleteSavedEntry(id: string): Promise<void> {
  if(hasWailsBackend()){await window.go!.desktop!.App!.DeleteSavedEntry(id);return;}
  localStorage.setItem("snaptrans.history",JSON.stringify((await getHistory()).filter(e=>e.id!==id)));
}
export async function pinImage(image: string, x=80, y=80): Promise<void> {
  if(hasWailsBackend()){await window.go!.desktop!.App!.PinImage({image,x:Math.round(x),y:Math.round(y)});return;}
  throw new Error("桌面贴钉仅在 Windows 应用中可用。 / Pins require the Windows desktop app.");
}
export async function exportMarkdown(text: string): Promise<string> {
  if(hasWailsBackend()) return window.go!.desktop!.App!.ExportMarkdown(text);
  const url=URL.createObjectURL(new Blob([text],{type:"text/markdown;charset=utf-8"}));
  const link=document.createElement("a");link.href=url;link.download="snapTrans-cards.md";link.click();
  window.setTimeout(()=>URL.revokeObjectURL(url),1000);return link.download;
}
