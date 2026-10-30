import { useEffect, useState, type ReactNode } from "react";
import { Button } from "../../ui/button";

/** A failed read must never turn defaults into an editable, saveable snapshot. */
export function useSettingsLoad(load: () => Promise<void>) {
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<{ attempt: number; error?: boolean }>();
  useEffect(() => {
    let active = true;
    void load().then(
      () => { if (active) setResult({ attempt }); },
      () => { if (active) setResult({ attempt, error: true }); },
    );
    return () => { active = false; };
  }, [load, attempt]);
  const current = result?.attempt === attempt ? result : undefined;
  const ready = !!current && !current.error;
  return {
    ready,
    error: !!current?.error,
    retry: () => setAttempt(value => value + 1),
    requireReady: () => {
      if (!ready) throw new Error("Carregue as configurações antes de salvar.");
    },
  };
}

export function SettingsLoadState({ error, retry }: { error: boolean; retry: () => void }) {
  return error ? (
    <div role="alert" className="rounded-xl border border-status-critical/30 bg-status-critical-soft p-4 space-y-3">
      <p>Não foi possível carregar as configurações. Tente novamente para editar com segurança.</p>
      <Button variant="outline" onClick={retry}>Tentar novamente</Button>
    </div>
  ) : <p role="status" className="p-4 text-muted-foreground">Carregando configurações…</p>;
}

export function UnavailableSetting({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-muted/40 p-5 space-y-2">
      <p className="text-sm font-semibold">{title} <span className="text-caption text-muted-foreground">— Em breve</span></p>
      <p className="text-sm text-muted-foreground">{children}</p>
    </div>
  );
}
