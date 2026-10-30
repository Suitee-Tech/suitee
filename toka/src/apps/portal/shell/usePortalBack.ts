import { useCallback } from "react";
import { useNavigate } from "react-router";

/**
 * Voltar das páginas internas do portal.
 *
 * Regra única: se existe uma entrada anterior DENTRO do app
 * (`history.state.idx > 0`), volta para onde a pessoa estava; senão (link
 * direto, aba nova, recarga) vai para o destino informado.
 *
 * Antes cada tela decidia sozinha — umas iam para `/portal`, outras para
 * `/portal/menu`, outras usavam `navigate(-1)` — e o resultado percebido era
 * "o botão de voltar não volta".
 */
export function usePortalBack(fallbackTo = "/portal") {
  const navigate = useNavigate();
  return useCallback(() => {
    const idx = typeof window !== "undefined" ? Number((window.history.state as { idx?: number } | null)?.idx ?? 0) : 0;
    if (idx > 0) {
      navigate(-1);
      return;
    }
    navigate(fallbackTo);
  }, [fallbackTo, navigate]);
}
