import { getEnv } from "../lib/runtime-env"

const IMGPROXY_URL = getEnv("VITE_IMGPROXY_URL")

export interface ImgproxyOptions {
  width?: number
  height?: number
  quality?: number
  resizingType?: "fill" | "fit" | "crop" | "pad"
}

export function extractS3Path(rawUrl: string): string | null {
  if (!rawUrl) return null
  if (rawUrl.startsWith("data:") || rawUrl.startsWith("blob:")) return null
  if (rawUrl.endsWith(".svg") || rawUrl.includes(".svg?")) return null
  if (rawUrl.includes("toca-documents")) return null

  const MINIO_HOSTS = ["minio:9000", "localhost:9000", "127.0.0.1:9000"]

  try {
    const base = rawUrl.startsWith("/") ? `http://placeholder${rawUrl}` : rawUrl
    const parsed = new URL(base)
    const hostPort = `${parsed.hostname}${parsed.port ? ":" + parsed.port : ""}`

    if (MINIO_HOSTS.includes(hostPort)) {
      const parts = parsed.pathname.split("/").filter(Boolean)
      if (parts.length >= 2) {
        const [bucket, ...keyParts] = parts
        const key = keyParts.map((p) => encodeURIComponent(p)).join("/")
        return `s3://${bucket}/${key}`
      }
    }

    const uploadsMatch = parsed.pathname.match(/^\/uploads\/([^/]+)\/(.+)$/)
    if (uploadsMatch) {
      const key = uploadsMatch[2].split("/").map((p) => encodeURIComponent(p)).join("/")
      return `s3://${uploadsMatch[1]}/${key}`
    }
  } catch {
    return null
  }

  return null
}

export function getImgproxyUrl(rawUrl: string, options: ImgproxyOptions = {}): string {
  if (!rawUrl || !IMGPROXY_URL) return rawUrl

  const s3Path = extractS3Path(rawUrl)
  if (!s3Path) return rawUrl

  const { width = 0, height = 0, quality = 80, resizingType = "fill" } = options
  const base = IMGPROXY_URL.replace(/\/$/, "")
  const opts = `rs:${resizingType}:${width}:${height}/q:${quality}`

  return `${base}/insecure/${opts}/plain/${s3Path}@webp`
}
