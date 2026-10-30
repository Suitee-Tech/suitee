import * as z from "zod";
import { type LeadSource } from "../../shared/data";
import { parseMoneyBR0 } from "../../shared/utils/format";
import { parsePhoneBR } from "../../shared/utils/masks";
import { personNameError, sanitizePersonName } from "../../shared/utils/personName";
import { phoneValidationError } from "../../shared/utils/phone";

// Schema do formulário de lead (criar e editar). Fica fora da página para ser
// testável: a regra de telefone precisa ser a MESMA da API — quando as duas
// divergem, o formulário aceita o que o servidor recusa e o operador recebe um
// erro genérico no fim do fluxo.

export type LeadFormData = {
  name: string;
  email: string;
  phone: string;
  property?: string;
  budget?: number;
  status: "Novo" | "Em Atendimento" | "Qualificado";
  source: LeadSource;
  agentId?: string;
};

export const leadSchema: z.ZodType<LeadFormData> = z.object({
  // Mesma regra da API (`pkg/validation/person_name.go`): nome de pessoa com
  // sobrenome. O lead vira a parte do negócio e um monônimo só falharia depois,
  // ao iniciar o negócio na esteira.
  name: z.preprocess(
    (value) => (typeof value === "string" ? sanitizePersonName(value) : value),
    z.string().superRefine((value, ctx) => {
      const problem = personNameError(value, "Nome");
      if (problem) ctx.addIssue({ code: z.ZodIssueCode.custom, message: problem });
    }),
  ),
  email: z.string().email("E-mail inválido"),
  // Mesma regra da API (`pkg/validation/phone.go`): fixo com DDD (10 dígitos,
  // assinante 2–5) ou celular com DDD (11 dígitos, assinante começando em 9).
  // Só contar dígitos deixava passar o celular sem um dígito — o formulário
  // aceitava e o cadastro voltava com erro genérico do servidor.
  phone: z.preprocess(
    (value) => (typeof value === "string" ? parsePhoneBR(value) : value),
    z.string().superRefine((value, ctx) => {
      const problem = phoneValidationError(value);
      if (problem) ctx.addIssue({ code: z.ZodIssueCode.custom, message: problem });
    }),
  ),
  property: z.string().optional(),
  budget: z.preprocess(
    (value) => {
      if (value === null || value === undefined) return undefined;
      if (typeof value === "string") {
        const s = value.trim();
        if (!s) return undefined;
        return parseMoneyBR0(s) ?? undefined;
      }
      return value;
    },
    z.number().int().min(0, "O orçamento não pode ser negativo").optional(),
  ),
  status: z.enum(["Novo", "Em Atendimento", "Qualificado"]),
  source: z.enum(["Site", "Indicacao", "Redes Sociais", "Manual", "Campanha", "Outros"]),
  agentId: z.string().optional(),
});
