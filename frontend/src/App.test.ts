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
    rapidOCRPath: "./RapidOCR-json_v0.2.0",
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
    isDesktop: false,
    loadConfig: vi.fn(async () => ({ ...defaultConfig })),
    processImage: vi.fn(async () => {}),
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
  hasWailsBackend: () => backendMocks.isDesktop,
  hideWindow: backendMocks.hideWindow,
  loadConfig: backendMocks.loadConfig,
  onBackendEvent: (eventName: string, callback: (payload: unknown) => void) => {
    const listeners = backendMocks.listeners.get(eventName) ?? new Set<(payload: unknown) => void>();
    listeners.add(callback);
    backendMocks.listeners.set(eventName, listeners);
    return () => listeners.delete(callback);
  },
  processImage: backendMocks.processImage,
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
    backendMocks.isDesktop = false;
    backendMocks.processImage.mockClear();
    backendMocks.triggerCapture.mockClear();
    backendMocks.loadConfig.mockClear();
    vi.stubGlobal("Image", MockImage);
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      clearRect: vi.fn(),
      drawImage: vi.fn()
    } as unknown as CanvasRenderingContext2D);
    vi.spyOn(HTMLCanvasElement.prototype, "toDataURL").mockReturnValue("data:image/png;base64,c2VsZWN0aW9u");
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 800,
      bottom: 600,
      width: 800,
      height: 600,
      toJSON: () => ({})
    } as DOMRect);
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

  it("hides the desktop settings window when closed from the title action", async () => {
    backendMocks.isDesktop = true;
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Settings']").trigger("click");
    await flushPromises();

    expect(wrapper.find("form").exists()).toBe(true);

    await wrapper.find("button[aria-label='Close']").trigger("click");
    await flushPromises();

    expect(wrapper.find("form").exists()).toBe(false);
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });

  it("shows a translating placeholder while waiting for the first streamed token", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    backendMocks.emit("translation-start", {});
    await flushPromises();

    expect(wrapper.text()).toContain("Translating...");
  });

  it("closes the result panel when clicking outside it", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    backendMocks.emit("translation-token", "测试");
    backendMocks.emit("translation-done", {});
    await flushPromises();

    expect(wrapper.text()).toContain("测试");

    await wrapper.find("main").trigger("mousedown", { button: 0 });
    await flushPromises();

    expect(wrapper.text()).not.toContain("测试");
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });

  it("closes the result panel with right click", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    backendMocks.emit("translation-token", "测试");
    backendMocks.emit("translation-done", {});
    await flushPromises();

    await wrapper.find("main").trigger("contextmenu");
    await flushPromises();

    expect(wrapper.text()).not.toContain("测试");
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });
});
