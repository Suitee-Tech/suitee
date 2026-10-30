import { useEffect, useState } from "react";

/**
 * Altura, em pixels, da área coberta pelo teclado virtual do celular.
 *
 * Por que existe: no mobile o teclado NÃO encolhe a viewport de layout no iOS —
 * só a *visual viewport*. Um dropdown ancorado ao campo (Radix Popover) é
 * posicionado contra a viewport de layout, então ele abria por baixo do campo
 * exatamente na faixa coberta pelo teclado e as opções ficavam intocáveis
 * ("o teclado cobre a área impedindo a seleção").
 *
 * Devolve 0 quando não há teclado (equivalente à viewport de layout) — nesse
 * caso o consumidor mantém o posicionamento padrão, sem nenhuma mudança.
 */
export function useVisualViewportInset(): number {
  const [inset, setInset] = useState(0);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const visualViewport = window.visualViewport;
    if (!visualViewport) return;

    const update = () => {
      // A viewport visual pode estar deslocada (offsetTop) quando o navegador
      // rola a página para o campo focado: a parte coberta é o que sobra entre
      // o fim da viewport visual e o fim da viewport de layout.
      const hidden = window.innerHeight - visualViewport.height - visualViewport.offsetTop;
      setInset(Math.max(0, Math.round(hidden)));
    };

    update();
    visualViewport.addEventListener("resize", update);
    visualViewport.addEventListener("scroll", update);
    return () => {
      visualViewport.removeEventListener("resize", update);
      visualViewport.removeEventListener("scroll", update);
    };
  }, []);

  return inset;
}

/**
 * Altura visível real da tela (viewport de layout menos a faixa coberta pelo
 * teclado virtual). É a medida que um seletor deve usar para se posicionar e
 * limitar a própria altura no celular.
 */
export function useVisibleViewportHeight(): number {
  const inset = useVisualViewportInset();
  const [layoutHeight, setLayoutHeight] = useState(() =>
    typeof window === "undefined" ? 0 : window.innerHeight,
  );

  useEffect(() => {
    if (typeof window === "undefined") return;
    const update = () => setLayoutHeight(window.innerHeight);
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, []);

  return Math.max(0, layoutHeight - inset);
}

/**
 * Tela pequena (celular). Usada quando a apresentação do componente precisa ser
 * diferente: um dropdown flutuante ancorado ao campo é frágil com o teclado
 * aberto, enquanto um painel fixo no topo da área visível nunca é coberto.
 */
export function useIsSmallScreen(query = "(max-width: 639px)"): boolean {
  // Ambientes sem matchMedia (jsdom, SSR) ficam no comportamento de tela grande:
  // é o mesmo caminho usado no desktop, sem alterar a apresentação testada.
  const supportsMatchMedia = () =>
    typeof window !== "undefined" && typeof window.matchMedia === "function";

  const [matches, setMatches] = useState(() =>
    supportsMatchMedia() ? window.matchMedia(query).matches : false,
  );

  useEffect(() => {
    if (!supportsMatchMedia()) return;
    const media = window.matchMedia(query);
    const update = () => setMatches(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [query]);

  return matches;
}
