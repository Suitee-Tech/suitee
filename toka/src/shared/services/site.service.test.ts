import { describe, expect, it } from "vitest";

import { previewSiteUrl, suggestSiteSlug, validateSiteSlug } from "./site.service";

describe("site.service — contrato do host comercial", () => {
  it("monta o preview com o sufixo configurado no servidor", () => {
    expect(previewSiteUrl("https", "imobiliaria-exemplo", "imoveis.example.com")).toBe(
      "https://imobiliaria-exemplo.imoveis.example.com",
    );
    expect(previewSiteUrl("http", "exemplo", "imoveis.localhost")).toBe(
      "http://exemplo.imoveis.localhost",
    );
    expect(previewSiteUrl("https", "", "imoveis.example.com")).toBe("");
    expect(previewSiteUrl("http", "exemplo", "imoveis.localhost", "3002")).toBe(
      "http://exemplo.imoveis.localhost:3002",
    );
    expect(previewSiteUrl("https", "exemplo", "imoveis.example.com", "")).toBe(
      "https://exemplo.imoveis.example.com",
    );
  });

  it("recusa marcas reservadas e malformadas", () => {
    for (const slug of [
      "www", "app", "api", "admin", "portal", "sites", "mail", "cdn", "imoveis", "suitee",
      "login", "painel", "teste", "demo", "imobiliaria", "imovel",
    ]) {
      expect(validateSiteSlug(slug)).not.toBeNull();
    }
    expect(validateSiteSlug("123")).not.toBeNull();
    expect(validateSiteSlug("2026")).not.toBeNull();
    expect(validateSiteSlug("ab")).not.toBeNull();
    expect(validateSiteSlug("Exemplo-")).not.toBeNull();
    expect(validateSiteSlug("-Exemplo")).not.toBeNull();
    expect(validateSiteSlug("ca--riri")).not.toBeNull();
    expect(validateSiteSlug("exemplo")).not.toBeNull();
  });

  it("aceita as marcas válidas que o backend aceita", () => {
    for (const slug of ["exemplo", "imobiliaria-exemplo", "casa-2", "a1b"]) {
      expect(validateSiteSlug(slug)).toBeNull();
    }
  });

  it("sugere um slug semântico a partir do nome, sem acentos", () => {
    expect(suggestSiteSlug("Imobiliária Exemplo")).toBe("imobiliaria-exemplo");
    expect(suggestSiteSlug("  João & Cia Imóveis  ")).toBe("joao-cia-imoveis");
    expect(suggestSiteSlug("123")).toBe("123");
    expect(suggestSiteSlug("")).toBe("");
  });
});
