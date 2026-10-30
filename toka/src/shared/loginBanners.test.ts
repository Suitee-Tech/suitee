import { describe, expect, it } from "vitest";
import {
  DEFAULT_LOGIN_BANNER_ID,
  LOGIN_BANNERS,
  effectiveLoginBannerSource,
  findLoginBanner,
  hasCustomLoginBanner,
  resolveLoginBanner,
} from "./loginBanners";

const srcOf = (id: string) => LOGIN_BANNERS.find((banner) => banner.id === id)?.src;
const defaultSrc = () => findLoginBanner(DEFAULT_LOGIN_BANNER_ID)?.src;

describe("loginBanners", () => {
  it("registra todo preset com id, rotulo e imagem", () => {
    expect(LOGIN_BANNERS.length).toBeGreaterThan(0);
    for (const banner of LOGIN_BANNERS) {
      expect(banner.id).toBeTruthy();
      expect(banner.label).toBeTruthy();
      expect(banner.src).toBeTruthy();
    }
  });

  it("mantem o preset padrao dentro do registro", () => {
    expect(findLoginBanner(DEFAULT_LOGIN_BANNER_ID)).toBeDefined();
    expect(defaultSrc()).toBeTruthy();
  });

  it("nao repete ids", () => {
    const ids = LOGIN_BANNERS.map((banner) => banner.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe("resolveLoginBanner", () => {
  // A ordem importa: a imobiliária pode escolher um preset E ter enviado uma
  // imagem; a imagem enviada é a decisão mais recente e mais específica.
  it("prefere a imagem enviada pela imobiliaria sobre o preset", () => {
    expect(resolveLoginBanner("room", "https://cdn.exemplo.com/banner.png")).toBe(
      "https://cdn.exemplo.com/banner.png",
    );
  });

  it("usa o preset escolhido quando nao ha imagem propria", () => {
    expect(resolveLoginBanner("cliente", "")).toBe(srcOf("cliente"));
  });

  it("cai no preset padrao quando nada foi escolhido", () => {
    expect(resolveLoginBanner(undefined, undefined)).toBe(defaultSrc());
    expect(resolveLoginBanner(null, null)).toBe(defaultSrc());
    expect(resolveLoginBanner("", "")).toBe(defaultSrc());
  });

  // Um id removido do registro não pode deixar o painel sem imagem: é o caso de
  // um tenant que escolheu um preset que depois saiu do código.
  it("cai no padrao quando o preset e desconhecido", () => {
    expect(resolveLoginBanner("preset-que-nao-existe", "")).toBe(defaultSrc());
  });

  it("ignora imagem propria em branco", () => {
    expect(resolveLoginBanner("trm", "   ")).toBe(srcOf("trm"));
  });

  it("devolve uma imagem real, nunca um src vazio", () => {
    expect(resolveLoginBanner(undefined, undefined)).toMatch(/\.webp$/);
  });
});

// A imobiliária pode ter as DUAS imagens guardadas (a enviada e um preset) e
// alternar entre elas. Antes, escolher um preset apagava a imagem enviada do
// registro e obrigava a subir o arquivo de novo.
describe("resolveLoginBanner com a fonte escolhida", () => {
  const CUSTOM = "https://cdn.exemplo.com/banner.png";

  it("usa o preset quando a fonte e o preset, mesmo com imagem enviada", () => {
    expect(resolveLoginBanner("room", CUSTOM, "preset")).toBe(srcOf("room"));
  });

  it("usa a imagem enviada quando a fonte e custom", () => {
    expect(resolveLoginBanner("room", CUSTOM, "custom")).toBe(CUSTOM);
  });

  it("volta para a imagem enviada depois de passar por um preset", () => {
    const comPreset = resolveLoginBanner("room", CUSTOM, "preset");
    const deVolta = resolveLoginBanner("room", CUSTOM, "custom");
    expect(comPreset).toBe(srcOf("room"));
    expect(deVolta).toBe(CUSTOM);
  });

  // Sem imagem guardada, `custom` não pode deixar o painel vazio.
  it("cai no preset quando a fonte e custom mas nao ha imagem", () => {
    expect(resolveLoginBanner("trm", "", "custom")).toBe(srcOf("trm"));
    expect(resolveLoginBanner("trm", "   ", "custom")).toBe(srcOf("trm"));
  });

  it("cai no preset padrao quando a fonte e custom sem imagem e sem preset", () => {
    expect(resolveLoginBanner("", "", "custom")).toBe(defaultSrc());
  });

  // Compatibilidade: sessões e registros gravados antes do campo existir não
  // podem mudar de imagem.
  it("mantem a regra antiga quando a fonte nao esta definida", () => {
    expect(resolveLoginBanner("room", CUSTOM)).toBe(CUSTOM);
    expect(resolveLoginBanner("room", "")).toBe(srcOf("room"));
    expect(resolveLoginBanner("room", CUSTOM, "")).toBe(CUSTOM);
    expect(resolveLoginBanner("room", CUSTOM, null)).toBe(CUSTOM);
  });

  it("nao deixa um valor desconhecido apagar a imagem do painel", () => {
    expect(resolveLoginBanner("room", CUSTOM, "qualquer-coisa")).toBe(CUSTOM);
    expect(resolveLoginBanner("room", "", "qualquer-coisa")).toBe(srcOf("room"));
  });
});

describe("effectiveLoginBannerSource", () => {
  it("respeita as duas fontes validas", () => {
    expect(effectiveLoginBannerSource("custom", "")).toBe("custom");
    expect(effectiveLoginBannerSource("preset", "https://cdn.exemplo.com/a.png")).toBe("preset");
  });

  it("deriva da URL quando o campo esta vazio, como antes do campo existir", () => {
    expect(effectiveLoginBannerSource("", "https://cdn.exemplo.com/a.png")).toBe("custom");
    expect(effectiveLoginBannerSource(undefined, "")).toBe("preset");
    expect(effectiveLoginBannerSource(null, "   ")).toBe("preset");
    expect(effectiveLoginBannerSource("", "  https://cdn.exemplo.com/a.png  ")).toBe("custom");
  });
});

describe("hasCustomLoginBanner", () => {
  it("so considera imagem util quando ha URL de verdade", () => {
    expect(hasCustomLoginBanner("https://cdn.exemplo.com/a.png")).toBe(true);
    expect(hasCustomLoginBanner("")).toBe(false);
    expect(hasCustomLoginBanner("   ")).toBe(false);
    expect(hasCustomLoginBanner(undefined)).toBe(false);
  });
});
