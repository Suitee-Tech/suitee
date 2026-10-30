import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { STALE_CHUNK_COOLDOWN_MS, isStaleChunkError, navigation, reloadForStaleChunk } from "./staleChunk";

describe("staleChunk", () => {
  let reload: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    sessionStorage.clear();
    reload = vi.fn();
    // jsdom não navega de verdade; observa a chamada pelo seam `navigation`.
    vi.spyOn(navigation, "reload").mockImplementation(reload as unknown as () => void);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("reconhece as mensagens de chunk obsoleto", () => {
    expect(isStaleChunkError(new Error("Failed to fetch dynamically imported module: /assets/x.js"))).toBe(true);
    expect(isStaleChunkError(new Error("Importing a module script failed."))).toBe(true);
    expect(isStaleChunkError(new Error("Loading chunk 3 failed."))).toBe(true);
    expect(isStaleChunkError(new Error("network timeout"))).toBe(false);
    expect(isStaleChunkError(undefined)).toBe(false);
  });

  it("recarrega na primeira falha de chunk", () => {
    reloadForStaleChunk("test");
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("respeita o cooldown para não entrar em loop de reload", () => {
    reloadForStaleChunk("primeira");
    reloadForStaleChunk("segunda logo em seguida");
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("volta a recarregar depois do cooldown (aba do iOS restaurada do cache)", () => {
    vi.useFakeTimers();
    try {
      reloadForStaleChunk("primeira");
      vi.advanceTimersByTime(STALE_CHUNK_COOLDOWN_MS + 1);
      reloadForStaleChunk("depois do cooldown");
      expect(reload).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
