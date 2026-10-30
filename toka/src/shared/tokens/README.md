# Design system operacional

Implementação canônica: `src/shared/tokens/{index,types}.ts`, componentes em
`src/shared/ui`, tema em `src/styles/theme.css` e variáveis em
`src/styles/tailwind.css`. `src/app` contém compatibilidade; código novo não deve
importar essa árvore.

## Uso

- Marca: `primary`, `accent` e escala `primary-*`, configurados pelo `ThemeContext`.
- Superfícies: `bg-background`, `bg-card`, `bg-muted`, `text-foreground`,
  `text-muted-foreground`, `border-border`.
- Estados: `status-ok`, `status-warning`, `status-pending`, `status-critical`,
  `status-info`, `status-neutral`, com variantes `-soft` e `-foreground`.
  O significado do estado permanece o mesmo quando a marca muda.
- Tipografia: `text-micro`, `text-caption`, `text-label`, `text-title`,
  `text-title-lg`, `text-section`, além da escala Tailwind. Confira também peso
  e entrelinha ao migrar: os tokens semânticos alteram mais que o tamanho.
- Use `Button`, `Dialog`, `ConfirmDialog`, `Badge` e `ResponsiveTable` compartilhados.
  Confirmações assíncronas podem usar `useConfirmAction`; aguardam a operação,
  bloqueiam repetição e permanecem abertas em caso de erro.
- Cada controle precisa de nome acessível. `FieldRow` fornece o nome aos
  controles compartilhados de configurações. Ações de tabela têm botão de
  abertura para teclado. Diálogos devem devolver o foco ao acionador.
- Carregamento, erro com tentativa novamente, vazio e sucesso são estados
  distintos. Nunca mostre sucesso para uma operação indisponível.

## Validação e migração

Execute `pnpm lint` em `toka`. As baselines de qualidade e design em `scripts/`
registram dívida existente por arquivo/regra e bloqueiam aumento. Elas não
significam conformidade integral ou aprovação de acessibilidade. Não aumente a
baseline automaticamente para fazer CI passar; reduza-a ao corrigir ocorrências.
`pnpm lint:quality` verifica hooks e JSX acessível; `pnpm lint:design` verifica
cores de paleta, fontes arbitrárias e confirmações nativas.

Impressão (ex.: recibo), gráficos e materiais de marca podem exigir cores fixas;
essas exceções precisam ser verificadas no consumidor. Antes de migrar outro
fluxo, valide claro/escuro, marca alternativa, teclado, larguras 360/768/1440 e
zoom 200%. A baseline não substitui essa inspeção.

O tema é gerenciado por `ThemeContext`, incluindo a classe `.dark` no documento.
Não adicione um segundo provider `next-themes`. O inventário histórico e o plano
remanescente estão em `docs/diagnostics/2026-09-26-consistencia-qualidade-ux.md`
na raiz do repositório.
