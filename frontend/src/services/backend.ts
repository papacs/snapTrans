export interface AppConfig {
  shortcutKey: string;
  deepSeekAPIKey: string;
  deepSeekBaseURL: string;
  deepSeekModel: string;
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

type EventName =
  | "capture-start"
  | "ocr-start"
  | "translation-start"
  | "translation-token"
  | "translation-done"
  | "workflow-error"
  | "settings-open";

type Listener<T> = (payload: T) => void;

const fallbackListeners = new Map<EventName, Set<Listener<unknown>>>();

export const defaultConfig: AppConfig = {
  shortcutKey: "Alt+Q",
  deepSeekAPIKey: "",
  deepSeekBaseURL: "https://api.deepseek.com",
  deepSeekModel: "deepseek-chat",
  rapidOCRPath: "./rapidocr_json.exe",
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

export async function processImage(base64Crop: string): Promise<void> {
  if (hasWailsBackend()) {
    await window.go!.main!.App!.ProcessImage(base64Crop);
    return;
  }

  void streamFallbackTranslation();
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

async function streamFallbackTranslation(): Promise<void> {
  emitFallback("ocr-start", {});
  await delay(250);
  emitFallback("translation-start", {});

  const tokens = [
    "即时",
    "截图",
    "翻译",
    "已经",
    "进入",
    "流式",
    "输出",
    "。",
    "\n\n",
    "这里",
    "会",
    "显示",
    " DeepSeek ",
    "返回",
    "的",
    "翻译",
    "结果",
    "。"
  ];

  for (const token of tokens) {
    emitFallback("translation-token", token);
    await delay(90);
  }

  emitFallback("translation-done", {});
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
