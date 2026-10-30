# Compatibilidade de tokens

Esta árvore preserva imports antigos. A implementação e as instruções atuais
estão em [src/shared/tokens](../../shared/tokens/README.md).

Novos consumidores devem importar `@/shared/tokens` e `@/shared/ui`.
A migração visual ainda tem ocorrências pendentes, inventariadas nos arquivos
`scripts/design-system-baseline.json` e `scripts/code-quality-baseline.json`.
Execute `pnpm lint` em `toka` para verificar regressões.
