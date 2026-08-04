export interface AppConfig {
  shortcutKey: string;
  apiKey: string;
  baseURL: string;
  model: string;
  rapidOCRPath: string;
  rapidOCRTimeoutSeconds: number;
}

export interface CapturePayload {
  image: string;
  width: number;
  height: number;
  originX?: number;
  originY?: number;
  source?: "wails" | "browser";
}

export interface WorkflowErrorPayload {
  stage: "capture" | "ocr" | "translation" | "config" | "unknown";
  message: string;
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

export type TranslationDirection = "to-zh" | "to-en";

type EventName =
  | "capture-start"
  | "ocr-start"
  | "ocr-result"
  | "translation-start"
  | "translation-token"
  | "translation-done"
  | "workflow-error"
  | "settings-open";

type Listener<T> = (payload: T) => void;

const fallbackListeners = new Map<EventName, Set<Listener<unknown>>>();

export const defaultConfig: AppConfig = {
  shortcutKey: "Alt+Q",
  apiKey: "",
  baseURL: "https://api.deepseek.com",
  model: "deepseek-chat",
  rapidOCRPath: "./RapidOCR-json_v0.2.0",
  rapidOCRTimeoutSeconds: 15
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
    return window.go!.main!.App!.LoadConfig();
  }

  const saved = localStorage.getItem("snaptrans.config");
  if (!saved) {
    return { ...defaultConfig };
  }

  try {
    return { ...defaultConfig, ...JSON.parse(saved) };
  } catch {
    return { ...defaultConfig };
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

  emitFallback("capture-start", createFallbackCapture());
}

export async function showCaptureWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.ShowCaptureWindow();
  }
}

export async function processImage(base64Crop: string, direction: TranslationDirection = "to-zh"): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.ProcessImage(base64Crop, direction);
    return;
  }

  void streamFallbackTranslation(direction);
}

export async function hideWindow(): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.HideWindow();
  }
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

function createFallbackCapture(): CapturePayload {
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
    source: "browser"
  };
}

async function streamFallbackTranslation(direction: TranslationDirection): Promise<void> {
  emitFallback("ocr-start", {});
  await delay(80);
  emitFallback("ocr-result", {
    text: "Neutral\nNegative\nPositive",
    blocks: [
      { text: "Neutral", x: 0.16, y: 0.2, width: 0.15, height: 0.36 },
      { text: "Negative", x: 0.39, y: 0.2, width: 0.17, height: 0.36 },
      { text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 }
    ]
  } satisfies OCRResultPayload);
  emitFallback("translation-start", {});

  const tokens =
    direction === "to-en"
      ? ["Neutral", "\n", "Negative", "\n", "Positive"]
      : ["\u4e2d\u6027", "\n", "\u8d1f\u9762", "\n", "\u6b63\u9762"];

  for (const token of tokens) {
    emitFallback("translation-token", token);
    await delay(35);
  }

  emitFallback("translation-done", {});
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
