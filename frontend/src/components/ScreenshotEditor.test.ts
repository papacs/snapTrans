import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ScreenshotEditor from "./ScreenshotEditor.vue";
import type { OCRResultPayload } from "../services/backend";

const mocks = vi.hoisted(() => ({
  extract: vi.fn(async (_image: string): Promise<OCRResultPayload> => ({ text: "Original text", blocks: [] })),
  copy: vi.fn(async (_text: string) => {}),
}));
vi.mock("../services/backend", () => ({
  extractText: mocks.extract,
  copyText: mocks.copy,
  beginScrollingScreenshot: vi.fn(),
  cancelScrollingScreenshot: vi.fn(),
  finishScrollingScreenshot: vi.fn(),
  showCaptureWindow: vi.fn(),
  stepScrollingScreenshot: vi.fn(),
}));

enableAutoUnmount(afterEach);
beforeEach(() => {
  mocks.extract.mockReset();
  mocks.copy.mockClear();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
    clearRect: vi.fn(), drawImage: vi.fn(),
    save: vi.fn(), restore: vi.fn(), fillText: vi.fn(),
  } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, "toDataURL").mockReturnValue("data:image/png;base64,capture");
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    x: 0, y: 0, left: 0, top: 0, right: 800, bottom: 600, width: 800, height: 600,
    toJSON: () => ({}),
  } as DOMRect);
});
afterEach(() => vi.restoreAllMocks());

function editor() {
  const sourceCanvas = document.createElement("canvas");
  sourceCanvas.width = 800;
  sourceCanvas.height = 600;
  return mount(ScreenshotEditor, {
    props: {
      sourceCanvas,
      capture: { image: "data:image/png;base64,capture", width: 800, height: 600, mode: "screenshot" },
      rect: { x: 80, y: 70, width: 440, height: 290 },
      locale: "en",
    },
    global: { stubs: { ExtensionWorkbench: true } },
  });
}

const table: OCRResultPayload = {
  text: "Name Score Alice 92",
  blocks: [
    { text: "Name", x: 0.08, y: 0.08, width: 0.16, height: 0.08 },
    { text: "Score", x: 0.62, y: 0.09, width: 0.12, height: 0.08 },
    { text: "Alice", x: 0.09, y: 0.34, width: 0.14, height: 0.08 },
    { text: "92", x: 0.63, y: 0.35, width: 0.05, height: 0.08 },
  ],
};

