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
    copyImageDataUrl: vi.fn(async () => {}),
    hideWindow: vi.fn(async () => {}),
    isDesktop: false,
    loadConfig: vi.fn(async () => ({ ...defaultConfig })),
    processImage: vi.fn(async () => {}),
    showCaptureWindow: vi.fn(async () => {}),
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
  copyImageDataUrl: backendMocks.copyImageDataUrl,
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
  showCaptureWindow: backendMocks.showCaptureWindow,
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
    backendMocks.copyImageDataUrl.mockClear();
    backendMocks.isDesktop = false;
    backendMocks.processImage.mockClear();
    backendMocks.showCaptureWindow.mockClear();
    backendMocks.triggerCapture.mockClear();
    backendMocks.loadConfig.mockClear();
    vi.stubGlobal("Image", MockImage);
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      clearRect: vi.fn(),
      drawImage: vi.fn(),
      fillRect: vi.fn(),
      fillText: vi.fn(),
      getImageData: vi.fn(() => ({
        data: new Uint8ClampedArray([18, 24, 38, 255, 20, 26, 40, 255])
      })),
      measureText: vi.fn((text: string) => ({ width: text.length * 16 })),
      restore: vi.fn(),
      save: vi.fn()
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

  it("does not show a full-screen mask before the user starts drawing", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();

    expect(wrapper.find("section.cursor-crosshair").exists()).toBe(true);
    expect(wrapper.html()).not.toContain("bg-slate-950/20");
  });

  it("anchors the result panel to the selected region", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 120, clientY: 140 });
    await captureLayer.trigger("mousemove", { clientX: 380, clientY: 220 });
    await captureLayer.trigger("mouseup", { clientX: 380, clientY: 220 });
    await flushPromises();

    backendMocks.emit("translation-token", "\u6d4b\u8bd5");
    backendMocks.emit("translation-done", {});
    await flushPromises();

    const resultPanel = wrapper.find("section.z-20");
    expect(resultPanel.attributes("style")).toContain("left: 120px");
    expect(resultPanel.attributes("style")).toContain("top: 140px");
    expect(resultPanel.attributes("style")).toContain("width: 260px");
    expect(resultPanel.attributes("style")).toContain("min-height: 80px");
  });

  it("renders translated OCR blocks at their original positions", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 148, clientY: 26 });
    await captureLayer.trigger("mousemove", { clientX: 600, clientY: 78 });
    await captureLayer.trigger("mouseup", { clientX: 600, clientY: 78 });
    await flushPromises();

    backendMocks.emit("ocr-result", {
      blocks: [
        { text: "Neutral", x: 0.16, y: 0.2, width: 0.15, height: 0.36 },
        { text: "Negative", x: 0.39, y: 0.2, width: 0.17, height: 0.36 },
        { text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 }
      ]
    });
    backendMocks.emit("translation-token", "\u4e2d\u6027\n\u8d1f\u9762\n\u6b63\u9762");
    backendMocks.emit("translation-done", {});
    await flushPromises();

    const labels = wrapper.findAll("[data-testid='ocr-block']");
    const resultPanel = wrapper.find("section.z-20");
    expect(resultPanel.classes()).toContain("border-emerald-400");
    expect(labels).toHaveLength(3);
    expect(labels[0].text()).toBe("\u4e2d\u6027");
    expect(labels[1].text()).toBe("\u8d1f\u9762");
    expect(labels[2].text()).toBe("\u6b63\u9762");
    expect(labels[2].attributes("style")).toContain("left: 280px");
    expect(labels[2].attributes("style")).toContain("top: 10px");
    expect(labels[2].attributes("style")).toContain("width: 81px");
    expect(labels[2].attributes("style")).toContain("height: 19px");
    expect(labels[2].attributes("style")).toContain("font-size: 18px");
    expect(labels[2].attributes("style")).toContain("color: rgb(248, 250, 252)");
    expect(labels[2].attributes("style")).toContain("background-color: rgba(19, 25, 39");
    expect(wrapper.find("button[aria-label='Copy translated screenshot']").exists()).toBe(true);
  });

  it("copies a translated screenshot from the selected region", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 148, clientY: 26 });
    await captureLayer.trigger("mousemove", { clientX: 600, clientY: 78 });
    await captureLayer.trigger("mouseup", { clientX: 600, clientY: 78 });
    await flushPromises();

    backendMocks.emit("ocr-result", {
      blocks: [{ text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 }]
    });
    backendMocks.emit("translation-token", "\u6b63\u9762");
    backendMocks.emit("translation-done", {});
    await flushPromises();

    await wrapper.find("button[aria-label='Copy translated screenshot']").trigger("click");
    await flushPromises();

    expect(backendMocks.copyImageDataUrl).toHaveBeenCalledWith("data:image/png;base64,c2VsZWN0aW9u");
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
