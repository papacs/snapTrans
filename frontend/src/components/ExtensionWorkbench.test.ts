import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import ExtensionWorkbench from "./ExtensionWorkbench.vue";
import { defaultFeatures } from "../utils/features";
const mocks = vi.hoisted(() => ({
  start: vi.fn(async (_request: unknown) => {}),
  cancel: vi.fn(async () => {}),
  extract: vi.fn(async () => ({ text: "OCR text", blocks: [] })),
  save: vi.fn(async () => {}),
  listener: null as null | ((event: any) => void),
}));
vi.mock("../services/backend", () => ({
  startTextAction: mocks.start,
  cancelTextAction: mocks.cancel,
  extractText: mocks.extract,
  saveLearningCard: mocks.save,
  onBackendEvent: (_name: string, fn: (event: any) => void) => {
    mocks.listener = fn;
    return () => {
      mocks.listener = null;
    };
  },
  copyText: vi.fn(async () => {}),
  copyImageDataUrl: vi.fn(async () => {}),
  pinImage: vi.fn(async () => {}),
  saveScreenshot: vi.fn(async () => ""),
}));
enableAutoUnmount(afterEach);
beforeEach(() => {
  vi.clearAllMocks();
  mocks.listener = null;
});
const props = {
  features: { ...defaultFeatures },
  locale: "zh-CN",
  image: "data:image/png;base64,test",
  source: "connection refused",
  origin: { x: 0, y: 0 },
};
describe("extension workbench", () => {
  it("does not show disabled experiments or run OCR on open", () => {
    const wrapper = mount(ExtensionWorkbench, { props });
    expect(
      wrapper.find('[data-testid="extension-tab-memeExplanation"]').exists(),
    ).toBe(false);
    expect(
      wrapper.find('[data-testid="extension-tab-imageCompare"]').exists(),
    ).toBe(false);
    expect(mocks.extract).not.toHaveBeenCalled();
    expect(mocks.start).not.toHaveBeenCalled();
  });
  it("streams only matching requests and ignores late tokens after stop", async () => {
    const wrapper = mount(ExtensionWorkbench, { props });
    await wrapper
      .get('[data-testid="extension-tab-textActions"]')
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "解释一下")!
      .trigger("click");
    await flushPromises();
    expect(mocks.extract).not.toHaveBeenCalled();
    const request = mocks.start.mock.calls[0]![0] as any;
    mocks.listener!({ id: "other", token: "wrong", done: false });
    await flushPromises();
    expect(wrapper.text()).not.toContain("wrong");
    mocks.listener!({ id: request.id, token: "possible cause", done: false });
    await flushPromises();
    expect(wrapper.text()).toContain("possible cause");
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "停止")!
      .trigger("click");
    expect(mocks.cancel).toHaveBeenCalledWith(request.id);
    mocks.listener!({ id: request.id, token: "late", done: true });
    await flushPromises();
    expect(wrapper.text()).not.toContain("late");
  });
  it("saves an editable card locally without a model request", async () => {
    const wrapper = mount(ExtensionWorkbench, {
      props: {
        ...props,
        features: { ...defaultFeatures, learningCards: true },
        translation: "连接被拒绝",
      },
    });
    await wrapper
      .get('[data-testid="extension-tab-learningCards"]')
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "保存学习卡片")!
      .trigger("click");
    await flushPromises();
    expect(mocks.save).toHaveBeenCalledWith(
      "connection refused",
      "连接被拒绝",
      "",
    );
    expect(mocks.start).not.toHaveBeenCalled();
  });
  it("does not start a model request after closing during OCR", async () => {
    let resolve!: (value: any) => void;
    mocks.extract.mockImplementationOnce(
      () => new Promise((r) => (resolve = r)),
    );
    const wrapper = mount(ExtensionWorkbench, {
      props: { ...props, source: "" },
    });
    await wrapper
      .get('[data-testid="extension-tab-textActions"]')
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "解释一下")!
      .trigger("click");
    wrapper.unmount();
    resolve({ text: "late OCR", blocks: [] });
    await flushPromises();
    expect(mocks.start).not.toHaveBeenCalled();
  });
});
