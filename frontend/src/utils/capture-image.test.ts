import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { loadCaptureImage } from "./capture-image";

class TestImage {
  static instances: TestImage[] = [];
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  src = "";
  constructor() { TestImage.instances.push(this); }
  removeAttribute(name: string) { if (name === "src") this.src = ""; }
}
beforeEach(() => { TestImage.instances = []; vi.stubGlobal("Image", TestImage); });
afterEach(() => vi.unstubAllGlobals());

it("stops an obsolete decode without failing the next capture", async () => {
  const controller = new AbortController();
  const loaded = loadCaptureImage("/capture/old.png", controller.signal);
  const image = TestImage.instances[0]!;
  controller.abort();
  await expect(loaded).resolves.toBeNull();
  expect(image.src).toBe("");
  expect(image.onload).toBeNull();
  expect(image.onerror).toBeNull();
});

it("does not start loading an already cancelled capture", async () => {
  const controller = new AbortController();
  controller.abort();
  await expect(loadCaptureImage("/capture/old.png", controller.signal)).resolves.toBeNull();
  expect(TestImage.instances[0]!.src).toBe("");
});

it("returns the loaded image and removes stale event handlers", async () => {
  const loaded = loadCaptureImage("/capture/current.png", new AbortController().signal);
  const image = TestImage.instances[0]!;
  image.onload!();
  await expect(loaded).resolves.toBe(image);
  expect(image.onload).toBeNull();
  expect(image.onerror).toBeNull();
});

it("releases failed image data before reporting a decode error", async () => {
  const loaded = loadCaptureImage("/capture/broken.png", new AbortController().signal);
  const image = TestImage.instances[0]!;
  const failed = expect(loaded).rejects.toThrow("Failed to load captured image");
  image.onerror!();
  await failed;
  expect(image.src).toBe("");
});
