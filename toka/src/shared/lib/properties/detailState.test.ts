import { describe, expect, it } from "vitest";
import type { Property } from "../../data";
import { propertyNeedsHydration, resolvePropertyDetailSnapshot } from "./detailState";

function property(overrides: Partial<Property> = {}): Property {
  return {
    id: "property-1",
    title: "Casa antiga",
    description: "Descrição",
    address: {
      street: "Rua A",
      number: "10",
      neighborhood: "Centro",
      city: "São Paulo",
      state: "São Paulo",
      stateCode: "SP",
      postalCode: "01000-000",
    },
    price: 4_000_000,
    type: "Casa",
    status: "Em Captação",
    beds: 2,
    baths: 1,
    area: 80,
    image: "detail.jpg",
    images: [{ url: "detail.jpg" }],
    videos: [],
    owners: [{ id: "owner-1" }],
    capturingBrokers: [{ id: "broker-1", role: "captador" }],
    ...overrides,
  };
}

describe("resolvePropertyDetailSnapshot", () => {
  it("keeps fresh scalar values from a partial list snapshot without losing hydrated collections", () => {
    const cachedDetail = property();
    const refreshedListItem = property({
      title: "Casa atualizada",
      price: 4_000_001,
      image: "",
      images: [],
      owners: undefined,
      capturingBrokers: undefined,
    });

    expect(propertyNeedsHydration(refreshedListItem)).toBe(true);
    expect(resolvePropertyDetailSnapshot(refreshedListItem, cachedDetail)).toMatchObject({
      title: "Casa atualizada",
      price: 4_000_001,
      images: [{ url: "detail.jpg" }],
      owners: [{ id: "owner-1" }],
      capturingBrokers: [{ id: "broker-1", role: "captador" }],
    });
  });

  it("uses a complete context mutation result as the authoritative snapshot", () => {
    const cachedDetail = property();
    const mutationResult = property({
      price: 4_000_001,
      images: [],
      owners: [],
      capturingBrokers: [],
    });

    expect(propertyNeedsHydration(mutationResult)).toBe(false);
    expect(resolvePropertyDetailSnapshot(mutationResult, cachedDetail)).toBe(mutationResult);
  });
});
