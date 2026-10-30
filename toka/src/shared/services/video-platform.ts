export type VideoPlatform = "youtube" | "vimeo" | "other";

export function detectVideoPlatform(url: string): VideoPlatform {
  const u = (url || "").toLowerCase();
  if (u.includes("youtube.com") || u.includes("youtu.be")) return "youtube";
  if (u.includes("vimeo.com")) return "vimeo";
  return "other";
}

export function extractVideoId(url: string): { platform: VideoPlatform; id: string | null } {
  const platform = detectVideoPlatform(url);
  if (platform === "youtube") {
    const patterns = [
      /youtube\.com\/watch\?v=([^&]+)/i,
      /youtu\.be\/([^?&/]+)/i,
      /youtube\.com\/shorts\/([^?&/]+)/i,
      /youtube\.com\/embed\/([^?&/]+)/i,
    ];
    for (const p of patterns) {
      const m = url.match(p);
      if (m?.[1]) return { platform, id: m[1] };
    }
    return { platform, id: null };
  }

  if (platform === "vimeo") {
    const patterns = [/vimeo\.com\/(\d+)/i, /player\.vimeo\.com\/video\/(\d+)/i];
    for (const p of patterns) {
      const m = url.match(p);
      if (m?.[1]) return { platform, id: m[1] };
    }
    return { platform, id: null };
  }

  return { platform: "other", id: null };
}

export function getVideoEmbedUrl(v: { url: string; platform?: string }): string {
  const platform = (v.platform as VideoPlatform | undefined) || detectVideoPlatform(v.url);
  const { id } = extractVideoId(v.url);

  if (platform === "youtube" && id) {
    return `https://www.youtube.com/embed/${id}`;
  }
  if (platform === "vimeo" && id) {
    return `https://player.vimeo.com/video/${id}`;
  }
  return v.url;
}
