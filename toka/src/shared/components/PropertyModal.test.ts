import { describe, expect, it } from "vitest";
import { resolveCreatedPropertyImages } from "./PropertyModal";

describe("resolveCreatedPropertyImages", () => {
  it("troca previews temporários pelas URLs persistidas sem perder a ordem", () => {
    const images = [
      { url: "blob:first", filename: "pending:first", order: 0 },
      { url: "https://example.test/external.jpg", order: 1 },
      { url: "blob:last", filename: "pending:last", order: 2 },
    ];
    const uploads = new Map([
      [
        "pending:first",
        {
          url: "https://storage.test/first.jpg",
          filename: "properties/first.jpg",
        },
      ],
      [
        "pending:last",
        {
          url: "https://storage.test/last.jpg",
          filename: "properties/last.jpg",
        },
      ],
    ]);

    expect(resolveCreatedPropertyImages(images, uploads)).toEqual([
      {
        url: "https://storage.test/first.jpg",
        filename: "properties/first.jpg",
        order: 0,
      },
      { url: "https://example.test/external.jpg", order: 1 },
      {
        url: "https://storage.test/last.jpg",
        filename: "properties/last.jpg",
        order: 2,
      },
    ]);
  });

  it("não mantém blobs de uploads que falharam no estado do imóvel", () => {
    const images = [
      { url: "https://example.test/external.jpg", order: 0 },
      { url: "blob:failed", filename: "pending:failed", order: 1 },
      { url: "blob:orphan", order: 2 },
    ];

    expect(resolveCreatedPropertyImages(images, new Map())).toEqual([
      { url: "https://example.test/external.jpg", order: 0 },
    ]);
  });
});
