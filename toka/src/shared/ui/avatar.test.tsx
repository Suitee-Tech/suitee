import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Avatar, AvatarFallback, AvatarImage } from "./avatar";

type RuntimeEnvGlobal = typeof globalThis & {
  __TOCA_ENV__?: Record<string, string | undefined>;
};

const runtimeGlobal = globalThis as RuntimeEnvGlobal;
const originalRuntimeEnv = runtimeGlobal.__TOCA_ENV__;

beforeEach(() => {
  runtimeGlobal.__TOCA_ENV__ = {
    VITE_SUPABASE_URL: "http://localhost:54321",
  };
  vi.stubGlobal(
    "Image",
    class {
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      referrerPolicy = "";
      crossOrigin: string | null = null;
      complete = true;
      naturalWidth = 100;
      naturalHeight = 100;
      private loadHandlers: Array<(e: { currentTarget: unknown }) => void> = [];
      private errorHandlers: Array<(e: { currentTarget: unknown }) => void> = [];

      addEventListener(type: "load" | "error", handler: (e: { currentTarget: unknown }) => void) {
        if (type === "load") this.loadHandlers.push(handler);
        else this.errorHandlers.push(handler);
      }

      removeEventListener(type: "load" | "error", handler: (e: { currentTarget: unknown }) => void) {
        if (type === "load") {
          this.loadHandlers = this.loadHandlers.filter((h) => h !== handler);
        } else {
          this.errorHandlers = this.errorHandlers.filter((h) => h !== handler);
        }
      }

      set src(_value: string) {
        queueMicrotask(() => {
          this.onload?.();
          this.loadHandlers.forEach((h) => h({ currentTarget: this }));
        });
      }
    },
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  runtimeGlobal.__TOCA_ENV__ = originalRuntimeEnv;
});

describe("AvatarImage", () => {
  it("normalizes every avatar through the shared media resolver", async () => {
    render(
      <Avatar>
        <AvatarImage
          src="http://host.docker.internal:54321/storage/v1/object/public/toca-avatars/avatar.png"
          alt="Pessoa"
        />
        <AvatarFallback>PS</AvatarFallback>
      </Avatar>,
    );

    expect(await screen.findByRole("img", { name: "Pessoa" })).toHaveAttribute(
      "src",
      "http://localhost:54321/storage/v1/object/public/toca-avatars/avatar.png",
    );
  });
});