describe("screenshot extraction recovery", () => {
  it.each(["text", "table"] as const)("ignores an earlier copy failure after editing the extracted %s", async kind => {
    let fail!: (reason: Error) => void;
    mocks.extract.mockResolvedValueOnce(kind === "text" ? {text:"Original text",blocks:[]} : table);
    mocks.copy.mockImplementationOnce(() => new Promise<void>((_resolve,reject) => { fail = reject; }));
    const wrapper = editor();
    await wrapper.get(`[data-testid="screenshot-extract-${kind}"]`).trigger("click");
    await flushPromises();
    await wrapper.get(kind === "text" ? '[data-testid="copy-extracted-text"]' : '[data-testid="copy-table-tsv"]').trigger("click");
    const input = wrapper.get<HTMLInputElement | HTMLTextAreaElement>(kind === "text" ? '[data-testid="extracted-text-editor"]' : '[data-testid="table-cell-1-1"]');
    await input.setValue("Edited content");
    fail(new Error("OLD_COPY_FAILURE"));
    await flushPromises();
    expect(input.element.value).toBe("Edited content");
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
  });
  it.each(["success", "failure"] as const)("ignores an old text copy's %s after recognizing again", async outcome => {
    let finish!: () => void;
    let fail!: (reason: Error) => void;
    mocks.extract.mockResolvedValueOnce({text:"Old recognized text",blocks:[]}).mockResolvedValueOnce({text:"New recognized text",blocks:[]});
    mocks.copy.mockImplementationOnce(() => new Promise<void>((resolve,reject) => { finish = resolve; fail = reject; }));
    const wrapper = editor();
    await wrapper.get('[data-testid="screenshot-extract-text"]').trigger("click");
    await flushPromises();
    await wrapper.get('[data-testid="copy-extracted-text"]').trigger("click");
    await wrapper.get('[data-testid="retry-text-extraction"]').trigger("click");
    await flushPromises();
    if(outcome === "success") finish(); else fail(new Error("OLD_COPY_FAILURE"));
    await flushPromises();
    expect(wrapper.get<HTMLTextAreaElement>('[data-testid="extracted-text-editor"]').element.value).toBe("New recognized text");
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="copy-extracted-text"]').text()).not.toContain("已复制");
  });
  it.each(["success", "failure"] as const)("ignores an old table copy's %s after recognizing again", async outcome => {
    let finish!: () => void;
    let fail!: (reason: Error) => void;
    mocks.extract.mockResolvedValueOnce(table).mockResolvedValueOnce({...table,blocks:table.blocks.map(block => block.text === "92" ? {...block,text:"99"} : block)});
    mocks.copy.mockImplementationOnce(() => new Promise<void>((resolve,reject) => { finish = resolve; fail = reject; }));
    const wrapper = editor();
    await wrapper.get('[data-testid="screenshot-extract-table"]').trigger("click");
    await flushPromises();
    await wrapper.get('[data-testid="copy-table-tsv"]').trigger("click");
    await wrapper.get('[data-testid="retry-table-extraction"]').trigger("click");
    await flushPromises();
    if(outcome === "success") finish(); else fail(new Error("OLD_COPY_FAILURE"));
    await flushPromises();
    expect(wrapper.get<HTMLInputElement>('[data-testid="table-cell-1-1"]').element.value).toBe("99");
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="copy-table-tsv"]').text()).not.toContain("Copied");
  });
  it.each(["failure", "empty"] as const)("keeps edited text after a retry returns %s", async result => {
    let finish!: (value: OCRResultPayload) => void;
    let fail!: (error: Error) => void;
    mocks.extract.mockResolvedValueOnce({ text: "Original text", blocks: [] })
      .mockImplementationOnce(() => new Promise((resolve, reject) => { finish = resolve; fail = reject; }));
    const wrapper = editor();
    await wrapper.get('[data-testid="screenshot-extract-text"]').trigger("click");
    await flushPromises();
    const text = wrapper.get<HTMLTextAreaElement>('[data-testid="extracted-text-editor"]');
    await text.setValue("My corrections");
    await wrapper.get('[data-testid="retry-text-extraction"]').trigger("click");
    expect(text.element.value).toBe("My corrections");
    expect(text.element.disabled).toBe(true);
    if (result === "failure") fail(new Error("OCR unavailable"));
    else finish({ text: "  ", blocks: [] });
    await flushPromises();
    expect(text.element.value).toBe("My corrections");
    expect(text.element.disabled).toBe(false);
    expect(wrapper.get('[role="alert"]').text()).not.toBe("");
    await wrapper.get('[data-testid="copy-extracted-text"]').trigger("click");
    expect(mocks.copy).toHaveBeenCalledWith("My corrections");

    mocks.extract.mockResolvedValueOnce({ text: "New recognition", blocks: [] });
    await wrapper.get('[data-testid="retry-text-extraction"]').trigger("click");
    await flushPromises();
    expect(text.element.value).toBe("New recognition");
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
  });

  it.each(["failure", "empty"] as const)("keeps edited table cells after a retry returns %s", async result => {
    let finish!: (value: OCRResultPayload) => void;
    let fail!: (error: Error) => void;
    mocks.extract.mockResolvedValueOnce(table)
      .mockImplementationOnce(() => new Promise((resolve, reject) => { finish = resolve; fail = reject; }));
    const wrapper = editor();
    await wrapper.get('[data-testid="screenshot-extract-table"]').trigger("click");
    await flushPromises();
    const score = wrapper.get<HTMLInputElement>('[data-testid="table-cell-1-1"]');
    await score.setValue("95");
    await wrapper.get('[data-testid="retry-table-extraction"]').trigger("click");
    expect(score.element.value).toBe("95");
    expect(score.element.disabled).toBe(true);
    if (result === "failure") fail(new Error("OCR unavailable"));
    else finish({ text: "", blocks: [] });
    await flushPromises();
    expect(score.element.value).toBe("95");
    expect(score.element.disabled).toBe(false);
    expect(wrapper.get('[role="alert"]').text()).not.toBe("");
    await wrapper.get('[data-testid="copy-table-tsv"]').trigger("click");
    expect(mocks.copy).toHaveBeenCalledWith("Name\tScore\nAlice\t95");

    mocks.extract.mockResolvedValueOnce(table);
    await wrapper.get('[data-testid="retry-table-extraction"]').trigger("click");
    await flushPromises();
    expect(score.element.value).toBe("92");
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
  });
});

