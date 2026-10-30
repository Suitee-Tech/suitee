import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import { CONTRACT_VARIABLES, getVariableByKey } from "../shared/contracts/contractVariables";

const read = (relative: string) => readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

describe("documentação de variáveis de contrato", () => {
  const help = read("./docs/topicos/21-variaveis-contrato.md");
  const reference = read("../../../docs/contratos/VARIAVEIS-CONTRATO.md");

  it("documenta todas as variáveis do catálogo na Ajuda e na referência", () => {
    const missingHelp = CONTRACT_VARIABLES
      .map((variable) => variable.key)
      .filter((key) => !help.includes(`{{${key}}}`));
    const missingReference = CONTRACT_VARIABLES
      .map((variable) => variable.key)
      .filter((key) => !reference.includes(`{{${key}}}`));

    expect(missingHelp).toEqual([]);
    expect(missingReference).toEqual([]);
  });

  it("não documenta código que não exista no catálogo", () => {
    const keys = new Set(CONTRACT_VARIABLES.map((variable) => variable.key));
    const documented = Array.from(help.matchAll(/^\| `\{\{([a-zA-Z0-9_.]+)\}\}`/gm), (match) => match[1]);
    expect(Array.from(new Set(documented)).filter((key) => !keys.has(key) && !getVariableByKey(key))).toEqual([]);
  });
});
