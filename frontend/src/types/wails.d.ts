import type {
  AppConfig,
  CapturePayload,
  EnvironmentStatus,
  GenerationEvent,
  HistoryEntry,
  OCRResultEvent,
  TranslationDirection,
  TranslationDirectionEvent,
  TranslationTokenEvent,
  WorkflowErrorPayload
} from "../services/backend";

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          LoadConfig: () => Promise<AppConfig>;
          SaveConfig: (config: AppConfig) => Promise<void>;
          TriggerCapture: () => Promise<void>;
          ShowCaptureWindow: () => Promise<void>;
          ProcessImage: (base64Crop: string, direction: TranslationDirection, generation?: number) => Promise<void>;
          HideWindow: () => Promise<void>;
          QuitApp: () => Promise<void>;
          ShowSettings: () => Promise<void>;
          GetHistory: () => Promise<HistoryEntry[]>;
          ClearHistory: () => Promise<void>;
          TestConnection: () => Promise<void>;
          GetEnvironmentStatus: () => Promise<EnvironmentStatus>;
          SetAutoStart: (enabled: boolean) => Promise<void>;
          IsAutoStartEnabled: () => Promise<boolean>;
          GetWindowPosition: () => Promise<[number, number]>;
          SetWindowPosition: (x: number, y: number) => Promise<void>;
          GetVersion: () => Promise<string>;
          OpenLogFolder: () => Promise<void>;
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
  "ocr-start": GenerationEvent;
  "ocr-result": OCRResultEvent;
  "translation-direction": TranslationDirectionEvent;
  "translation-start": GenerationEvent;
  "translation-token": TranslationTokenEvent;
  "translation-done": GenerationEvent;
  "workflow-error": WorkflowErrorPayload;
  "settings-open": Record<string, never>;
}
