// Regra canônica de telefone brasileiro: `src/shared/utils/phone.ts`, que
// espelha `pkg/validation/phone.go`. Este módulo mantém o caminho histórico de
// importação (app/lib/leads/validation) para os formulários e para os testes,
// sem duplicar a regra — duas cópias divergentes foram exatamente o que deixou
// o formulário aceitar um telefone que a API recusava.

export {
  normalizeBRPhone,
  phoneErrorMessage,
  phoneValidationError,
} from "../../../shared/utils/phone";
export type {
  NormalizedPhone,
  InvalidPhone,
  PhoneIssue,
  PhoneNormalizationResult,
} from "../../../shared/utils/phone";
