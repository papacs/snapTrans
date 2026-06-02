import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.vue";

const backendMocks = vi.hoisted(() => {
  type Listener = (payload: unknown) => void;
  const listeners = new Map<string, Set<Listener>>();
  const defaultConfig = {
    shortcutKey: "Alt+Q",
    deepSeekAPIKey: "",
    deepSeekBaseURL: "https://api.deepseek.com",
    deepSeekModel: "deepseek-chat",
    rapidOCRPath: "./rapidocr_json.exe",
    rapidOCRTimeoutSeconds: 15
  };

  function emit(eventName: string, payload: unknown): void {
    for (const listener of listeners.get(eventName) ?? []) {
      listener(payload);
    }
  }

  return {
    defaultConfig,
    listeners,
    emit,
    hideWindow: vi.fn(async () => {}),
    loadConfig: vi.fn(async () => ({ ...defaultConfig })),
    triggerCapture: vi.fn(async () => {
      emit("capture-start", {
        image: "data:image/png;base64,ZmFrZQ==",
        width: 800,
        height: 600,
        originX: 0,
        originY: 0,
        source: "browser"
      });
    })
  };
});

vi.mock("./services/backend", () => ({
  copyText: vi.fn(async () => {}),
  defaultConfig: backendMocks.defaultConfig,
  hasWailsBackend: () => false,
  hideWindow: backendMocks.hideWindow,
  loadConfig: backendMocks.loadConfig,
  onBackendEvent: (eventName: string, callback: (payload: unknown) => void) => {
    const listeners = backendMocks.listeners.get(eventName) ?? new Set<(payload: unknown) => void>();
    listeners.add(callback);
    backendMocks.listeners.set(eventName, listeners);
    return () => listeners.delete(callback);
  },
  processImage: vi.fn(async () => {}),
  saveConfig: vi.fn(async () => {}),
  triggerCapture: backendMocks.triggerCapture
}));

class MockImage {
  naturalWidth = 800;
  naturalHeight = 600;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;

  set src(_value: string) {
    queueMicrotask(() => this.onload?.());
  }
}

describe("App capture cancellation", () => {
  beforeEach(() => {
    backendMocks.listeners.clear();
    backendMocks.hideWindow.mockClear();
    backendMocks.triggerCapture.mockClear();
    backendMocks.loadConfig.mockClear();
    vi.stubGlobal("Image", MockImage);
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      clearRect: vi.fn(),
      drawImage: vi.fn()
    } as unknown as CanvasRenderingContext2D);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("cancels capture selection with right click", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();

    expect(wrapper.find("section.cursor-crosshair").exists()).toBe(true);

    await wrapper.find("section.cursor-crosshair").trigger("contextmenu");
    await flushPromises();

    expect(wrapper.find("section.cursor-crosshair").exists()).toBe(false);
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });

  it("cancels capture selection with Escape", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();

    expect(wrapper.find("section.cursor-crosshair").exists()).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();

    expect(wrapper.find("section.cursor-crosshair").exists()).toBe(false);
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });
});
