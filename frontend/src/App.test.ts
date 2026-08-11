import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.vue";

const backendMocks = vi.hoisted(() => {
  type Listener = (payload: unknown) => void;
  const listeners = new Map<string, Set<Listener>>();
  const defaultConfig = {
    uiLanguage: "zh-CN" as const,
    shortcutKey: "Alt+Q",
    screenshotShortcutKey: "Alt+W",
    apiKey: "",
    baseURL: "https://api.deepseek.com",
    model: "deepseek-chat",
    rapidOCRPath: "./RapidOCR-json_v0.2.0",
    rapidOCRTimeoutSeconds: 15,
    autoDirection: false,
    persistentOCR: true,
    autoCopy: false,
    systemPrompt: "",
    glossary: ""
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
    beginScrollingScreenshot: vi.fn(async () => ({
      frames: 1,
      width: 440,
      height: 290
    })),
    stepScrollingScreenshot: vi.fn(async () => ({
      currentImage: "data:image/png;base64,Y3VycmVudA==",
      previewImage: "data:image/png;base64,cHJldmlldw==",
      frames: 2,
      width: 440,
      height: 580,
      appended: true
    })),
    finishScrollingScreenshot: vi.fn(async () => ({
      image: "data:image/png;base64,bG9uZw==",
      width: 440,
      height: 870,
      originX: 80,
      originY: 70,
      source: "browser" as const,
      mode: "screenshot" as const,
      scrollFrames: 3
    })),
    cancelScrollingScreenshot: vi.fn(async () => {}),
    copyText: vi.fn(async () => {}),
    extractText: vi.fn(async () => ({
      text: "\u622a\u56fe\u91cc\u7684\u6587\u5b57\u53ef\u4ee5\u7f16\u8f91\u548c\u590d\u5236\u3002",
      blocks: []
    })),
    frontendReady: vi.fn(async () => {}),
    getHistory: vi.fn(async () => []),
    hideWindow: vi.fn(async () => {}),
    isDesktop: false,
    loadConfig: vi.fn(async () => ({ ...defaultConfig })),
    processImage: vi.fn(async (_image: string, _direction: string, _generation: number) => {}),
    saveScreenshot: vi.fn(async () => "capture.png"),
    showCaptureWindow: vi.fn(async () => {}),
    showSettingsWindow: vi.fn(async () => {}),
    triggerCapture: vi.fn(async () => {
      emit("capture-start", {
        image: "data:image/png;base64,ZmFrZQ==",
        width: 800,
        height: 600,
        originX: 0,
        originY: 0,
        source: "browser"
      });
    }),
    triggerScreenshot: vi.fn(async () => {
      emit("capture-start", {
        image: "data:image/png;base64,ZmFrZQ==",
        width: 800,
        height: 600,
        originX: 0,
        originY: 0,
        source: "browser",
        mode: "screenshot"
      });
    })
  };
});

