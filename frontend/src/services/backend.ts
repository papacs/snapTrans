export interface AppConfig {
  uiLanguage: "zh-CN" | "en";
  shortcutKey: string;
  screenshotShortcutKey: string;
  apiKey: string;
  baseURL: string;
  model: string;
  rapidOCRPath: string;
  rapidOCRTimeoutSeconds: number;
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
}

export interface ScrollCaptureRegion {
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
  currentImage: string;
  previewImage: string;
  frames: number;
  width: number;
  height: number;
  appended: boolean;
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

export interface OCRBlockPayload {
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
  | "capture-start"
  | "ocr-start"
  | "ocr-result"
  | "translation-direction"
  | "translation-start"
  | "translation-token"
  | "translation-done"
  | "workflow-error"
  | "settings-open";

type Listener<T> = (payload: T) => void;

const fallbackListeners = new Map<EventName, Set<Listener<unknown>>>();

export const defaultConfig: AppConfig = {
  uiLanguage: "zh-CN",
  shortcutKey: "Alt+Q",
  screenshotShortcutKey: "Alt+W",
  apiKey: "",
  baseURL: "https://api.deepseek.com",
  model: "deepseek-chat",
  rapidOCRPath: "./RapidOCR-json_v0.2.0",
  rapidOCRTimeoutSeconds: 15,
  autoDirection: true,
  systemPrompt: "",
  glossary: "",
  persistentOCR: true,
  autoCopy: false
};

export function hasWailsBackend(): boolean {
  return Boolean(window.go?.main?.App);
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
    const loaded = await window.go!.main!.App!.LoadConfig();
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
    uiLanguage: loaded?.uiLanguage === "en" ? "en" : "zh-CN"
  };
}

export async function frontendReady(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.FrontendReady();
  }
}

export async function saveConfig(config: AppConfig): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.SaveConfig(config);
    return;
  }

  localStorage.setItem("snaptrans.config", JSON.stringify(config));
}

export async function triggerCapture(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.TriggerCapture();
    return;
  }

  emitFallback("capture-start", createFallbackCapture("translate"));
}

export async function triggerScreenshot(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.TriggerScreenshot();
    return;
  }

  emitFallback("capture-start", createFallbackCapture("screenshot"));
}

export async function showCaptureWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.ShowCaptureWindow();
  }
}

let fallbackScrollRegion: ScrollCaptureRegion | null = null;
let fallbackScrollFrames = 1;

export async function beginScrollingScreenshot(region: ScrollCaptureRegion): Promise<ManualScrollStatus> {
  if (hasWailsBackend()) {
    return window.go!.main!.App!.BeginScrollingScreenshot(region);
  }

  fallbackScrollRegion = region;
  fallbackScrollFrames = 1;
  return { frames: 1, width: region.width, height: region.height };
}

export async function stepScrollingScreenshot(): Promise<ScrollCaptureStepResult> {
  if (hasWailsBackend()) {
    return window.go!.main!.App!.StepScrollingScreenshot();
  }
  if (!fallbackScrollRegion) {
    throw new Error("no scrolling capture is active");
  }
  return {
    currentImage: "",
    previewImage: "",
    frames: fallbackScrollFrames,
    width: fallbackScrollRegion.width,
    height: fallbackScrollRegion.height,
    appended: false
  };
}

export async function finishScrollingScreenshot(): Promise<CapturePayload> {
  if (hasWailsBackend()) {
    return window.go!.main!.App!.FinishScrollingScreenshot();
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
    await window.go!.main!.App!.CancelScrollingScreenshot();
  }
  fallbackScrollRegion = null;
}

export async function showSettingsWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.ShowSettingsWindow();
  }
}

export async function processImage(
  base64Crop: string,
  direction: TranslationDirection = "to-zh",
  generation = 0
): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.ProcessImage(base64Crop, direction, generation);
    return;
  }

  void streamFallbackTranslation(direction, generation);
}

export async function extractText(base64Image: string): Promise<OCRResultPayload> {
  if (hasWailsBackend()) {
    const result = await window.go!.main!.App!.ExtractText(base64Image);
    return {
      text: typeof result?.text === "string" ? result.text : "",
      blocks: Array.isArray(result?.blocks) ? result.blocks : []
    };
  }

  await delay(220);
  return {
    text: "Browser preview: extracted screenshot text can be edited and copied here.",
    blocks: []
  };
}

export async function hideWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.HideWindow();
  }
}

export async function getHistory(): Promise<HistoryEntry[]> {
  if (hasWailsBackend()) {
    const entries = await window.go!.main!.App!.GetHistory();
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
    await window.go!.main!.App!.ClearHistory();
    return;
  }

  localStorage.removeItem("snaptrans.history");
}

export async function testConnection(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.TestConnection();
    return;
  }

  await delay(250);
}

export async function getEnvironmentStatus(): Promise<EnvironmentStatus> {
  if (hasWailsBackend()) {
    return window.go!.main!.App!.GetEnvironmentStatus();
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
    await window.go!.main!.App!.SetAutoStart(enabled);
  }
}

export async function isAutoStartEnabled(): Promise<boolean> {
  if (hasWailsBackend()) {
    return window.go!.main!.App!.IsAutoStartEnabled();
  }
  return false;
}

export async function getVersion(): Promise<string> {
  if (hasWailsBackend()) {
    return window.go!.main!.App!.GetVersion();
  }
  return "browser-preview";
}

export async function openLogFolder(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.OpenLogFolder();
  }
}

export async function saveScreenshot(dataUrl: string): Promise<string> {
  if (hasWailsBackend()) {
    return window.go!.main!.App!.SaveScreenshot(dataUrl);
  }

  const link = document.createElement("a");
  link.href = dataUrl;
  link.download = `snapTrans-${new Date().toISOString().replace(/[:.]/g, "-")}.png`;
  link.click();
  return link.download;
}

export async function copyText(text: string): Promise<void> {
  if (window.runtime?.ClipboardSetText) {
    await window.runtime.ClipboardSetText(text);
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

  await copyText(dataUrl);
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

  context.fillStyle = "rgba(255, 255, 255, 0.78)";
  const targetWidth = Math.min(520, canvas.width * 0.34);
  const targetHeight = Math.min(210, canvas.height * 0.22);
  const targetX = Math.max(48, canvas.width - targetWidth - 80);
  const targetY = Math.max(cardY + cardHeight + 64, canvas.height - targetHeight - 96);
  context.fillRect(targetX, targetY, targetWidth, targetHeight);
  context.fillStyle = "#0f766e";
  context.font = `${Math.max(24, Math.round(canvas.width * 0.024))}px Segoe UI, sans-serif`;
  context.fillText("OCR target area", targetX + 48, targetY + targetHeight / 2 + 12);

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
