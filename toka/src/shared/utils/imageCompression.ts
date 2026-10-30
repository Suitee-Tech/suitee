export interface CompressionOptions {
  maxDimensionPx?: number;
  quality?: number;
  preserveType?: boolean;
}

const DEFAULTS: Required<CompressionOptions> = {
  maxDimensionPx: 2000,
  quality: 0.85,
  preserveType: false,
};

export async function compressImageFile(file: File, options: CompressionOptions = {}): Promise<File> {
  const opts = { ...DEFAULTS, ...options };

  if (!file.type.startsWith("image/")) {
    throw new Error(`Not an image: ${file.type}`);
  }

  const url = URL.createObjectURL(file);
  let img: HTMLImageElement;
  try {
    img = await loadImageElement(url);
  } finally {
    URL.revokeObjectURL(url);
  }

  if (img.naturalWidth <= opts.maxDimensionPx && img.naturalHeight <= opts.maxDimensionPx) {
    return file;
  }

  let targetW: number;
  let targetH: number;
  if (img.naturalWidth >= img.naturalHeight) {
    targetW = opts.maxDimensionPx;
    targetH = Math.round(img.naturalHeight * (opts.maxDimensionPx / img.naturalWidth));
  } else {
    targetH = opts.maxDimensionPx;
    targetW = Math.round(img.naturalWidth * (opts.maxDimensionPx / img.naturalHeight));
  }

  const canvas = document.createElement("canvas");
  canvas.width = targetW;
  canvas.height = targetH;
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    throw new Error("2d canvas context unavailable");
  }

  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = "high";
  ctx.drawImage(img, 0, 0, targetW, targetH);

  const mime = opts.preserveType ? file.type : "image/jpeg";
  const blob = await canvasToBlob(canvas, mime, opts.quality);
  if (!blob) {
    throw new Error("canvas.toBlob returned null");
  }

  const newName = opts.preserveType ? file.name : file.name.replace(/\.[a-z0-9]+$/i, "") + ".jpg";
  return new File([blob], newName, { type: mime, lastModified: Date.now() });
}

function loadImageElement(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error("Failed to decode image"));
    img.src = src;
  });
}

function canvasToBlob(canvas: HTMLCanvasElement, mime: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => {
    canvas.toBlob((b) => resolve(b), mime, quality);
  });
}
