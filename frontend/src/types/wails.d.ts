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
  TextRegionsEvent,
  WorkflowErrorPayload
} from "../services/backend";

declare global {
  interface Window {
    go?: {
      desktop?: {
        App?: {
          StartTextAction: (request: import("../services/backend").TextActionRequest) => Promise<void>;
          CancelTextAction: (id: string) => Promise<void>;
          SetHistoryFavorite: (id: string, favorite: boolean) => Promise<void>;
          SaveLearningCard: (source: string, meaning: string, example: string) => Promise<void>;
          DeleteSavedEntry: (id: string) => Promise<void>;
          PinImage: (request: {image: string; x: number; y: number}) => Promise<void>;
          ExportMarkdown: (text: string) => Promise<string>;
          LoadConfig: () => Promise<AppConfig>;
          FrontendReady: () => Promise<void>;
          SaveConfig: (config: AppConfig) => Promise<void>;
          SaveSettings: (config: AppConfig, autoStart: boolean) => Promise<void>;
          TriggerTranslation: () => Promise<void>;
          TranslateSelection: (id: string, direction: TranslationDirection, generation: number) => Promise<void>;
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
          TestConnection: (config: AppConfig) => Promise<void>;
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
      WindowMinimise?: () => void;
      ClipboardSetText?: (text: string) => Promise<void | boolean>;
    };
  }
}

export interface BackendEvents {
  "capture-start": CapturePayload;
  "ocr-start": GenerationEvent;
  "ocr-result": OCRResultEvent;
  "text-regions": TextRegionsEvent;
  "translation-direction": TranslationDirectionEvent;
  "translation-start": GenerationEvent;
  "translation-token": TranslationTokenEvent;
  "translation-done": GenerationEvent;
  "workflow-error": WorkflowErrorPayload;
  "settings-open": Record<string, never>;
}
