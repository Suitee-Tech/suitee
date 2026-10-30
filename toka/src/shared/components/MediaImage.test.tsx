import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MediaImage } from "./MediaImage";

type RuntimeEnvGlobal = typeof globalThis & {
  __TOCA_ENV__?: Record<string, string | undefined>;
};

const runtimeGlobal = globalThis as RuntimeEnvGlobal;
const originalRuntimeEnv = runtimeGlobal.__TOCA_ENV__;

beforeEach(() => {
  runtimeGlobal.__TOCA_ENV__ = {
    VITE_SUPABASE_URL: "http://localhost:54321",
  };
});

afterEach(() => {
  cleanup();
  runtimeGlobal.__TOCA_ENV__ = originalRuntimeEnv;
});

describe("MediaImage", () => {
  it("normalizes internal Supabase URLs before rendering", () => {
    render(
      <MediaImage
        src="http://host.docker.internal:54321/storage/v1/object/public/toca-avatars/avatar.png"
        alt="Avatar"
      />,
    );

    expect(screen.getByRole("img", { name: "Avatar" })).toHaveAttribute(
      "src",
      "http://localhost:54321/storage/v1/object/public/toca-avatars/avatar.png",
    );
  });

  it("calls the consumer error handler and renders its fallback", () => {
    const onError = vi.fn();
    render(
      <MediaImage
        src="https://cdn.example.com/broken.png"
        alt="Imagem"
        onError={onError}
        fallback={<span>Imagem indisponível</span>}
      />,
    );

    fireEvent.error(screen.getByRole("img", { name: "Imagem" }));

    expect(onError).toHaveBeenCalledOnce();
    expect(screen.getByText("Imagem indisponível")).toBeInTheDocument();
  });

  it("tries fallbackSrc before rendering the fallback node", () => {
    render(
      <MediaImage
        src="https://cdn.example.com/broken.png"
        fallbackSrc="https://cdn.example.com/fallback.png"
        alt="Imagem"
        fallback={<span>Sem imagem</span>}
      />,
    );

    const image = screen.getByRole("img", { name: "Imagem" });
    fireEvent.error(image);
    expect(image).toHaveAttribute(
      "src",
      "https://cdn.example.com/fallback.png",
    );

    fireEvent.error(image);
    expect(screen.getByText("Sem imagem")).toBeInTheDocument();
  });

  it("recovers when src changes after an error", () => {
    const { rerender } = render(
      <MediaImage
        src="https://cdn.example.com/old.png"
        alt="Imagem"
        fallback={<span>Sem imagem</span>}
      />,
    );
    fireEvent.error(screen.getByRole("img", { name: "Imagem" }));
    expect(screen.getByText("Sem imagem")).toBeInTheDocument();

    rerender(
      <MediaImage
        src="https://cdn.example.com/new.png"
        alt="Imagem"
        fallback={<span>Sem imagem</span>}
      />,
    );

    expect(screen.getByRole("img", { name: "Imagem" })).toHaveAttribute(
      "src",
      "https://cdn.example.com/new.png",
    );
  });

  it("does not render a raw authenticated Storage route", () => {
    render(
      <MediaImage
        src="http://localhost:54321/storage/v1/object/authenticated/toca-documents/private.png"
        alt="Privada"
        fallback={<span>Conteúdo protegido</span>}
      />,
    );

    expect(screen.queryByRole("img", { name: "Privada" })).not.toBeInTheDocument();
    expect(screen.getByText("Conteúdo protegido")).toBeInTheDocument();
  });
});
