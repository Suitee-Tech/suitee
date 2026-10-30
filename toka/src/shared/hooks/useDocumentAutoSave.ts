import { useCallback, useEffect, useRef, useState } from "react";
import { documentService, type UpdateDocumentDTO } from "../services/document.service";

const DEBOUNCE_MS = 600;

export type AutoSaveStatus = "idle" | "saving" | "saved" | "error";

interface UseDocumentAutoSaveArgs {
  docId: string | null | undefined;
  draft: UpdateDocumentDTO | null;
  baseline?: UpdateDocumentDTO | null;
  onSaved?: () => void;
  paused?: boolean;
}

type QueuedSave = {
  docId: string;
  payload: UpdateDocumentDTO;
  documentVersion: number;
};

function isEqual(a: UpdateDocumentDTO | null, b: UpdateDocumentDTO | null): boolean {
  if (!a && !b) return true;
  if (!a || !b) return false;
  return JSON.stringify(a) === JSON.stringify(b);
}

export function useDocumentAutoSave({
  docId,
  draft,
  baseline,
  onSaved,
  paused,
}: UseDocumentAutoSaveArgs): { status: AutoSaveStatus; savedAt: number | null; error: string | null } {
  const [status, setStatus] = useState<AutoSaveStatus>("idle");
  const [savedAt, setSavedAt] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  const lastSavedRef = useRef<UpdateDocumentDTO | null>(baseline ?? null);
  const lastBaselineRef = useRef<UpdateDocumentDTO | null>(baseline ?? null);
  const inFlightRef = useRef<QueuedSave | null>(null);
  const pendingRef = useRef<QueuedSave | null>(null);
  const documentVersionRef = useRef(0);
  const previousDocIdRef = useRef(docId);
  const currentDocIdRef = useRef(docId);
  const latestDraftRef = useRef(draft);
  const pausedRef = useRef(Boolean(paused));
  const onSavedRef = useRef(onSaved);

  currentDocIdRef.current = docId;
  latestDraftRef.current = draft;
  pausedRef.current = Boolean(paused);
  onSavedRef.current = onSaved;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      pendingRef.current = null;
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    const documentChanged = previousDocIdRef.current !== docId;
    if (documentChanged) {
      previousDocIdRef.current = docId;
      documentVersionRef.current += 1;
      pendingRef.current = null;
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    }

    const nextBaseline = baseline ?? null;
    const currentDraft = latestDraftRef.current;
    const localDraftIsDirty = !isEqual(currentDraft, lastBaselineRef.current);
    const serverAcknowledgedDraft = isEqual(currentDraft, nextBaseline);

    if (!documentChanged && localDraftIsDirty && !serverAcknowledgedDraft) {
      return;
    }

    lastBaselineRef.current = nextBaseline;
    lastSavedRef.current = nextBaseline;
    if (mountedRef.current && !inFlightRef.current && !pendingRef.current) {
      setStatus("idle");
      setError(null);
    }
  }, [docId, baseline]);

  const flushPending = useCallback(async function drainQueue(): Promise<void> {
    if (!mountedRef.current || pausedRef.current || inFlightRef.current) return;

    const queued = pendingRef.current;
    if (!queued) return;
    pendingRef.current = null;

    const isCurrentDocument = () =>
      mountedRef.current &&
      queued.documentVersion === documentVersionRef.current &&
      queued.docId === currentDocIdRef.current;

    if (!isCurrentDocument()) return;

    if (isEqual(queued.payload, lastSavedRef.current)) {
      setStatus("saved");
      setError(null);
      return;
    }

    inFlightRef.current = queued;
    setStatus("saving");
    setError(null);

    try {
      await documentService.update(queued.docId, queued.payload);
      if (isCurrentDocument()) {
        lastSavedRef.current = queued.payload;
        const hasNewerDraft =
          latestDraftRef.current !== null &&
          !isEqual(latestDraftRef.current, queued.payload);
        setSavedAt(Date.now());
        setStatus(hasNewerDraft ? "idle" : "saved");
        setError(null);
        if (!hasNewerDraft) {
          onSavedRef.current?.();
        }
      }
    } catch (err) {
      if (isCurrentDocument()) {
        const msg = err instanceof Error ? err.message : String(err);
        setStatus("error");
        setError(msg);
      }
    } finally {
      inFlightRef.current = null;
      if (mountedRef.current && !pausedRef.current && pendingRef.current) {
        void drainQueue();
      }
    }
  }, []);

  useEffect(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }

    const documentVersion = documentVersionRef.current;
    const pendingSave = pendingRef.current;
    if (
      pendingSave &&
      pendingSave.docId === docId &&
      pendingSave.documentVersion === documentVersion
    ) {
      pendingRef.current = null;
    }

    if (paused || !docId || !draft) return;

    const activeSave = inFlightRef.current;
    const activePayload =
      activeSave?.docId === docId && activeSave.documentVersion === documentVersion
        ? activeSave.payload
        : null;
    const expectedPersistedPayload = activePayload ?? lastSavedRef.current;
    if (isEqual(draft, expectedPersistedPayload)) return;

    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      if (
        !mountedRef.current ||
        pausedRef.current ||
        currentDocIdRef.current !== docId ||
        documentVersionRef.current !== documentVersion
      ) {
        return;
      }

      pendingRef.current = {
        docId,
        payload: draft,
        documentVersion,
      };
      void flushPending();
    }, DEBOUNCE_MS);

    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [baseline, docId, draft, flushPending, paused]);

  return { status, savedAt, error };
}
