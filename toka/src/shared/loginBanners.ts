import cliente from "../assets/login-banners/cliente.webp";
import equipe from "../assets/login-banners/equipe.webp";
import pessoas from "../assets/login-banners/pessoas.webp";
import room from "../assets/login-banners/room.webp";
import trm from "../assets/login-banners/trm.webp";

/**
 * Imagens de banner oferecidas para o painel esquerdo da tela de login
 * (desktop). São assets locais de propósito: a antiga foto vinha de um host
 * externo (Unsplash) e a requisição vazava do gate e2e, além de depender de
 * terceiros para uma superfície de marca.
 *
 * A imobiliária escolhe um preset em Configurações > Aparência ou envia a
 * própria imagem (`loginBannerUrl`). As duas ficam guardadas e
 * `loginBannerSource` diz qual está ativa, para a troca entre elas ser um
 * clique — antes, voltar para um preset apagava a imagem enviada.
 */
export interface LoginBannerPreset {
  id: string;
  label: string;
  src: string;
}

export const LOGIN_BANNERS: LoginBannerPreset[] = [
  { id: "equipe", label: "Equipe colaborando", src: equipe },
  { id: "room", label: "Sala de estar", src: room },
  { id: "trm", label: "Escritório", src: trm },
  { id: "pessoas", label: "Equipe no escritório", src: pessoas },
  { id: "cliente", label: "Cliente com o celular", src: cliente },
];

export const DEFAULT_LOGIN_BANNER_ID = "equipe";

/** Qual das duas imagens do tenant está ativa no painel de login. */
export type LoginBannerSource = "custom" | "preset";

export const LOGIN_BANNER_SOURCE_CUSTOM: LoginBannerSource = "custom";
export const LOGIN_BANNER_SOURCE_PRESET: LoginBannerSource = "preset";

export function findLoginBanner(id?: string | null): LoginBannerPreset | undefined {
  if (!id) return undefined;
  return LOGIN_BANNERS.find((banner) => banner.id === id);
}

/**
 * Resolve a fonte quando o campo está vazio — registro gravado antes dele
 * existir, ou payload antigo do localStorage. Mesma regra do backend
 * (`EffectiveLoginBannerSource` em toca-core): a URL preenchida ganha do preset,
 * que era o comportamento anterior. Valor desconhecido cai na mesma regra em vez
 * de deixar o painel sem imagem.
 */
export function effectiveLoginBannerSource(
  source?: string | null,
  customUrl?: string | null,
): LoginBannerSource {
  if (source === LOGIN_BANNER_SOURCE_CUSTOM || source === LOGIN_BANNER_SOURCE_PRESET) {
    return source;
  }
  return (customUrl ?? "").trim() ? LOGIN_BANNER_SOURCE_CUSTOM : LOGIN_BANNER_SOURCE_PRESET;
}

/** Há uma imagem própria guardada para oferecer na troca? */
export function hasCustomLoginBanner(customUrl?: string | null): boolean {
  return (customUrl ?? "").trim() !== "";
}

/**
 * Decide qual imagem o painel de login deve exibir.
 *
 * Precedência: a fonte ativa (imagem enviada ou preset) — com a regra de
 * compatibilidade acima para registros antigos. Devolve `undefined` só se nem o
 * padrão existir, para o chamador conseguir omitir o `<img>` em vez de
 * renderizar um `src` inválido.
 */
export function resolveLoginBanner(
  presetId?: string | null,
  customUrl?: string | null,
  source?: string | null,
): string | undefined {
  const custom = (customUrl ?? "").trim();
  if (effectiveLoginBannerSource(source, custom) === LOGIN_BANNER_SOURCE_CUSTOM && custom) {
    return custom;
  }

  const preset = findLoginBanner(presetId) ?? findLoginBanner(DEFAULT_LOGIN_BANNER_ID);
  return preset?.src;
}
