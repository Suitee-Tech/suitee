import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { documentService, type UpdateDocumentDTO } from "../services/document.service";
import { useDocumentAutoSave } from "./useDocumentAutoSave";

vi.mock("../services/document.service", () => ({
  documentService: {
    update: vi.fn(),
  },
}));

type Deferred = {
  promise: Promise<unknown>;
  resolve: (value?: unknown) => void;
};

function deferred(): Deferred {
  let resolve!: (value?: unknown) => void;
  const promise = new Promise<unknown>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

async function advanceDebounce() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(600);
  });
}

describe("useDocumentAutoSave", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("coalesces drafts received during a save and flushes only the latest one", async () => {
    const firstSave = deferred();
    vi.mocked(documentService.update)
      .mockImplementationOnce(() => firstSave.promise as ReturnType<typeof documentService.update>)
      .mockResolvedValueOnce({} as Awaited<ReturnType<typeof documentService.update>>);

    const baseline: UpdateDocumentDTO = { notes: "original" };
    const onSaved = vi.fn();
    const { rerender, result } = renderHook(
      ({ draft }: { draft: UpdateDocumentDTO }) =>
        useDocumentAutoSave({
          docId: "doc-1",
          baseline,
          draft,
          onSaved,
        }),
      { initialProps: { draft: { notes: "primeiro" } } },
    );

    await advanceDebounce();
    expect(documentService.update).toHaveBeenCalledTimes(1);
    expect(documentService.update).toHaveBeenLastCalledWith("doc-1", { notes: "primeiro" });
    expect(result.current.status).toBe("saving");

    rerender({ draft: { notes: "intermediario" } });
    await advanceDebounce();
    rerender({ draft: { notes: "mais recente" } });
    await advanceDebounce();

    expect(documentService.update).toHaveBeenCalledTimes(1);

    await act(async () => {
      firstSave.resolve();
      await firstSave.promise;
      await Promise.resolve();
    });

    expect(documentService.update).toHaveBeenCalledTimes(2);
    expect(documentService.update).toHaveBeenLastCalledWith("doc-1", { notes: "mais recente" });
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(result.current.status).toBe("saved");
    expect(result.current.error).toBeNull();
  });

  it("waits for a newer draft before reconciling an intermediate save", async () => {
    const firstSave = deferred();
    vi.mocked(documentService.update)
      .mockImplementationOnce(() => firstSave.promise as ReturnType<typeof documentService.update>)
      .mockResolvedValueOnce({} as Awaited<ReturnType<typeof documentService.update>>);

    const baseline: UpdateDocumentDTO = { notes: "original" };
    const onSaved = vi.fn();
    const { rerender, result } = renderHook(
      ({ draft }: { draft: UpdateDocumentDTO }) =>
        useDocumentAutoSave({
          docId: "doc-1",
          baseline,
          draft,
          onSaved,
        }),
      { initialProps: { draft: { notes: "primeiro" } } },
    );

    await advanceDebounce();
    rerender({ draft: { notes: "mais recente" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });

    await act(async () => {
      firstSave.resolve();
      await firstSave.promise;
    });

    expect(documentService.update).toHaveBeenCalledTimes(1);
    expect(onSaved).not.toHaveBeenCalled();
    expect(result.current.status).toBe("idle");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });

    expect(documentService.update).toHaveBeenCalledTimes(2);
    expect(documentService.update).toHaveBeenLastCalledWith("doc-1", { notes: "mais recente" });
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(result.current.status).toBe("saved");
  });

  it("persists a draft that returns to the baseline while another value is in flight", async () => {
    const firstSave = deferred();
    vi.mocked(documentService.update)
      .mockImplementationOnce(() => firstSave.promise as ReturnType<typeof documentService.update>)
      .mockResolvedValueOnce({} as Awaited<ReturnType<typeof documentService.update>>);

    const baseline: UpdateDocumentDTO = { notes: "original" };
    const { rerender } = renderHook(
      ({ draft }: { draft: UpdateDocumentDTO }) =>
        useDocumentAutoSave({
          docId: "doc-1",
          baseline,
          draft,
        }),
      { initialProps: { draft: { notes: "alterado" } } },
    );

    await advanceDebounce();
    rerender({ draft: { notes: "original" } });
    await advanceDebounce();

    await act(async () => {
      firstSave.resolve();
      await firstSave.promise;
      await Promise.resolve();
    });

    expect(documentService.update).toHaveBeenCalledTimes(2);
    expect(documentService.update).toHaveBeenNthCalledWith(1, "doc-1", { notes: "alterado" });
    expect(documentService.update).toHaveBeenNthCalledWith(2, "doc-1", baseline);
  });

  it("does not publish lifecycle updates or flush queued work after unmount", async () => {
    const firstSave = deferred();
    vi.mocked(documentService.update).mockImplementationOnce(
      () => firstSave.promise as ReturnType<typeof documentService.update>,
    );
    const onSaved = vi.fn();
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    const baseline: UpdateDocumentDTO = { notes: "original" };
    const { rerender, unmount } = renderHook(
      ({ draft }: { draft: UpdateDocumentDTO }) =>
        useDocumentAutoSave({
          docId: "doc-1",
          baseline,
          draft,
          onSaved,
        }),
      { initialProps: { draft: { notes: "alterado" } } },
    );

    await advanceDebounce();
    rerender({ draft: { notes: "pendente" } });
    await advanceDebounce();
    unmount();

    await act(async () => {
      firstSave.resolve();
      await firstSave.promise;
      await Promise.resolve();
    });

    expect(documentService.update).toHaveBeenCalledTimes(1);
    expect(onSaved).not.toHaveBeenCalled();
    expect(consoleError).not.toHaveBeenCalled();
  });

  it("preserves a failed local draft when a stale server baseline refetches", async () => {
    vi.mocked(documentService.update)
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({} as Awaited<ReturnType<typeof documentService.update>>);

    const { result, rerender } = renderHook(
      ({ baseline, draft }: { baseline: UpdateDocumentDTO; draft: UpdateDocumentDTO }) =>
        useDocumentAutoSave({
          docId: "doc-1",
          baseline,
          draft,
        }),
      {
        initialProps: {
          baseline: { notes: "original" },
          draft: { notes: "alterado" },
        },
      },
    );

    await advanceDebounce();
    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe("network");

    rerender({
      baseline: { notes: "original" },
      draft: { notes: "alterado" },
    });

    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe("network");

    await advanceDebounce();
    expect(documentService.update).toHaveBeenCalledTimes(2);
    expect(documentService.update).toHaveBeenLastCalledWith("doc-1", { notes: "alterado" });
    expect(result.current.status).toBe("saved");
  });
});
