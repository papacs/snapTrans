export type SettingsLocale = "zh-CN" | "en";

export interface SettingsMessages {
  title: string;
  subtitle: string;
  close: string;
  minimize: string;
  chinese: string;
  english: string;
  useLightTheme: string;
  useDarkTheme: string;
  aiService: string;
  aiServiceDescription: string;
  ocr: string;
  ready: string;
  notFound: string;
  apiKey: string;
  configured: string;
  missingAPIKey: string;
  apiKeyPlaceholder: string;
  baseURL: string;
  baseURLHelp: string;
  model: string;
  modelHelp: string;
  testConnection: string;
  testing: string;
  connectionSuccessful: string;
  connectionFailed: string;
  captureOCR: string;
  captureOCRDescription: string;
  captureShortcut: string;
  screenshotShortcut: string;
  screenshotShortcutHelp: string;
  record: string;
  recording: string;
  pressShortcut: string;
  shortcutHelp: string;
  rapidOCRPath: string;
  rapidOCRPathHelp: string;
  startWithWindows: string;
  startWithWindowsDescription: string;
  keepOCRReady: string;
  keepOCRReadyDescription: string;
  translation: string;
  translationDescription: string;
  detectDirection: string;
  detectDirectionDescription: string;
  copyAutomatically: string;
  copyAutomaticallyDescription: string;
  translationTimeout: string;
  translationTimeoutHelp: string;
  customInstructions: string;
  customInstructionsPlaceholder: string;
  glossary: string;
  glossaryPlaceholder: string;
  glossaryHelp: string;
  recentTranslations: string;
  recentTranslationsDescription: string;
  clear: string;
  clearHistory: string;
  noRecentTranslations: string;
  loadingHistory: string;
  copyHistoryEntry: string;
  openLogs: string;
  unsavedHint: string;
  cancel: string;
  saveChanges: string;
  saving: string;
}

export const defaultSettingsLocale: SettingsLocale = "zh-CN";

export function normalizeSettingsLocale(value: string | null | undefined): SettingsLocale {
  return value === "en" ? "en" : defaultSettingsLocale;
}

const english: SettingsMessages = {
  title: "Settings",
  subtitle: "Configure capture, OCR, and translation behavior",
  close: "Close settings",
  minimize: "Minimize to taskbar",
  chinese: "\u4e2d\u6587",
  english: "EN",
  useLightTheme: "Switch to light theme",
  useDarkTheme: "Switch to dark theme",
  aiService: "AI service",
  aiServiceDescription: "Connect DeepSeek, LiteLLM, or another OpenAI-compatible provider.",
  ocr: "OCR",
  ready: "ready",
  notFound: "not found",
  apiKey: "API key",
  configured: "configured",
  missingAPIKey: "missing - set it below",
  apiKeyPlaceholder: "Enter your API key",
  baseURL: "Base URL",
  baseURLHelp: "OpenAI-compatible endpoints usually end with /v1.",
  model: "Model",
  modelHelp: "Use the exact model ID exposed by your provider.",
  testConnection: "Test connection",
  testing: "Testing...",
  connectionSuccessful: "Connection successful",
  connectionFailed: "Connection failed",
  captureOCR: "Capture & OCR",
  captureOCRDescription: "Choose how captures start and how local OCR runs.",
  captureShortcut: "Translation shortcut",
  screenshotShortcut: "Screenshot shortcut",
  screenshotShortcutHelp: "Global shortcut for capture, annotation, copy, and save.",
  record: "Record",
  recording: "Listening...",
  pressShortcut: "Press a combination, or Esc to cancel.",
  shortcutHelp: "Global shortcut for instant capture and translation.",
  rapidOCRPath: "RapidOCR path",
  rapidOCRPathHelp: "Folder or full path to rapidocr_json.exe.",
  startWithWindows: "Start with Windows",
  startWithWindowsDescription: "Keep snapTrans available from sign-in.",
  keepOCRReady: "Keep OCR ready",
  keepOCRReadyDescription: "Faster first recognition with a small memory cost.",
  translation: "Translation",
  translationDescription: "Tune automatic behavior and terminology.",
  detectDirection: "Detect direction",
  detectDirectionDescription: "Choose Chinese or English from OCR text.",
  copyAutomatically: "Copy automatically",
  copyAutomaticallyDescription: "Copy each completed translation.",
  translationTimeout: "Translation timeout (s)",
  translationTimeoutHelp: "Cancel a stalled translation after this many seconds.",
  customInstructions: "Custom instructions",
  customInstructionsPlaceholder: "Optional tone, formatting, or domain instructions",
  glossary: "Glossary",
  glossaryPlaceholder: "API -> interface\ncommit -> submit",
  glossaryHelp: "One source -> target term per line.",
  recentTranslations: "Recent translations",
  recentTranslationsDescription: "Reuse a recent result without capturing again.",
  clear: "Clear",
  clearHistory: "Clear translation history",
  noRecentTranslations: "No recent translations yet.",
  loadingHistory: "Loading history...",
  copyHistoryEntry: "Copy this translation",
  openLogs: "Open logs",
  unsavedHint: "Changes apply after saving.",
  cancel: "Cancel",
  saveChanges: "Save changes",
  saving: "Saving..."
};

