import { useEffect, useRef } from "react";

const VAR_REGEX = /\{\{\s*([#/^]?)\s*([a-zA-Z0-9_.]+)\s*\}\}/g;

/**
 * Hook para highlighting de variáveis via DOM (substituto nativo do plugin ProseMirror).
 *
 * Funciona em qualquer versão do SuperDoc, já que opera diretamente no DOM renderizado.
 *
 * Funcionalidades:
 * - Envolve {{variavel}} em spans com classe variable-highlight
 * - Click delegation: detecta clique em variável e dispara variable-clicked
 */
export function useDomVariableHighlight(
  canvasRef: React.RefObject<HTMLDivElement | null>,
  enabled: boolean,
) {
  const observerRef = useRef<MutationObserver | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    function scheduleHighlight() {
      if (!enabledRef.current) return;
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(highlightVariables, 300);
    }

    function highlightVariables() {
      const walker = document.createTreeWalker(
        canvas!,
        NodeFilter.SHOW_TEXT,
        {
          acceptNode(node) {
            // Pula nós já processados por nós
            const parent = node.parentElement;
            if (
              parent?.classList.contains("variable-highlight") ||
              parent?.hasAttribute("data-variable-key")
            ) {
              return NodeFilter.FILTER_REJECT;
            }
            // Pula conteúdo dentro de áreas editáveis (contenteditable)
            if (parent?.closest("[contenteditable]")) {
              return NodeFilter.FILTER_REJECT;
            }
            const text = node.textContent || "";
            // Reseta o regex para cada nó
            VAR_REGEX.lastIndex = 0;
            return VAR_REGEX.test(text)
              ? NodeFilter.FILTER_ACCEPT
              : NodeFilter.FILTER_SKIP;
          },
        },
      );

      const replacements: Array<{
        node: Text;
        start: number;
        end: number;
        key: string;
        marker: string;
        text: string;
      }> = [];

      let node: Text | null;
      while ((node = walker.nextNode() as Text | null)) {
        const text = node.textContent || "";
        VAR_REGEX.lastIndex = 0;
        let match;
        while ((match = VAR_REGEX.exec(text)) !== null) {
          const marker = match[1];
          const key = match[2];
          replacements.push({
            node,
            start: match.index,
            end: match.index + match[0].length,
            key,
            marker,
            text: match[0],
          });
        }
      }

      // Aplica replacements de trás pra frente (preserva índices)
      for (let i = replacements.length - 1; i >= 0; i--) {
        const { node: textNode, start, end, key, marker, text } = replacements[i];
        try {
          const range = document.createRange();
          range.setStart(textNode, start);
          range.setEnd(textNode, end);

          const span = document.createElement("span");
          span.className = "variable-highlight";
          span.setAttribute("data-variable-key", key);
          span.setAttribute("data-variable-marker", marker);
          span.textContent = text;

          range.deleteContents();
          range.insertNode(span);
        } catch {
          // Nó pode ter sido removido durante as operações anteriores
        }
      }
    }

    // Click delegation
    function handleClick(e: MouseEvent) {
      const target = e.target as HTMLElement;
      const wrapper = target.closest("[data-variable-key]") as HTMLElement | null;
      if (!wrapper) return;

      const key = wrapper.getAttribute("data-variable-key");
      if (!key) return;
      const marker = wrapper.getAttribute("data-variable-marker") || "";
      const sourceTag = wrapper.textContent || `{{${marker}${key}}}`;

      const occurrences = Array.from(
        canvas!.querySelectorAll<HTMLElement>("[data-variable-key]"),
      ).filter(
        (element) =>
          element.getAttribute("data-variable-key") === key &&
          (element.getAttribute("data-variable-marker") || "") === marker &&
          element.textContent === sourceTag,
      );
      const occurrenceIndex = Math.max(0, occurrences.indexOf(wrapper));

      window.dispatchEvent(
        new CustomEvent("variable-clicked", {
          detail: { key, marker, pos: occurrenceIndex, sourceTag },
        }),
      );
    }

    // Observer
    observerRef.current = new MutationObserver(() => {
      scheduleHighlight();
    });

    observerRef.current.observe(canvas, {
      childList: true,
      subtree: true,
      characterData: true,
    });

    canvas.addEventListener("click", handleClick);

    // Highlight inicial
    setTimeout(highlightVariables, 500);

    return () => {
      if (observerRef.current) observerRef.current.disconnect();
      if (timerRef.current) clearTimeout(timerRef.current);
      canvas.removeEventListener("click", handleClick);
    };
  }, [canvasRef]);

  // Força re-highlight quando enabled muda (ex: novo template carregado)
  useEffect(() => {
    if (!enabled || !canvasRef.current) return;
    const timer = setTimeout(() => {
      // Força re-scan removendo todos os spans anteriores
      const existing = canvasRef.current!.querySelectorAll("[data-variable-key]");
      existing.forEach((el) => {
        const parent = el.parentNode;
        if (parent) {
          parent.replaceChild(document.createTextNode(el.textContent || ""), el);
        }
      });
      // Agenda novo highlight
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        // Re-dispara via observer
        const canvas = canvasRef.current;
        if (canvas) {
          // Trigger re-scan forçando uma mutação
          const dummy = document.createTextNode("");
          canvas.appendChild(dummy);
          canvas.removeChild(dummy);
        }
      }, 100);
    }, 200);
    return () => clearTimeout(timer);
  }, [enabled, canvasRef]);
}