describe("screenshot editor IME shortcuts", () => {
  it("commits an existing text annotation before starting one at another position", async () => {
    const wrapper = editor();
    await wrapper.get('[data-testid="annotation-text"]').trigger("click");
    const canvas = wrapper.get('[data-testid="screenshot-viewport"] canvas');
    await canvas.trigger("mousedown", {button:0,clientX:120,clientY:100});
    await wrapper.get('input[placeholder="输入文字"]').setValue("First annotation");
    await canvas.trigger("mousedown", {button:0,clientX:240,clientY:160});
    expect(wrapper.get('[data-testid="annotation-undo"]').attributes("disabled")).toBeUndefined();
    const context = document.createElement("canvas").getContext("2d")!;
    expect(context.fillText).toHaveBeenCalledWith("First annotation",expect.any(Number),expect.any(Number));
    expect(wrapper.get<HTMLInputElement>('input[placeholder="输入文字"]').element.value).toBe("");
  });
  it.each([{ isComposing: true }, { keyCode: 229 }])("keeps text annotation drafts while the IME handles Enter or Escape (%s)", async options => {
    const wrapper = editor();
    await wrapper.get('[data-testid="annotation-text"]').trigger("click");
    await wrapper.get('[data-testid="screenshot-viewport"] canvas').trigger("mousedown", { button: 0, clientX: 120, clientY: 100 });
    const draft = wrapper.get<HTMLInputElement>('input[placeholder="输入文字"]');
    await draft.setValue("中文标注");
    for (const key of ["Enter", "Escape"]) {
      const event = new KeyboardEvent("keydown", { key, ...options, bubbles: true, cancelable: true });
      draft.element.dispatchEvent(event);
      await flushPromises();
      expect(event.defaultPrevented).toBe(false);
      expect(wrapper.get<HTMLInputElement>('input[placeholder="输入文字"]').element.value).toBe("中文标注");
      expect(wrapper.get('[data-testid="annotation-undo"]').attributes("disabled")).toBeDefined();
    }
    await draft.trigger("keydown", { key: "Enter" });
    expect(wrapper.find('input[placeholder="输入文字"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="annotation-undo"]').attributes("disabled")).toBeUndefined();
  });

  it("discards a text annotation with Escape outside IME composition", async () => {
    const wrapper = editor();
    await wrapper.get('[data-testid="annotation-text"]').trigger("click");
    await wrapper.get('[data-testid="screenshot-viewport"] canvas').trigger("mousedown", { button: 0, clientX: 120, clientY: 100 });
    const draft = wrapper.get('input[placeholder="输入文字"]');
    await draft.setValue("取消的标注");
    await draft.trigger("keydown", { key: "Escape" });
    expect(wrapper.find('input[placeholder="输入文字"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="annotation-undo"]').attributes("disabled")).toBeDefined();
  });

  it.each([{ isComposing: true }, { keyCode: 229 }])("keeps extracted text open when Escape belongs to the IME (%s)", async options => {
    mocks.extract.mockResolvedValueOnce({ text: "识别文字", blocks: [] });
    const wrapper = editor();
    await wrapper.get('[data-testid="screenshot-extract-text"]').trigger("click");
    await flushPromises();
    const text = wrapper.get('[data-testid="extracted-text-editor"]');
    const event = new KeyboardEvent("keydown", { key: "Escape", ...options, bubbles: true, cancelable: true });
    text.element.dispatchEvent(event);
    await flushPromises();
    expect(event.defaultPrevented).toBe(false);
    expect(wrapper.find('[data-testid="text-extraction-panel"]').exists()).toBe(true);
    await text.trigger("keydown", { key: "Escape" });
    expect(wrapper.find('[data-testid="text-extraction-panel"]').exists()).toBe(false);
  });
});
