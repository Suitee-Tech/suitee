import { z } from "zod";

import {
  isValidCnpj,
  isValidCpf,
  isValidCpfCnpj,
  parseCep,
  parsePhoneBR,
} from "../utils/masks";
import type { DocumentMode } from "../ui/document-input";

// Builders zod reutilizáveis para os campos padronizados. Centralizam as
// mensagens e reusam os validadores ESTRITOS de masks.ts (CPF/CNPJ espelham
// o backend Go — nunca relaxar). Convenção: campos de texto usam "" como
// vazio (os inputs mascarados emitem "" quando esvaziados), então as versões
// "opcionais" aceitam "" via refine em vez de `.optional()`.

type Opt = { required?: boolean };

export const zRequiredString = (message = "Campo obrigatório") =>
  z.string().trim().min(1, message);

export const zEmail = ({ required = true }: Opt = {}) => {
  const valid = z.string().trim().email("E-mail inválido");
  return required ? valid : z.union([z.literal(""), valid]);
};

// Nome de pessoa: mínimo 2 caracteres e sem dígitos (regra do PortalProfile).
export const zPersonName = (message = "Informe o nome completo") =>
  z
    .string()
    .trim()
    .min(2, message)
    .refine((v) => !/\d/.test(v), "Nome não pode conter números");

export const zPhone = ({ required = false }: Opt = {}) =>
  z
    .string()
    .refine(
      (v) => (required ? parsePhoneBR(v).length >= 10 : !v || parsePhoneBR(v).length >= 10),
      "Telefone incompleto",
    );

export const zCep = ({ required = false }: Opt = {}) =>
  z
    .string()
    .refine(
      (v) => (required ? parseCep(v).length === 8 : !v || parseCep(v).length === 8),
      "CEP incompleto",
    );

export const zDocument = ({
  mode = "auto",
  required = false,
}: { mode?: DocumentMode } & Opt = {}) => {
  const validator = mode === "cpf" ? isValidCpf : mode === "cnpj" ? isValidCnpj : isValidCpfCnpj;
  const label = mode === "cpf" ? "CPF" : mode === "cnpj" ? "CNPJ" : "CPF/CNPJ";
  return z
    .string()
    .refine((v) => (required ? validator(v) : !v || validator(v)), `${label} inválido`);
};

export const zUF = ({ required = false }: Opt = {}) =>
  z
    .string()
    .trim()
    .refine(
      (v) => (required ? /^[A-Za-z]{2}$/.test(v) : !v || /^[A-Za-z]{2}$/.test(v)),
      "UF inválida",
    );

export const zPercent = ({ required = false }: Opt = {}) => {
  const base = z.number().min(0, "Mínimo 0%").max(100, "Máximo 100%");
  return required ? base : base.optional();
};

// Valor monetário em reais (MoneyInput emite reais como número).
export const zMoneyReais = ({ required = false }: Opt = {}) => {
  const base = z.number().nonnegative("Valor inválido");
  return required ? base : base.optional();
};

// Valor monetário já em centavos inteiros (campos `*_cents`).
export const zMoneyCents = ({ required = false }: Opt = {}) => {
  const base = z.number().int("Valor inválido").nonnegative("Valor inválido");
  return required ? base : base.optional();
};
