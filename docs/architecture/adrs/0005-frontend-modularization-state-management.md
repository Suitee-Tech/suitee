# ADR 0005 — Modularização do frontend e estratégia de estado

- **Status**: Proposed
- **Data**: 2026-05-30
- **Owners**: (a definir)
- **RFC relacionada**: rfc-frontend-refactor-toka.md

## Contexto
O frontend `toka` (Vite/React) cresceu com um “centro” muito carregado de responsabilidades:

- Rotas, guards e permissões concentradas e repetitivas em [routes.tsx](../../../toka/src/app/routes.tsx).
- Layout com múltiplos concerns e filtragem de menu por permissões em [Layout.tsx](../../../toka/src/app/components/Layout.tsx).
- Provedores globais empilhados em [App.tsx](../../../toka/src/app/App.tsx#L16-L41).
- Estado de CRM então concentrado em `toka/src/app/context/CRMContext.tsx`, arquivo histórico posteriormente removido durante a modularização.

Além disso, bibliotecas grandes (PDF/DOCX, mapas, MUI, etc.) podem entrar cedo no bundle por imports eager em componentes como [DocumentViewerDialog.tsx](../../../toka/src/app/components/DocumentViewerDialog.tsx#L14-L30) e [PropertyMap.tsx](../../../toka/src/app/components/PropertyMap.tsx#L1-L6).

Isso resulta em:
- manutenção difícil (mudanças exigem tocar múltiplas pastas por tipo),
- inconsistência potencial entre menu/rotas/permissões,
- maior risco de regressão e menor previsibilidade,
- custo de render e bundle inicial acima do necessário.

## Decisão
Adotar uma evolução incremental do frontend com três pilares:

1. **Organização por “apps” e por feature**
   - Separar “admin” e “portal” em subárvores (`src/apps/admin` e `src/apps/portal`) com providers e rotas próprias.
   - Mover gradualmente o código para `src/features/<feature>/...` (feature-first), com `src/shared` para componentes e utilidades comuns.

2. **Server state como padrão e UI state granular**
   - Usar React Query como fonte da verdade para dados remotos sempre que aplicável.
   - Evitar Contexts monolíticos; preferir contexts menores por domínio ou store com selectors, reduzindo re-render em cascata.

3. **Uma única fonte para navegação/permissões + code-splitting por bordas**
   - Definir metadados de rota em um route registry tipado por app, usado tanto para gerar o router quanto o menu.
   - Bibliotecas pesadas devem ser carregadas de forma lazy/condicional (viewer de PDF/DOCX, mapas, editores).

## Alternativas consideradas
1. **Manter organização atual e só “limpar” arquivos grandes**
   - Pros: menor mudança estrutural imediata.
   - Cons: não resolve acoplamento entre tipos, não reduz blast radius de estado, e tende a reincidir.

2. **Migrar para micro-frontends**
   - Pros: isolamento forte por domínio.
   - Cons: overhead operacional e de arquitetura alto para o momento; complexidade de deploy/integração.

3. **Reescrever o frontend**
   - Pros: liberdade total.
   - Cons: alto risco de perda de funcionalidades e longo período de divergência; não atende o requisito de preservar características atuais.

## Consequências
### Aceitas
- Melhor encapsulamento de mudanças e redução de imports cruzados.
- Estado mais previsível, com menor custo de re-render e menos efeitos em cascata.
- Permissões e navegação mais consistentes (menu e rotas alinhados).
- Melhor tempo de carregamento inicial ao evitar carregar features raras cedo.

### Pagas
- Período de migração com convivência de padrões (até que features sejam movidas).
- Necessidade de disciplina de revisão (PRs precisam respeitar boundaries e convenções novas).
- Trabalho inicial para criar estrutura base, route registry e “infra” compartilhada de HTTP.

## Quando reconsiderar
- Se a separação “apps” (admin/portal) impedir compartilhamentos legítimos de UI/fluxos, rever boundaries.
- Se o custo de manutenção de route registry não compensar (ex.: rotas extremamente dinâmicas), rever o grau de centralização de metadados.
- Se o store/contexts menores ainda não reduzir o blast radius do CRM, reavaliar a estratégia (incluindo mudanças maiores de arquitetura de estado).
