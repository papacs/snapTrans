import type { AppConfig, CapturePayload, OCRResultPayload, WorkflowErrorPayload } from "../services/backend";

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          LoadConfig: () => Promise<AppConfig>;
          SaveConfig: (config: AppConfig) => Promise<void>;
          TriggerCapture: () => Promise<void>;
          ShowCaptureWindow: () => Promise<void>;
          ProcessImage: (base64Crop: string) => Promise<void>;
          HideWindow: () => Promise<void>;
          QuitApp: () => Promise<void>;
          ShowSettings: () => Promise<void>;
        };
      };
    };
    runtime?: {
      EventsOn: <T = unknown>(eventName: string, callback: (payload: T) => void) => (() => void) | void;
      EventsOff?: (eventName: string) => void;
      ClipboardSetText?: (text: string) => Promise<void>;
    };
  }
}

export interface BackendEvents {
  "capture-start": CapturePayload;
  "ocr-start": Record<string, never>;
  "ocr-result": OCRResultPayload;
  "translation-start": Record<string, never>;
  "translation-token": string;
  "translation-done": Record<string, never>;
  "workflow-error": WorkflowErrorPayload;
  "settings-open": Record<string, never>;
}
