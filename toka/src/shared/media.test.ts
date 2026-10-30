import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  getAvatarThumbUrl,
  getMediaUrl,
  normalizeStorageUrl,
  resolveMediaUrl,
} from "./media";

type RuntimeEnvGlobal = typeof globalThis & {
  __TOCA_ENV__?: Record<string, string | undefined>;
};

const runtimeGlobal = globalThis as RuntimeEnvGlobal;
const originalRuntimeEnv = runtimeGlobal.__TOCA_ENV__;

beforeEach(() => {
  runtimeGlobal.__TOCA_ENV__ = {
    VITE_SUPABASE_URL: "http://localhost:54321",
    VITE_MEDIA_URL: "http://localhost:8080",
  };
});

afterEach(() => {
  runtimeGlobal.__TOCA_ENV__ = originalRuntimeEnv;
});

describe("resolveMediaUrl", () => {
  it("keeps a newly uploaded avatar on the configured local Supabase origin", () => {
    const avatar =
      "http://localhost:54321/storage/v1/object/public/toca-avatars/avatar.png";

    expect(resolveMediaUrl(avatar)).toBe(avatar);
    expect(getMediaUrl(avatar)).toBe(avatar);
    expect(getAvatarThumbUrl(avatar, 64)).toBe(avatar);
  });

  it("rewrites an internal Docker Storage URL and preserves its signature", () => {
    const internal =
      "http://host.docker.internal:54321/storage/v1/object/sign/toca-documents/file.png?token=abc#preview";

    expect(resolveMediaUrl(internal)).toBe(
      "http://localhost:54321/storage/v1/object/sign/toca-documents/file.png?token=abc#preview",
    );
  });

  it("resolves relative Storage paths using VITE_SUPABASE_URL", () => {
    expect(
      resolveMediaUrl(
        "/storage/v1/object/public/toca-property-images/property.png?version=2",
      ),
    ).toBe(
      "http://localhost:54321/storage/v1/object/public/toca-property-images/property.png?version=2",
    );
  });

  it("preserves external HTTP media and inline previews", () => {
    expect(resolveMediaUrl("https://cdn.example.com/photo.jpg")).toBe(
      "https://cdn.example.com/photo.jpg",
    );
    expect(resolveMediaUrl("blob:http://localhost:3000/blob-id")).toBe(
      "blob:http://localhost:3000/blob-id",
    );
    expect(resolveMediaUrl("data:image/png;base64,AAAA")).toBe(
      "data:image/png;base64,AAAA",
    );
  });

  it("keeps legacy uploads on the configured media service", () => {
    expect(resolveMediaUrl("/uploads/avatars/avatar.jpg?version=3")).toBe(
      "http://localhost:8080/uploads/avatars/avatar.jpg?version=3",
    );
    expect(resolveMediaUrl("http://localhost:9000/avatars/avatar.jpg")).toBe(
      "http://localhost:8080/avatars/avatar.jpg",
    );
  });

  it("uses the environment API URL when the media URL is unavailable", () => {
    runtimeGlobal.__TOCA_ENV__ = {
      VITE_MEDIA_URL: "not-a-url",
      VITE_API_URL: "https://api.example.com/api/v1",
    };

    expect(resolveMediaUrl("/uploads/avatars/avatar.jpg")).toBe(
      "https://api.example.com/uploads/avatars/avatar.jpg",
    );
  });

  it("falls back to the current frontend origin when API configuration is relative", () => {
    runtimeGlobal.__TOCA_ENV__ = {
      VITE_MEDIA_URL: "not-a-url",
      VITE_API_URL: "/api/v1",
    };

    expect(resolveMediaUrl("/uploads/avatars/avatar.jpg")).toBe(
      `${window.location.origin}/uploads/avatars/avatar.jpg`,
    );
  });

  it.each([
    "javascript:alert(1)",
    "file:///tmp/photo.png",
    "data:text/html,<script>alert(1)</script>",
    "https://project.supabase.co/storage/v1/object/authenticated/toca-documents/private.png",
  ])("rejects a non-renderable media URL: %s", (url) => {
    expect(resolveMediaUrl(url)).toBeNull();
    expect(getMediaUrl(url)).toBe("");
  });

  it("does not mistake a Docker-looking public hostname for an internal host", () => {
    const external =
      "https://host.docker.internal.example.com/storage/v1/object/public/toca-avatars/avatar.png";
    expect(normalizeStorageUrl(external)).toBe(external);
  });
});
