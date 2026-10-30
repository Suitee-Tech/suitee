import { useCallback, useRef } from "react";
import type { MouseEvent as ReactMouseEvent, TouchEvent as ReactTouchEvent } from "react";

type LongPressOptions = {
  /** Tempo (ms) que o toque precisa permanecer parado para contar como long-press. */
  delay?: number;
  /** Movimento máximo (px) tolerado antes de o gesto virar rolagem/arrasto. */
  moveTolerance?: number;
};

/**
 * Detector de long-press para telas de toque.
 *
 * Devolve handlers para espalhar no elemento-alvo; `onLongPress` dispara apenas
 * quando o toque fica (quase) parado por `delay` ms. Mover (rolagem) ou soltar
 * cancela o disparo, então o gesto nunca compete com o scroll da lista. No
 * desktop nada acontece: lá a UI já expõe hover e clique-direito.
 *
 * O clique sintético que o navegador emite depois do long-press é suprimido na
 * fase de captura para não acionar também os cliques internos da mensagem
 * (ex.: abrir um anexo).
 */
export function useLongPress(onLongPress: () => void, options: LongPressOptions = {}) {
  const { delay = 450, moveTolerance = 10 } = options;

  const timer = useRef<number | null>(null);
  const origin = useRef<{ x: number; y: number } | null>(null);
  const fired = useRef(false);
  const suppressClickUntil = useRef(0);

  const clearTimer = useCallback(() => {
    if (timer.current !== null) {
      window.clearTimeout(timer.current);
      timer.current = null;
    }
  }, []);

  const onTouchStart = useCallback(
    (event: ReactTouchEvent<HTMLElement>) => {
      if (event.touches.length !== 1) return;
      const touch = event.touches[0];
      origin.current = { x: touch.clientX, y: touch.clientY };
      fired.current = false;
      clearTimer();
      timer.current = window.setTimeout(() => {
        timer.current = null;
        fired.current = true;
        // Janela curta para engolir o clique que o navegador emite no touchend.
        suppressClickUntil.current = Date.now() + 700;
        onLongPress();
      }, delay);
    },
    [clearTimer, delay, onLongPress],
  );

  const onTouchMove = useCallback(
    (event: ReactTouchEvent<HTMLElement>) => {
      if (!origin.current || event.touches.length !== 1) return;
      const touch = event.touches[0];
      if (
        Math.abs(touch.clientX - origin.current.x) > moveTolerance ||
        Math.abs(touch.clientY - origin.current.y) > moveTolerance
      ) {
        clearTimer();
      }
    },
    [clearTimer, moveTolerance],
  );

  const onTouchEnd = useCallback(() => {
    clearTimer();
    origin.current = null;
  }, [clearTimer]);

  const onClickCapture = useCallback((event: ReactMouseEvent<HTMLElement>) => {
    if (fired.current || Date.now() < suppressClickUntil.current) {
      fired.current = false;
      event.preventDefault();
      event.stopPropagation();
    }
  }, []);

  return {
    onTouchStart,
    onTouchMove,
    onTouchEnd,
    onTouchCancel: onTouchEnd,
    onClickCapture,
  };
}