vi.mock("./services/backend", () => ({
  beginScrollingScreenshot: backendMocks.beginScrollingScreenshot,
  cancelScrollingScreenshot: backendMocks.cancelScrollingScreenshot,
  copyImageDataUrl: backendMocks.copyImageDataUrl,
  copyText: backendMocks.copyText,
  extractText: backendMocks.extractText,
  defaultConfig: backendMocks.defaultConfig,
  frontendReady: backendMocks.frontendReady,
  finishScrollingScreenshot: backendMocks.finishScrollingScreenshot,
  getHistory: backendMocks.getHistory,
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
  saveScreenshot: backendMocks.saveScreenshot,
  saveConfig: vi.fn(async () => {}),
  showCaptureWindow: backendMocks.showCaptureWindow,
  showSettingsWindow: backendMocks.showSettingsWindow,
  stepScrollingScreenshot: backendMocks.stepScrollingScreenshot,
  triggerCapture: backendMocks.triggerCapture,
  triggerScreenshot: backendMocks.triggerScreenshot
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

class ControlledImage {
  naturalWidth = 800;
  naturalHeight = 600;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  source = "";

  set src(value: string) {
    this.source = value;
    controlledImages.push(this);
  }

  resolve(): void {
    this.onload?.();
  }
}

class MockFileReader {
	result: string | ArrayBuffer | null = null;
	error: DOMException | null = null;
	onload: (() => void) | null = null;
	onerror: (() => void) | null = null;

	readAsDataURL(_blob: Blob): void {
		this.result = "data:image/png;base64,c2VsZWN0aW9u";
		queueMicrotask(() => this.onload?.());
	}
}

const controlledImages: ControlledImage[] = [];
let drawImageMock: ReturnType<typeof vi.fn>;

function stylePx(style: string | undefined, property: string): number {
  const match = new RegExp(`${property}:\\s*(-?\\d+(?:\\.\\d+)?)px`).exec(style ?? "");
  return match ? Number.parseFloat(match[1]) : Number.NaN;
}

describe("App capture cancellation", () => {
  beforeEach(() => {
    backendMocks.listeners.clear();
    backendMocks.hideWindow.mockClear();
    backendMocks.copyImageDataUrl.mockClear();
    backendMocks.beginScrollingScreenshot.mockClear();
    backendMocks.cancelScrollingScreenshot.mockClear();
    backendMocks.finishScrollingScreenshot.mockClear();
    backendMocks.stepScrollingScreenshot.mockClear();
    backendMocks.copyText.mockClear();
    backendMocks.extractText.mockClear();
    backendMocks.frontendReady.mockClear();
    backendMocks.getHistory.mockClear();
    backendMocks.isDesktop = false;
    backendMocks.processImage.mockClear();
    backendMocks.saveScreenshot.mockClear();
    backendMocks.showCaptureWindow.mockClear();
    backendMocks.showSettingsWindow.mockClear();
    backendMocks.triggerCapture.mockClear();
    backendMocks.loadConfig.mockClear();
    controlledImages.length = 0;
    drawImageMock = vi.fn();
    vi.stubGlobal("Image", MockImage);
		vi.stubGlobal("FileReader", MockFileReader);
    const getImageData = vi.fn((x: number, _y: number, width: number) => {
      const textColor =
        x > 150 && width > 500
          ? [255, 83, 22, 255, 250, 91, 30, 255]
          : [248, 250, 252, 255, 242, 245, 250, 255];

      return {
        data: new Uint8ClampedArray([
          18, 24, 38, 255,
          20, 26, 40, 255,
          19, 25, 39, 255,
          18, 24, 38, 255,
          19, 25, 39, 255,
          ...textColor
        ])
      };
    });
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      clearRect: vi.fn(),
      drawImage: drawImageMock,
      fillRect: vi.fn(),
      fillText: vi.fn(),
      beginPath: vi.fn(),
      ellipse: vi.fn(),
      getImageData,
      lineTo: vi.fn(),
      measureText: vi.fn((text: string) => ({ width: text.length * 16 })),
      moveTo: vi.fn(),
      restore: vi.fn(),
      save: vi.fn(),
      stroke: vi.fn(),
      strokeRect: vi.fn()
    } as unknown as CanvasRenderingContext2D);
		vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((callback) => {
			callback(new Blob(["selection"], { type: "image/png" }));
		});
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
    vi.useRealTimers();
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

  it("supports Ctrl+Z in the screenshot annotation toolbar and copies the image", async () => {
    const wrapper = mount(App);
    await flushPromises();

    backendMocks.emit("capture-start", {
      image: "data:image/png;base64,ZmFrZQ==",
      width: 800,
      height: 600,
      originX: 0,
      originY: 0,
      source: "browser",
      mode: "screenshot"
    });
    await flushPromises();

    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { button: 0, clientX: 80, clientY: 70 });
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: 520, clientY: 360 }));
    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 520, clientY: 360 }));
    await flushPromises();

    expect(wrapper.find("[data-testid='screenshot-toolbar']").exists()).toBe(true);
    expect(backendMocks.processImage).not.toHaveBeenCalled();

    await wrapper.find("[data-testid='annotation-rectangle']").trigger("click");
    const editor = wrapper.find("[data-testid='screenshot-editor'] canvas");
    await editor.trigger("mousedown", { button: 0, clientX: 120, clientY: 120 });
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: 260, clientY: 220 }));
    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 260, clientY: 220 }));
    await flushPromises();
    const undoButton = wrapper.find("[data-testid='annotation-undo']");
    expect((undoButton.element as HTMLButtonElement).disabled).toBe(false);

    const undoEvent = new KeyboardEvent("keydown", {
      key: "z",
      ctrlKey: true,
      bubbles: true,
      cancelable: true
    });
    window.dispatchEvent(undoEvent);
    await flushPromises();

    expect(undoEvent.defaultPrevented).toBe(true);
    expect((undoButton.element as HTMLButtonElement).disabled).toBe(true);

    await wrapper.find("[data-testid='screenshot-complete']").trigger("click");
    await flushPromises();

    expect(backendMocks.copyImageDataUrl).toHaveBeenCalledWith("data:image/png;base64,c2VsZWN0aW9u");
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });

  it("extracts screenshot text into an editable side panel and copies the edited text", async () => {
    let finishExtraction!: () => void;
    backendMocks.extractText.mockImplementationOnce(() => new Promise((resolve) => {
      finishExtraction = () => resolve({ text: "\u622a\u56fe\u91cc\u7684\u6587\u5b57\u53ef\u4ee5\u7f16\u8f91\u548c\u590d\u5236\u3002", blocks: [] });
    }));

    const wrapper = mount(App);
    await flushPromises();

    backendMocks.emit("capture-start", {
      image: "data:image/png;base64,ZmFrZQ==",
      width: 800,
      height: 600,
      originX: 0,
      originY: 0,
      source: "browser",
      mode: "screenshot"
    });
    await flushPromises();

    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { button: 0, clientX: 80, clientY: 70 });
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: 520, clientY: 360 }));
    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 520, clientY: 360 }));
    await flushPromises();

    await wrapper.find("[data-testid='screenshot-extract-text']").trigger("click");
    await flushPromises();
    const panel = wrapper.find("[data-testid='text-extraction-panel']");
    const textarea = panel.find("textarea");
    expect(panel.exists()).toBe(true);
    expect(panel.find("[data-testid='text-extraction-loading']").exists()).toBe(true);
    expect((textarea.element as HTMLTextAreaElement).disabled).toBe(true);

    finishExtraction();
    await flushPromises();
    expect(panel.find("[data-testid='text-extraction-loading']").exists()).toBe(false);
    expect((textarea.element as HTMLTextAreaElement).disabled).toBe(false);

    expect(backendMocks.extractText).toHaveBeenCalledWith("data:image/png;base64,c2VsZWN0aW9u");
    expect(backendMocks.processImage).not.toHaveBeenCalled();
    expect((textarea.element as HTMLTextAreaElement).value).toBe("\u622a\u56fe\u91cc\u7684\u6587\u5b57\u53ef\u4ee5\u7f16\u8f91\u548c\u590d\u5236\u3002");

    await textarea.setValue("\u7f16\u8f91\u540e\u7684\u8bc6\u522b\u6587\u5b57");
    await panel.find("[data-testid='copy-extracted-text']").trigger("click");
    await flushPromises();

    expect(backendMocks.copyText).toHaveBeenCalledWith("\u7f16\u8f91\u540e\u7684\u8bc6\u522b\u6587\u5b57");
  });

  it("keeps the native capture visible beneath the screenshot annotation layer", async () => {
    const wrapper = mount(App);
    await flushPromises();

    backendMocks.emit("capture-start", {
      image: "data:image/png;base64,ZmFrZQ==",
      width: 800,
      height: 600,
      originX: 0,
      originY: 0,
      source: "browser",
      mode: "screenshot"
    });
    await flushPromises();

    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { button: 0, clientX: 80, clientY: 70 });
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: 520, clientY: 360 }));
    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 520, clientY: 360 }));
    await flushPromises();

    const viewport = wrapper.find("[data-testid='screenshot-viewport']");
    const annotationCanvas = viewport.find("canvas");
    expect(viewport.classes()).not.toContain("bg-white");
    expect(annotationCanvas.classes()).toContain("bg-transparent");
    expect(drawImageMock).toHaveBeenCalledTimes(4);
  });

  it("keeps a live full preview while observing user-driven scrolling", async () => {
    vi.useFakeTimers();
    const wrapper = mount(App);
    await flushPromises();

    backendMocks.emit("capture-start", {
      image: "data:image/png;base64,ZmFrZQ==",
      width: 800,
      height: 600,
      originX: 100,
      originY: 200,
      source: "browser",
      mode: "screenshot"
    });
    await flushPromises();

    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { button: 0, clientX: 80, clientY: 70 });
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: 520, clientY: 360 }));
    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 520, clientY: 360 }));
    await flushPromises();

    await wrapper.find("[data-testid='screenshot-scroll-down']").trigger("click");
    await flushPromises();

    expect(backendMocks.beginScrollingScreenshot).toHaveBeenCalledWith({
      x: 180,
      y: 270,
      width: 440,
      height: 290
    });
    expect(wrapper.find("[data-testid='manual-scroll-controller']").exists()).toBe(true);
    expect(wrapper.find("[data-testid='manual-scroll-current']").exists()).toBe(true);
    expect(wrapper.find("[data-testid='manual-scroll-preview']").exists()).toBe(true);

    const showCallsBeforePoll = backendMocks.showCaptureWindow.mock.calls.length;
    await vi.advanceTimersByTimeAsync(120);
    await flushPromises();

    expect(backendMocks.stepScrollingScreenshot).toHaveBeenCalledWith();
    expect(backendMocks.showCaptureWindow).toHaveBeenCalledTimes(showCallsBeforePoll);
    expect(wrapper.find("[data-testid='manual-scroll-preview']").text()).toContain("2 帧");
    expect(wrapper.find("img[alt='当前滚动画面']").attributes("src")).toContain("Y3VycmVudA==");
    expect(wrapper.find("img[alt='完整滚动截图预览']").attributes("src")).toContain("cHJldmlldw==");

    await wrapper.find("[data-testid='manual-scroll-complete']").trigger("click");
    await flushPromises();

    expect(backendMocks.finishScrollingScreenshot).toHaveBeenCalledTimes(1);
    expect(backendMocks.copyImageDataUrl).toHaveBeenCalledWith("data:image/png;base64,bG9uZw==");
    expect(wrapper.find("[data-testid='manual-scroll-controller']").exists()).toBe(false);
    expect(wrapper.find("[data-testid='screenshot-editor']").exists()).toBe(false);
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });

  it("cancels the manual scrolling session from its floating controller", async () => {
    const wrapper = mount(App);
    await flushPromises();

    backendMocks.emit("capture-start", {
      image: "data:image/png;base64,ZmFrZQ==",
      width: 800,
      height: 600,
      originX: 0,
      originY: 0,
      source: "browser",
      mode: "screenshot"
    });
    await flushPromises();

    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { button: 0, clientX: 80, clientY: 70 });
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: 520, clientY: 360 }));
    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 520, clientY: 360 }));
    await flushPromises();

    await wrapper.find("[data-testid='screenshot-scroll-down']").trigger("click");
    await flushPromises();
    await wrapper.find("[data-testid='manual-scroll-cancel']").trigger("click");
    await flushPromises();

    expect(backendMocks.cancelScrollingScreenshot).toHaveBeenCalledTimes(1);
    expect(wrapper.find("[data-testid='manual-scroll-controller']").exists()).toBe(false);
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });

  it("ignores an older screenshot that finishes loading after a newer capture", async () => {
    vi.stubGlobal("Image", ControlledImage);
    mount(App);
    await flushPromises();

    const olderPayload = {
      image: "data:image/png;base64,b2xkZXI=",
      width: 800,
      height: 600,
      originX: 0,
      originY: 0,
      source: "desktop"
    };
    const newerPayload = {
      ...olderPayload,
      image: "data:image/png;base64,bmV3ZXI="
    };

    backendMocks.emit("capture-start", olderPayload);
    await flushPromises();
    expect(controlledImages).toHaveLength(1);

    backendMocks.emit("capture-start", newerPayload);
    await flushPromises();

    expect(controlledImages).toHaveLength(2);
    const [olderImage, newerImage] = controlledImages;
    newerImage.resolve();
    await flushPromises();

    expect(drawImageMock).toHaveBeenCalledTimes(1);
    expect(drawImageMock.mock.calls[0]?.[0]).toBe(newerImage);
    expect(backendMocks.showCaptureWindow).toHaveBeenCalledTimes(1);

    olderImage.resolve();
    await flushPromises();

    expect(drawImageMock).toHaveBeenCalledTimes(1);
    expect(backendMocks.showCaptureWindow).toHaveBeenCalledTimes(1);
  });

  it("keeps drawing when the pointer leaves the capture surface and shows the clamped size", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();

    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { button: 0, clientX: 100, clientY: 100 });
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: 920, clientY: 760 }));
    await flushPromises();

    expect(wrapper.find("[data-testid='selection-size']").text()).toBe("700 × 500");

    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 920, clientY: 760 }));
    await flushPromises();

    expect(backendMocks.processImage).toHaveBeenCalledTimes(1);
  });

  it("shows OCR feedback before the selected PNG finishes encoding", async () => {
    let finishEncoding: (() => void) | undefined;
    const toBlob = vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((callback) => {
      finishEncoding = () => callback(new Blob(["selection"], { type: "image/png" }));
    });

    const wrapper = mount(App);
    await flushPromises();
    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();

    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { button: 0, clientX: 100, clientY: 100 });
    window.dispatchEvent(new MouseEvent("mouseup", { button: 0, clientX: 620, clientY: 420 }));
    await flushPromises();

    expect(wrapper.find("[data-testid='result-panel']").text()).toContain("OCR...");
    expect(toBlob).toHaveBeenCalledTimes(1);
    expect(backendMocks.processImage).not.toHaveBeenCalled();

    const resultPanel = wrapper.find("[data-testid='result-panel']");
    expect(resultPanel.classes()).not.toContain("backdrop-blur-[2px]");
    expect(resultPanel.classes()).not.toContain("bg-white/92");

    finishEncoding?.();
		await vi.waitFor(() => {
			expect(backendMocks.processImage).toHaveBeenCalledTimes(1);
		});
  });

  it("ignores translation events from the previous capture after a new selection starts", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    let captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    await backendMocks.triggerCapture();
    await flushPromises();
    captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 40, clientY: 40 });
    await captureLayer.trigger("mousemove", { clientX: 240, clientY: 120 });
    await captureLayer.trigger("mouseup", { clientX: 240, clientY: 120 });
    await flushPromises();

    expect(backendMocks.processImage.mock.calls[0]?.[2]).toBe(1);
    expect(backendMocks.processImage.mock.calls[1]?.[2]).toBe(2);

    backendMocks.emit("translation-token", { generation: 1, token: "STALE_RESULT" });
    backendMocks.emit("translation-token", { generation: 2, token: "CURRENT_RESULT" });
    backendMocks.emit("translation-done", { generation: 2 });
    await flushPromises();

    expect(wrapper.text()).not.toContain("STALE_RESULT");
    expect(wrapper.text()).toContain("CURRENT_RESULT");
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

  it("renders settings as a single-window shell with grouped content and fixed actions", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Settings']").trigger("click");
    await flushPromises();

    expect(wrapper.find("[data-testid='settings-shell']").exists()).toBe(true);
    expect(wrapper.find("[data-testid='settings-scroll']").exists()).toBe(true);
    expect(wrapper.findAll("[data-testid='settings-section']")).toHaveLength(4);
    expect(wrapper.find("[data-testid='settings-footer']").exists()).toBe(true);
  });

  it("opens settings in Chinese and can switch the page to English", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Settings']").trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("设置");
    expect(wrapper.text()).toContain("AI 服务");
    expect(wrapper.text()).toContain("保存更改");

    await wrapper.find("[data-testid='locale-en']").trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("Settings");
    expect(wrapper.text()).toContain("AI service");
    expect(wrapper.text()).toContain("Save changes");
  });

  it("uses a native Wails drag region for the settings title bar", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Settings']").trigger("click");
    await flushPromises();

    expect(wrapper.find("[data-testid='settings-drag-region']").attributes("style")).toContain(
      "--wails-draggable: drag"
    );
    expect(wrapper.find("[data-testid='locale-en']").attributes("style")).toContain(
      "--wails-draggable: no-drag"
    );
  });

  it("treats a null desktop history payload as an empty list", async () => {
    backendMocks.getHistory.mockResolvedValueOnce(null as never);
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Settings']").trigger("click");
    await flushPromises();

    expect(wrapper.find("[data-testid='settings-shell']").exists()).toBe(true);
    expect(wrapper.text()).toContain("\u6682\u65e0\u7ffb\u8bd1\u8bb0\u5f55\u3002");
  });

  it("handles a settings event while the initial config is still loading", async () => {
    let resolveConfig!: (config: typeof backendMocks.defaultConfig) => void;
    backendMocks.loadConfig.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveConfig = resolve;
        })
    );

    const wrapper = mount(App);
    await vi.waitFor(() => expect(backendMocks.loadConfig).toHaveBeenCalledTimes(1));
    backendMocks.emit("settings-open", {});
    resolveConfig({ ...backendMocks.defaultConfig });
    await flushPromises();

    expect(wrapper.find("[data-testid='settings-shell']").exists()).toBe(true);
  });

  it("shows the desktop settings window only after the settings shell renders", async () => {
    backendMocks.isDesktop = true;
    const wrapper = mount(App);
    await flushPromises();

    let settingsRenderedWhenShown = false;
    backendMocks.showSettingsWindow.mockImplementationOnce(async () => {
      settingsRenderedWhenShown = wrapper.find("[data-testid='settings-shell']").exists();
    });

    backendMocks.emit("settings-open", {});
    await flushPromises();

    expect(backendMocks.showSettingsWindow).toHaveBeenCalledTimes(1);
    expect(settingsRenderedWhenShown).toBe(true);
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

    backendMocks.emit("translation-start", { generation: 1 });
    await flushPromises();

    const progress = wrapper.find("[data-testid='translation-progress']");
    const card = wrapper.find("[data-testid='translation-progress-card']");
    expect(progress.text()).toContain("Translating...");
    expect(progress.classes()).toEqual(expect.arrayContaining(["absolute", "inset-0", "items-center", "justify-center"]));
    expect(card.classes()).toEqual(expect.arrayContaining(["min-h-24", "min-w-32"]));
  });

  it("shrinks a dense plain translation to the selected region without a scrollbar", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 260, clientY: 74 });
    await captureLayer.trigger("mouseup", { clientX: 260, clientY: 74 });
    await flushPromises();

    backendMocks.emit("translation-token", { generation: 1, token: "\u8fd9\u662f\u4e00\u4e2a\u5185\u5bb9\u8f83\u5bc6\u96c6\u7684\u7ffb\u8bd1\u7ed3\u679c\uff0c\u9700\u8981\u6839\u636e\u9009\u533a\u5bbd\u5ea6\u548c\u9ad8\u5ea6\u81ea\u52a8\u8c03\u6574\u5b57\u53f7\uff0c\u4fdd\u8bc1\u8bd1\u6587\u90fd\u80fd\u5728\u533a\u57df\u5185\u5b8c\u6574\u663e\u793a\u3002" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const result = wrapper.find("[data-testid='translation-result']");
    const fontSize = stylePx(result.attributes("style"), "font-size");
    expect(fontSize).toBeLessThan(19);
    expect(result.classes()).toContain("overflow-hidden");
    expect(result.classes()).not.toContain("overflow-y-auto");
  });

  it("batches streamed translation fragments into one animation-frame update", async () => {
    const frames: FrameRequestCallback[] = [];
    const requestFrame = vi.fn((callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("requestAnimationFrame", requestFrame);
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    const wrapper = mount(App);
    await flushPromises();
    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 420, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 420, clientY: 80 });
    await flushPromises();

    backendMocks.emit("translation-token", { generation: 1, token: "[1] 流式" });
    backendMocks.emit("translation-token", { generation: 1, token: "翻译" });
    await flushPromises();

    expect(requestFrame).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).not.toContain("流式翻译");

    frames.shift()?.(16);
    await flushPromises();
    expect(wrapper.text()).toContain("流式翻译");
  });

  it("expands a thin result selection for a long translation error", async () => {
    const wrapper = mount(App);
    await flushPromises();
    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 80, clientY: 70 });
    await captureLayer.trigger("mousemove", { clientX: 500, clientY: 102 });
    await captureLayer.trigger("mouseup", { clientX: 500, clientY: 102 });
    await flushPromises();

    backendMocks.emit("ocr-result", {
      generation: 1,
      blocks: [{ text: "Every key authenticates requests to the gateway", x: 0, y: 0, width: 0.96, height: 0.7 }]
    });
    backendMocks.emit("workflow-error", {
      generation: 1,
      stage: "translation",
      message: 'Post "https://litellm.example/v1/chat/completions": EOF'
    });
    await flushPromises();

    const resultPanel = wrapper.find("[data-testid='result-panel']");
    expect(stylePx(resultPanel.attributes("style"), "height")).toBeGreaterThanOrEqual(112);
    expect(wrapper.find("[data-testid='result-error']").classes()).toContain("break-words");
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

    backendMocks.emit("translation-token", { generation: 1, token: "\u6d4b\u8bd5" });
    backendMocks.emit("translation-done", { generation: 1 });
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

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        { text: "Neutral", x: 0.16, y: 0.2, width: 0.15, height: 0.36 },
        { text: "Negative", x: 0.39, y: 0.2, width: 0.17, height: 0.36 },
        { text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "\u4e2d\u6027\n\u8d1f\u9762\n\u6b63\u9762" });
    backendMocks.emit("translation-done", { generation: 1 });
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
    expect(labels[2].attributes("style")).toContain("font-size: 16px");
    expect(labels[2].attributes("style")).toContain("font-weight: 500");
    expect(labels[2].attributes("style")).toContain("color: rgb(248, 250, 252)");
    expect(labels[2].attributes("style")).toContain("background-color: rgba(19, 25, 39");
    expect(labels[2].attributes("style")).toContain("box-shadow: none");
    expect(wrapper.find("button[aria-label='Copy translated screenshot']").exists()).toBe(true);
  });

	it("keeps dense navigation translations on the source row", async () => {
		const wrapper = mount(App);
		await flushPromises();

		await wrapper.find("button[aria-label='Capture']").trigger("click");
		await flushPromises();
		const captureLayer = wrapper.find("section.cursor-crosshair");
		await captureLayer.trigger("mousedown", { clientX: 20, clientY: 80 });
		await captureLayer.trigger("mousemove", { clientX: 780, clientY: 480 });
		await captureLayer.trigger("mouseup", { clientX: 780, clientY: 480 });
		await flushPromises();

		backendMocks.emit("ocr-result", {
			generation: 1,
			blocks: [
				{ text: "All Models", x: 0.01, y: 0.04, width: 0.08, height: 0.08 },
				{ text: "Add Model", x: 0.11, y: 0.04, width: 0.08, height: 0.08 },
				{ text: "LLM Credentials", x: 0.21, y: 0.04, width: 0.13, height: 0.08 },
				{ text: "Health Status", x: 0.36, y: 0.04, width: 0.11, height: 0.08 },
				{ text: "Model Retry Settings", x: 0.49, y: 0.04, width: 0.16, height: 0.08 },
				{
					text: "To access these models: Create a Virtual Key without selecting a team on the Virtual Keys page",
					x: 0.02,
					y: 0.58,
					width: 0.78,
					height: 0.12
				}
			]
		});
		backendMocks.emit("translation-token", {
			generation: 1,
			token: "[1] 所有模型\n[2] 添加模型\n[3] 大语言模型凭证\n[4] 健康状态\n[5] 模型重试设置\n[6] 要访问这些模型，请在虚拟密钥页面创建一个未选择团队的虚拟密钥"
		});
		backendMocks.emit("translation-done", { generation: 1 });
		await flushPromises();

		const labels = wrapper.findAll("[data-testid='ocr-block']");
		expect(labels).toHaveLength(6);
		expect(wrapper.findAll("[data-testid='translation-line']")).toHaveLength(0);
		const firstRowTops = labels.slice(0, 5).map((label) => stylePx(label.attributes("style"), "top"));
		expect(new Set(firstRowTops)).toEqual(new Set([16]));
	});

	it("keeps a login form translation inside each source text box", async () => {
		const wrapper = mount(App);
		await flushPromises();

		await wrapper.find("button[aria-label='Capture']").trigger("click");
		await flushPromises();
		const captureLayer = wrapper.find("section.cursor-crosshair");
		await captureLayer.trigger("mousedown", { clientX: 8, clientY: 8 });
		await captureLayer.trigger("mousemove", { clientX: 792, clientY: 592 });
		await captureLayer.trigger("mouseup", { clientX: 792, clientY: 592 });
		await flushPromises();

		backendMocks.emit("ocr-result", {
			generation: 1,
			blocks: [
				{ text: "LiteLLM", x: 0.38, y: 0.09, width: 0.12, height: 0.05 },
				{ text: "Login", x: 0.41, y: 0.17, width: 0.07, height: 0.04 },
				{ text: "Access your LiteLLM Admin UI.", x: 0.36, y: 0.23, width: 0.2, height: 0.03 },
				{ text: "Default Credentials", x: 0.3, y: 0.31, width: 0.15, height: 0.04 },
				{
					text: "By default, Username is admin and Password is your set LiteLLM Proxy MASTER_KEY",
					x: 0.3,
					y: 0.37,
					width: 0.34,
					height: 0.07
				},
				{
					text: "Need to setup UI credentials or SSO? Check the documentation.",
					x: 0.3,
					y: 0.48,
					width: 0.3,
					height: 0.06
				},
				{ text: "Username", x: 0.25, y: 0.58, width: 0.08, height: 0.03 },
				{ text: "admin", x: 0.26, y: 0.63, width: 0.06, height: 0.03 },
				{ text: "Password", x: 0.25, y: 0.7, width: 0.08, height: 0.03 },
				{ text: "Login", x: 0.42, y: 0.8, width: 0.05, height: 0.04 },
				{ text: "Login with SSO", x: 0.38, y: 0.89, width: 0.13, height: 0.04 }
			]
		});
		backendMocks.emit("translation-token", {
			generation: 1,
			token: "[1] [1] 2 LiteLLM\n[2] 登录\n[3] 访问您的 LiteLLM 管理界面。\n[4] 默认凭据\n[5] 默认情况下，用户名为 admin，密码为您设置的 LiteLLM Proxy MASTER_KEY\n[6] 需要设置界面凭据或 SSO？请查看文档。\n[7] 用户名\n[8] admin\n[9] 密码\n[10] 登录\n[11] 使用 SSO 登录"
		});
		backendMocks.emit("translation-done", { generation: 1 });
		await flushPromises();

		const labels = wrapper.findAll("[data-testid='ocr-block']");
		expect(wrapper.findAll("[data-testid='translation-line']")).toHaveLength(0);
		expect(wrapper.findAll("[data-testid='translation-cover']")).toHaveLength(0);
		expect(labels.map((label) => label.text())).not.toContain("[1] 2 LiteLLM");
		expect(labels.find((label) => label.text() === "登录")?.attributes("style")).toContain("width: 55px");
	});

  it("renders only trusted translated OCR replacements and drops leaked delimiters", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 120, clientY: 40 });
    await captureLayer.trigger("mousemove", { clientX: 720, clientY: 360 });
    await captureLayer.trigger("mouseup", { clientX: 720, clientY: 360 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        { text: "Top 10 Model Popularity", x: 0.35, y: 0.08, width: 0.3, height: 0.08 },
        { text: "30", x: 0.92, y: 0.88, width: 0.04, height: 0.06 },
        { text: "Neutral", x: 0.55, y: 0.9, width: 0.1, height: 0.06 },
        { text: "Negative", x: 0.68, y: 0.9, width: 0.12, height: 0.06 },
        { text: "Positive", x: 0.82, y: 0.9, width: 0.11, height: 0.06 }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "OCR_TEXT_BEGIN\n[1] \u5341\u5927\u70ed\u95e8\u6a21\u578b\n[2] 30\n[3] \u4e2d\u6027\n[4] \u8d1f\u9762\n[5] \u6b63\u9762\nOCR_TEXT_END" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const labels = wrapper.findAll("[data-testid='ocr-block']");
    expect(labels.map((label) => label.text())).toEqual([
      "\u5341\u5927\u70ed\u95e8\u6a21\u578b",
      "\u4e2d\u6027",
      "\u8d1f\u9762",
      "\u6b63\u9762"
    ]);
    expect(wrapper.text()).not.toContain("OCR_TEXT_BEGIN");
    expect(wrapper.text()).not.toContain("OCR_TEXT_END");
    expect(labels.some((label) => label.text() === "30")).toBe(false);
    expect(labels[0].attributes("style")).toContain("white-space: normal");
    expect(labels[0].attributes("style")).toContain("overflow-wrap: anywhere");
    expect(labels[0].attributes("style")).toContain("box-shadow: none");
  });

  it("renders prose OCR translations from each original text line position", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        {
          text: "State of the Art of Coding Models",
          x: 0.24,
          y: 0,
          width: 0.52,
          height: 0.12
        },
        {
          text: "The space of AI-assisted coding is evolving rapidly.",
          x: 0,
          y: 0.15,
          width: 0.86,
          height: 0.08
        },
        {
          text: "Each day, the pipeline",
          x: 0,
          y: 0.34,
          width: 0.32,
          height: 0.09
        }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] \u7f16\u7801\u6a21\u578b\u6280\u672f\u73b0\u72b6\n[2] AI \u8f85\u52a9\u7f16\u7801\u7684\u9886\u57df\u6b63\u5728\u8fc5\u901f\u6f14\u53d8\u3002\n[3] \u6bcf\u5929\uff0c\u8fd9\u6761\u6d41\u7a0b" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const lines = wrapper.findAll("[data-testid='translation-line']");
    expect(lines).toHaveLength(3);
    expect(wrapper.findAll("[data-testid='ocr-block']")).toHaveLength(0);
    expect(wrapper.find("[data-testid='translation-flow']").exists()).toBe(false);
    expect(lines[0].text()).toBe("\u7f16\u7801\u6a21\u578b\u6280\u672f\u73b0\u72b6");
    expect(lines[0].attributes("style")).toContain("left: 189px");
    expect(lines[0].attributes("style")).toContain("top: 0px");
    expect(lines[0].attributes("style")).toContain("font-size: 26px");
    expect(lines[0].attributes("style")).toContain("color: rgb(255, 83, 22)");
    expect(lines[1].attributes("style")).toContain("left: 0px");
    expect(lines[1].attributes("style")).toContain("top: 53px");
    expect(lines[1].attributes("style")).toContain("font-size: 24px");
    expect(lines[1].attributes("style")).toContain("text-align: left");
  });

  it("keeps target-language Chinese rows while translating nearby English rows in place", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        {
          text: "\u5982\u679c\u8fd8\u6ca1\u6709\u5b89\u88c5 Wails\uff0c\u4e5f\u53ef\u4ee5\u7528\u6a21\u62df\u622a\u56fe\u548c\u6d41\u5f0f\u54cd\u5e94\u9884\u89c8\u524d\u7aef\uff1a",
          x: 0,
          y: 0.15,
          width: 0.72,
          height: 0.08
        },
        {
          text: "If Wails is not installed yet, the frontend can still be previewed with a simulated capture.",
          x: 0,
          y: 0.4,
          width: 0.86,
          height: 0.08
        }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] \u5982\u679c\u8fd8\u6ca1\u6709\u5b89\u88c5 Wails\uff0c\u4e5f\u53ef\u4ee5\u7528\u6a21\u62df\u622a\u56fe\u548c\u6d41\u5f0f\u54cd\u5e94\u9884\u89c8\u524d\u7aef\uff1a\n[2] \u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails\uff0c\u524d\u7aef\u4ecd\u53ef\u901a\u8fc7\u6a21\u62df\u622a\u56fe\u8fdb\u884c\u9884\u89c8\u3002" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const lines = wrapper.findAll("[data-testid='translation-line']");
    expect(lines).toHaveLength(2);
    expect(lines[0].text()).toContain("\u5982\u679c\u8fd8\u6ca1\u6709\u5b89\u88c5 Wails");
    expect(lines[1].text()).toContain("\u524d\u7aef\u4ecd\u53ef\u901a\u8fc7\u6a21\u62df\u622a\u56fe");
    expect(wrapper.findAll("[data-testid='translation-cover']")).toHaveLength(0);
  });

  it("uses hanging indentation for translated numbered list lines", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        {
          text: "2. Prompts an LLM to select those posts whose titles are about LLMs or coding in general.",
          x: 0.02,
          y: 0.28,
          width: 0.9,
          height: 0.08
        }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] 2. \u63d0\u793a\u4e00\u4e2a\u5927\u8bed\u8a00\u6a21\u578b\u7b5b\u9009\u51fa\u6807\u9898\u6d89\u53ca LLM \u6216\u7f16\u7801\u7684\u5e16\u5b50\uff0c\u56e0\u4e3a\u6211\u4eec\u8ba4\u4e3a\u8fd9\u4e9b\u5e16\u5b50\u4e2d\u4f1a\u6709\u66f4\u591a\u76f8\u5173\u8ba8\u8bba\u3002" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const line = wrapper.find("[data-testid='translation-line']");
    expect(line.exists()).toBe(true);
    expect(line.attributes("style")).toContain("padding: 1px 2px 1px 43px");
    expect(line.attributes("style")).toContain("text-indent: -43px");
  });

  it("reserves vertical space when long anchored prose translations wrap", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        {
          text: "The space of AI-assisted coding is evolving rapidly with the latest developments.",
          x: 0.58,
          y: 0.15,
          width: 0.42,
          height: 0.08
        },
        {
          text: "Each day, the pipeline",
          x: 0,
          y: 0.19,
          width: 0.32,
          height: 0.08
        }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] AI \u8f85\u52a9\u7f16\u7801\u9886\u57df\u6b63\u5728\u5feb\u901f\u53d1\u5c55\uff0c\u672c\u6587\u901a\u8fc7\u6355\u6349 Hacker News \u8bc4\u8bba\u4e2d\u7f16\u7801\u6a21\u578b\u7684\u6d41\u884c\u5ea6\u548c\u7528\u6237\u60c5\u7eea\uff0c\u8bd5\u56fe\u8ddf\u4e0a\u6700\u65b0\u8fdb\u5c55\u3002\n[2] \u6bcf\u65e5\u6d41\u7a0b\uff1a" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const lines = wrapper.findAll("[data-testid='translation-line']");
    expect(lines).toHaveLength(2);

    const firstStyle = lines[0].attributes("style");
    const secondStyle = lines[1].attributes("style");
    const firstTop = stylePx(firstStyle, "top");
    const firstHeight = stylePx(firstStyle, "min-height");
    const firstLineHeight = stylePx(firstStyle, "line-height");
    const secondTop = stylePx(secondStyle, "top");

    expect(firstHeight).toBeGreaterThan(firstLineHeight + 2);
    expect(secondTop).toBeGreaterThanOrEqual(firstTop + firstHeight + 2);
  });

  it("shrinks dense numbered prose rows before they overflow the anchored width", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        {
          text: "2. Prompts an LLM to select those posts whose titles are about LLMs or coding in general.",
          x: 0.02,
          y: 0.32,
          width: 0.9,
          height: 0.08
        },
        {
          text: "3. For each post, sends the title and comments to Gemini and asks it to identify models.",
          x: 0.04,
          y: 0.42,
          width: 0.9,
          height: 0.08
        },
        {
          text: "I wanted the ability to audit the process and the results.",
          x: 0,
          y: 0.52,
          width: 0.86,
          height: 0.08
        }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] 2. \u63d0\u793a\u4e00\u4e2a LLM\uff08\u5927\u8bed\u8a00\u6a21\u578b\uff09\u7b5b\u9009\u51fa\u6807\u9898\u6d89\u53ca LLM \u6216\u7f16\u7801\u7684\u5e16\u5b50\uff08\u6700\u591a50\u7bc7\uff09\uff0c\u56e0\u4e3a\u6211\u4eec\u8ba4\u4e3a\u8fd9\u4e9b\u5e16\u5b50\u4e2d\u7684\u8ba8\u8bba\u66f4\u5177\u76f8\u5173\u6027\uff08\u4e2a\u4eba\u5047\u8bbe\uff09\u3002\n[2] 3. \u5bf9\u6bcf\u7bc7\u5e16\u5b50\uff0c\u5c06\u6807\u9898\u548c\u8bc4\u8bba\u53d1\u9001\u81f3 Gemini\uff0c\u8981\u6c42\u5176\u4ece OpenRouter \u6a21\u578b\u5217\u8868\u4e2d\u8bc6\u522b\u6a21\u578b\uff0c\u5e76\u8bc4\u4f30\u6bcf\u6761\u8bc4\u8bba\u4e2d\u5bf9\u6bcf\u4e2a\u63d0\u53ca\u6a21\u578b\u7684\u60c5\u611f\u503e\u5411\u3002\n[3] \u6211\u5e0c\u671b\u80fd\u591f\u5ba1\u8ba1\u6d41\u7a0b\u548c\u7ed3\u679c\u3002" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const lines = wrapper.findAll("[data-testid='translation-line']");
    expect(lines).toHaveLength(3);

    const firstStyle = lines[0].attributes("style");
    const secondStyle = lines[1].attributes("style");
    expect(stylePx(firstStyle, "font-size")).toBeLessThanOrEqual(18);
    expect(stylePx(secondStyle, "font-size")).toBeLessThanOrEqual(18);
    expect(firstStyle).toContain("box-sizing: border-box");
    expect(secondStyle).toContain("box-sizing: border-box");
  });

  it("keeps streamed translations visible when a translation error arrives after partial output", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        { text: "The space of AI-assisted coding is evolving rapidly.", x: 0, y: 0, width: 0.86, height: 0.08 },
        { text: "Each day, the pipeline", x: 0, y: 0.2, width: 0.32, height: 0.08 }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] AI \u8f85\u52a9\u7f16\u7801\u7684\u9886\u57df\u6b63\u5728\u8fc5\u901f\u6f14\u53d8\u3002" });
    backendMocks.emit("workflow-error", {
      stage: "translation",
      message: "translation incomplete: expected 2 numbered translation lines, got 1; missing [2]; received [1]"
    });
    await flushPromises();

    expect(wrapper.text()).toContain("AI \u8f85\u52a9\u7f16\u7801");
    expect(wrapper.text()).not.toContain("translation incomplete");
    expect(wrapper.findAll("[data-testid='translation-line']")).toHaveLength(1);
  });

  it("covers untranslated source rows in prose layout without copying unchanged English", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        {
          text: "The space of AI-assisted coding is evolving rapidly.",
          x: 0,
          y: 0,
          width: 0.86,
          height: 0.08
        },
        {
          text: "You can open a comment by appending the comment ID to https://news.ycombinator.com/item?id=.",
          x: 0,
          y: 0.78,
          width: 0.88,
          height: 0.07
        }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] AI \u8f85\u52a9\u7f16\u7801\u7684\u9886\u57df\u6b63\u5728\u8fc5\u901f\u6f14\u53d8\u3002\n[2] You can open a comment by appending the comment ID to https://news.ycombinator.com/item?id=." });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const translatedLines = wrapper.findAll("[data-testid='translation-line']");
    const covers = wrapper.findAll("[data-testid='translation-cover']");
    expect(translatedLines).toHaveLength(1);
    expect(covers).toHaveLength(1);
    expect(covers[0].text()).toBe("");
    expect(covers[0].attributes("style")).toContain("top: 276px");
    expect(wrapper.text()).not.toContain("You can open a comment");
  });

  it("suppresses adjacent duplicate prose translations while covering the source row", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 14, clientY: 132 });
    await captureLayer.trigger("mousemove", { clientX: 1068, clientY: 486 });
    await captureLayer.trigger("mouseup", { clientX: 1068, clientY: 486 });
    await flushPromises();

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [
        {
          text: "I wanted the ability to audit the process and the results, for debugging and sanity checks.",
          x: 0,
          y: 0.62,
          width: 0.9,
          height: 0.08
        },
        {
          text: "results are logged to a Google Sheet, where you can see the comment IDs and sentiment.",
          x: 0,
          y: 0.7,
          width: 0.9,
          height: 0.08
        },
        {
          text: "You can open a comment by appending the comment ID to https://news.ycombinator.com/item?id=.",
          x: 0,
          y: 0.82,
          width: 0.88,
          height: 0.07
        }
      ]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] \u6211\u5e0c\u671b\u80fd\u591f\u5ba1\u8ba1\u6d41\u7a0b\u4e0e\u7ed3\u679c\uff0c\u7528\u4e8e\u8c03\u8bd5\u53ca\u5b9a\u671f\u9a8c\u8bc1\u6a21\u578b\u8f93\u51fa\u3002\u56e0\u6b64\u7ed3\u679c\u8bb0\u5f55\u5728 Google Sheet \u4e2d\uff0c\u53ef\u67e5\u770b\u63d0\u53ca\u7279\u5b9a\u6a21\u578b\u7684\u8bc4\u8bba ID \u53ca\u6a21\u578b\u5224\u5b9a\u7684\u60c5\u611f\u503e\u5411\u3002\n[2] \u6211\u5e0c\u671b\u80fd\u591f\u5ba1\u8ba1\u6d41\u7a0b\u548c\u7ed3\u679c\uff0c\u7528\u4e8e\u8c03\u8bd5\u4ee5\u53ca\u5076\u5c14\u5bf9\u6a21\u578b\u8f93\u51fa\u8fdb\u884c\u5408\u7406\u6027\u68c0\u67e5\u3002\u56e0\u6b64\uff0c\u7ed3\u679c\u4f1a\u8bb0\u5f55\u5230 Google \u8868\u683c\u4e2d\uff0c\u4f60\u53ef\u4ee5\u770b\u5230\u63d0\u53ca\u7279\u5b9a\u6a21\u578b\u7684\u8bc4\u8bba ID \u4ee5\u53ca\u6a21\u578b\u5224\u65ad\u51fa\u7684\u60c5\u611f\u503e\u5411\u3002\n[3] \u4f60\u53ef\u4ee5\u901a\u8fc7\u5c06\u8bc4\u8bba ID \u9644\u52a0\u5230 https://news.ycombinator.com/item?id= \u6765\u6253\u5f00\u8bc4\u8bba\u3002" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const translatedLines = wrapper.findAll("[data-testid='translation-line']");
    const covers = wrapper.findAll("[data-testid='translation-cover']");
    expect(translatedLines).toHaveLength(2);
    expect(covers).toHaveLength(1);
    expect(covers[0].text()).toBe("");
    expect(wrapper.text()).toContain("\u6211\u5e0c\u671b\u80fd\u591f\u5ba1\u8ba1\u6d41\u7a0b\u4e0e\u7ed3\u679c");
    expect(wrapper.text()).not.toContain("\u5076\u5c14\u5bf9\u6a21\u578b\u8f93\u51fa\u8fdb\u884c\u5408\u7406\u6027\u68c0\u67e5");
    expect(wrapper.text()).toContain("https://news.ycombinator.com/item?id=");
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

    backendMocks.emit("ocr-result", { generation: 1,
      blocks: [{ text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 }]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "\u6b63\u9762" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    await wrapper.find("button[aria-label='Copy translated screenshot']").trigger("click");
    await flushPromises();

    expect(backendMocks.copyImageDataUrl).toHaveBeenCalledWith("data:image/png;base64,c2VsZWN0aW9u");
  });

  it("reprocesses the current selection in English when reversing translation direction", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 148, clientY: 26 });
    await captureLayer.trigger("mousemove", { clientX: 600, clientY: 78 });
    await captureLayer.trigger("mouseup", { clientX: 600, clientY: 78 });
    await flushPromises();

    expect(backendMocks.processImage).toHaveBeenLastCalledWith("data:image/png;base64,c2VsZWN0aW9u", "to-zh", 1);

    backendMocks.emit("translation-token", { generation: 1, token: "\u6d4b\u8bd5" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    await wrapper.find("button[aria-label='Reverse translation direction']").trigger("click");
    await flushPromises();

    expect(backendMocks.processImage).toHaveBeenLastCalledWith("data:image/png;base64,c2VsZWN0aW9u", "to-en", 2);
    expect(wrapper.text()).toContain("Translating...");
  });

  it("drops stale workflow events from an earlier generation", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    backendMocks.emit("ocr-result", {
      generation: 1,
      blocks: [{ text: "Positive", x: 0.62, y: 0.2, width: 0.18, height: 0.36 }]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] \u6b63\u9762" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();
    expect(wrapper.text()).toContain("\u6b63\u9762");

    backendMocks.emit("ocr-result", { generation: 7, blocks: [{ text: "Ghost", x: 0.1, y: 0.1, width: 0.2, height: 0.1 }] });
    backendMocks.emit("translation-token", { generation: 7, token: "\u5e7d\u7075" });
    backendMocks.emit("translation-done", { generation: 7 });
    await flushPromises();

    expect(wrapper.text()).not.toContain("\u5e7d\u7075");
    const labels = wrapper.findAll("[data-testid='ocr-block']");
    expect(labels.length).toBeGreaterThan(0);
    expect(labels.every((label) => label.text() !== "\u5e7d\u7075")).toBe(true);
  });

  it("uses the backend-resolved direction when auto direction is enabled", async () => {
    backendMocks.loadConfig.mockResolvedValueOnce({
      ...backendMocks.defaultConfig,
      autoDirection: true
    });
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    expect(backendMocks.processImage).toHaveBeenLastCalledWith("data:image/png;base64,c2VsZWN0aW9u", "auto", 1);

    backendMocks.emit("translation-direction", { generation: 1, direction: "to-en" });
    backendMocks.emit("ocr-result", {
      generation: 1,
      blocks: [{ text: "\u8bbe\u7f6e", x: 0.1, y: 0.1, width: 0.2, height: 0.1 }]
    });
    backendMocks.emit("translation-token", { generation: 1, token: "[1] Settings" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    const labels = wrapper.findAll("[data-testid='ocr-block']");
    expect(labels.length).toBeGreaterThan(0);
    expect(labels.some((label) => label.text() === "Settings")).toBe(true);
  });

  it("keeps the manually reversed direction until the next capture", async () => {
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    backendMocks.emit("translation-token", { generation: 1, token: "\u6d4b\u8bd5" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    await wrapper.find("button[aria-label='Reverse translation direction']").trigger("click");
    await flushPromises();

    expect(backendMocks.processImage).toHaveBeenLastCalledWith("data:image/png;base64,c2VsZWN0aW9u", "to-en", 2);

    backendMocks.emit("translation-direction", { generation: 2, direction: "to-zh" });
    backendMocks.emit("translation-token", { generation: 2, token: "test" });
    backendMocks.emit("translation-done", { generation: 2 });
    await flushPromises();

    await wrapper.find("button[aria-label='Reverse translation direction']").trigger("click");
    await flushPromises();

    expect(backendMocks.processImage).toHaveBeenLastCalledWith("data:image/png;base64,c2VsZWN0aW9u", "to-zh", 3);
  });

  it("auto-copies the translation result when enabled", async () => {
    backendMocks.loadConfig.mockResolvedValueOnce({
      ...backendMocks.defaultConfig,
      autoCopy: true
    });
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.find("button[aria-label='Capture']").trigger("click");
    await flushPromises();
    const captureLayer = wrapper.find("section.cursor-crosshair");
    await captureLayer.trigger("mousedown", { clientX: 20, clientY: 20 });
    await captureLayer.trigger("mousemove", { clientX: 180, clientY: 80 });
    await captureLayer.trigger("mouseup", { clientX: 180, clientY: 80 });
    await flushPromises();

    backendMocks.emit("translation-token", { generation: 1, token: "\u6d4b\u8bd5" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    expect(backendMocks.copyText).toHaveBeenCalledWith("\u6d4b\u8bd5");
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

    backendMocks.emit("translation-token", { generation: 1, token: "测试" });
    backendMocks.emit("translation-done", { generation: 1 });
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

    backendMocks.emit("translation-token", { generation: 1, token: "测试" });
    backendMocks.emit("translation-done", { generation: 1 });
    await flushPromises();

    await wrapper.find("main").trigger("contextmenu");
    await flushPromises();

    expect(wrapper.text()).not.toContain("测试");
    expect(backendMocks.hideWindow).toHaveBeenCalledTimes(1);
  });
});
