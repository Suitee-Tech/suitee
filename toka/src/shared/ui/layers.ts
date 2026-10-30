/**
 * Escala de camadas (z-index) da aplicação.
 *
 * Existe porque as camadas estavam espalhadas e sem contrato: painéis laterais
 * entre 50 e 200, menus flutuantes em 50, e o diálogo de confirmação — que é
 * sempre a última coisa que o usuário precisa ver — também em 50. Resultado
 * relatado em 2026-10-05: "cancelar uma ordem de pagamento não é possível pois o
 * alert de confirmação fica abaixo do painel lateral".
 *
 * A regra é a ordem de leitura da tela:
 *
 *   1. `panel` (50)       — superfícies que abrem a ação: ExpandableSidePanel,
 *                           Sheet, Drawer;
 *   2. `panelAction` (60) — painel de ação por cima delas (FinancialActionPanel),
 *                           porque a lista que disparou a ação continua atrás;
 *   3. `panelTop` (200)   — painel que por decisão própria fica acima de outros
 *                           painéis (BrokerCapabilitiesPanel hoje);
 *   4. `dialog` (300)     — QUALQUER diálogo (Dialog e AlertDialog/ConfirmDialog)
 *                           precisa ficar acima de QUALQUER painel: a
 *                           confirmação é a última decisão antes do efeito;
 *   5. `floating` (400)   — menus flutuantes (Popover, Select, DropdownMenu,
 *                           ContextMenu, Tooltip, HoverCard). Ficam acima dos
 *                           diálogos porque precisam funcionar DENTRO deles.
 *
 * Os valores em classe Tailwind são literais nos primitivos (o Tailwind varre o
 * código-fonte, não aceita interpolação) e cada um exporta a sua constante de
 * camada, usada pelo teste `layers.test.tsx` para provar a ordem. Mudar um
 * número exige mudar o outro — não há como a escala voltar a se cruzar em
 * silêncio.
 */
export const LAYER = {
  panel: 50,
  panelAction: 60,
  panelTop: 200,
  dialog: 300,
  floating: 400,
  floatingTop: 420,
} as const;

/** Extrai o número de uma classe de camada (`z-50`, `z-[300]`) — usado nos testes. */
export function layerOf(className: string | null | undefined): number | null {
  if (!className) return null;
  const arbitrary = className.match(/z-\[(\d+)\]/);
  if (arbitrary) return Number(arbitrary[1]);
  const named = className.match(/\bz-(\d+)\b/);
  return named ? Number(named[1]) : null;
}
