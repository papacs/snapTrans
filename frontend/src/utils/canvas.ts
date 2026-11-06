export function releaseCanvas(canvas: HTMLCanvasElement | null | undefined): void {
  if (canvas) canvas.width = canvas.height = 0;
}

export function encodeTemporaryPNG(canvas: HTMLCanvasElement): string {
  try { return canvas.toDataURL("image/png"); }
  finally { releaseCanvas(canvas); }
}
