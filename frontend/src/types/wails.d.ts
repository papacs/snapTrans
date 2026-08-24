import type {
  AppConfig,
  CapturePayload,
  EnvironmentStatus,
  GenerationEvent,
  HistoryEntry,
  ManualScrollStatus,
  OCRResultEvent,
  OCRResultPayload,
  ScrollCaptureRegion,
  ScrollCaptureStepResult,
  SelectionRegion,
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
          FrontendReady: () => Promise<void>;
          SaveConfig: (config: AppConfig) => Promise<void>;
          TriggerCapture: () => Promise<void>;
          TriggerScreenshot: () => Promise<void>;
          ShowCaptureWindow: () => Promise<void>;
          BeginScrollingScreenshot: (region: ScrollCaptureRegion) => Promise<ManualScrollStatus>;
          StepScrollingScreenshot: () => Promise<ScrollCaptureStepResult>;
          FinishScrollingScreenshot: () => Promise<CapturePayload>;
          CancelScrollingScreenshot: () => Promise<void>;
          ProcessImage: (base64Crop: string, direction: TranslationDirection, generation?: number) => Promise<void>;
          TranslateRegion: (region: SelectionRegion, direction: TranslationDirection, generation?: number) => Promise<void>;
          ExtractText: (base64Image: string) => Promise<OCRResultPayload>;
          HideWindow: () => Promise<void>;
          QuitApp: () => Promise<void>;
          ShowSettings: () => Promise<void>;
          ShowSettingsWindow: () => Promise<void>;
          GetHistory: () => Promise<HistoryEntry[]>;
          ClearHistory: () => Promise<void>;
          TestConnection: () => Promise<void>;
          GetEnvironmentStatus: () => Promise<EnvironmentStatus>;
          SetAutoStart: (enabled: boolean) => Promise<void>;
          IsAutoStartEnabled: () => Promise<boolean>;
          GetVersion: () => Promise<string>;
          OpenLogFolder: () => Promise<void>;
          SaveScreenshot: (dataUrl: string) => Promise<string>;
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
