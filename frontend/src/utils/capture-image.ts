// Aborting a replaced capture also stops its decoder and releases the asset URL.
// A sequence check alone prevents stale drawing but leaves the old bitmap alive.
export function loadCaptureImage(source: string, signal: AbortSignal): Promise<HTMLImageElement | null> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    const cleanup = () => {
      image.onload = null;
      image.onerror = null;
      signal.removeEventListener("abort", abort);
    };
    const abort = () => {
      cleanup();
      image.removeAttribute("src");
      resolve(null);
    };
    image.onload = () => { cleanup(); resolve(image); };
    image.onerror = () => {
      cleanup();
      image.removeAttribute("src");
      reject(new Error("Failed to load captured image"));
    };
    if (signal.aborted) { abort(); return; }
    signal.addEventListener("abort", abort, { once: true });
    image.decoding = "async";
    image.src = source;
  });
}