const chinese: SettingsMessages = {
  title: "\u8bbe\u7f6e",
  subtitle: "\u914d\u7f6e\u622a\u56fe\u3001OCR \u4e0e\u7ffb\u8bd1\u4f53\u9a8c",
  close: "\u5173\u95ed\u8bbe\u7f6e",
  minimize: "\u6700\u5c0f\u5316\u5230\u4efb\u52a1\u680f",
  chinese: "\u4e2d\u6587",
  english: "EN",
  useLightTheme: "\u5207\u6362\u4e3a\u6d45\u8272\u4e3b\u9898",
  useDarkTheme: "\u5207\u6362\u4e3a\u6697\u8272\u4e3b\u9898",
  aiService: "AI \u670d\u52a1",
  aiServiceDescription: "\u8fde\u63a5 DeepSeek\u3001LiteLLM \u6216\u5176\u4ed6 OpenAI \u517c\u5bb9\u670d\u52a1\u3002",
  ocr: "OCR",
  ready: "\u5df2\u5c31\u7eea",
  notFound: "\u672a\u627e\u5230",
  apiKey: "API \u5bc6\u94a5",
  configured: "\u5df2\u914d\u7f6e",
  missingAPIKey: "\u672a\u914d\u7f6e\uff0c\u8bf7\u5728\u4e0b\u65b9\u586b\u5199",
  apiKeyPlaceholder: "\u8bf7\u8f93\u5165 API \u5bc6\u94a5",
  baseURL: "\u63a5\u53e3\u5730\u5740",
  baseURLHelp: "OpenAI \u517c\u5bb9\u63a5\u53e3\u901a\u5e38\u4ee5 /v1 \u7ed3\u5c3e\u3002",
  model: "\u6a21\u578b",
  modelHelp: "\u586b\u5199\u670d\u52a1\u5546\u63d0\u4f9b\u7684\u5b8c\u6574\u6a21\u578b ID\u3002",
  testConnection: "\u6d4b\u8bd5\u8fde\u63a5",
  testing: "\u6d4b\u8bd5\u4e2d...",
  connectionSuccessful: "\u8fde\u63a5\u6210\u529f",
  connectionFailed: "\u8fde\u63a5\u5931\u8d25",
  captureOCR: "\u622a\u56fe\u4e0e OCR",
  captureOCRDescription: "\u8bbe\u7f6e\u622a\u56fe\u65b9\u5f0f\u548c\u672c\u5730\u6587\u5b57\u8bc6\u522b\u3002",
  captureShortcut: "\u622a\u56fe\u7ffb\u8bd1\u5feb\u6377\u952e",
  screenshotShortcut: "\u622a\u56fe\u6d82\u9e26\u5feb\u6377\u952e",
  screenshotShortcutHelp: "\u5168\u5c40\u5feb\u6377\u952e\uff0c\u9009\u533a\u540e\u53ef\u6d82\u9e26\u3001\u590d\u5236\u6216\u4fdd\u5b58\u56fe\u7247\u3002",
  record: "\u5f55\u5165",
  recording: "\u6b63\u5728\u5f55\u5165...",
  pressShortcut: "\u8bf7\u6309\u4e0b\u65b0\u7684\u5feb\u6377\u952e\u7ec4\u5408...",
  shortcutHelp: "\u5168\u5c40\u5feb\u6377\u952e\uff0c\u677e\u5f00\u9f20\u6807\u540e\u7acb\u5373 OCR \u5e76\u7ffb\u8bd1\u3002",
  rapidOCRPath: "RapidOCR \u8def\u5f84",
  rapidOCRPathHelp: "\u53ef\u586b\u5199\u6587\u4ef6\u5939\u6216 rapidocr_json.exe \u5b8c\u6574\u8def\u5f84\u3002",
  startWithWindows: "\u5f00\u673a\u81ea\u542f",
  startWithWindowsDescription: "\u767b\u5f55 Windows \u540e\u81ea\u52a8\u5728\u6258\u76d8\u8fd0\u884c\u3002",
  keepOCRReady: "\u4fdd\u6301 OCR \u9884\u70ed",
  keepOCRReadyDescription: "\u5360\u7528\u5c11\u91cf\u5185\u5b58\uff0c\u6362\u53d6\u66f4\u5feb\u7684\u9996\u6b21\u8bc6\u522b\u3002",
  translation: "\u7ffb\u8bd1",
  translationDescription: "\u8c03\u6574\u81ea\u52a8\u7ffb\u8bd1\u884c\u4e3a\u548c\u4e13\u7528\u672f\u8bed\u3002",
  detectDirection: "\u81ea\u52a8\u8bc6\u522b\u8bed\u8a00",
  detectDirectionDescription: "\u6839\u636e OCR \u5185\u5bb9\u81ea\u52a8\u9009\u62e9\u4e2d\u8bd1\u82f1\u6216\u82f1\u8bd1\u4e2d\u3002",
  copyAutomatically: "\u81ea\u52a8\u590d\u5236",
  copyAutomaticallyDescription: "\u7ffb\u8bd1\u5b8c\u6210\u540e\u81ea\u52a8\u590d\u5236\u5230\u526a\u8d34\u677f\u3002",
  translationTimeout: "\u7ffb\u8bd1\u8d85\u65f6\uff08\u79d2\uff09",
  translationTimeoutHelp: "\u7ffb\u8bd1\u957f\u65f6\u95f4\u65e0\u54cd\u5e94\u65f6\u81ea\u52a8\u53d6\u6d88\u3002",
  customInstructions: "\u81ea\u5b9a\u4e49\u6307\u4ee4",
  customInstructionsPlaceholder: "\u53ef\u9009\uff1a\u8bed\u6c14\u3001\u683c\u5f0f\u6216\u9886\u57df\u8981\u6c42",
  glossary: "\u672f\u8bed\u8868",
  glossaryPlaceholder: "API -> \u63a5\u53e3\ncommit -> \u63d0\u4ea4",
  glossaryHelp: "\u6bcf\u884c\u4e00\u7ec4\uff1a\u539f\u6587 -> \u8bd1\u6587\u3002",
  recentTranslations: "\u6700\u8fd1\u7ffb\u8bd1",
  recentTranslationsDescription: "\u67e5\u770b\u5e76\u590d\u7528\u8fd1\u671f\u7ed3\u679c\u3002",
  clear: "\u6e05\u7a7a",
  clearHistory: "\u6e05\u7a7a\u7ffb\u8bd1\u5386\u53f2",
  noRecentTranslations: "\u6682\u65e0\u7ffb\u8bd1\u8bb0\u5f55\u3002",
  loadingHistory: "\u6b63\u5728\u52a0\u8f7d\u5386\u53f2\u8bb0\u5f55...",
  copyHistoryEntry: "\u590d\u5236\u8fd9\u6761\u8bd1\u6587",
  openLogs: "\u6253\u5f00\u65e5\u5fd7",
  unsavedHint: "\u4fdd\u5b58\u540e\u5e94\u7528\u66f4\u6539\u3002",
  cancel: "\u53d6\u6d88",
  saveChanges: "\u4fdd\u5b58\u66f4\u6539",
  saving: "\u6b63\u5728\u4fdd\u5b58..."
};

export const settingsMessages: Record<SettingsLocale, SettingsMessages> = {
  "zh-CN": chinese,
  en: english
};
