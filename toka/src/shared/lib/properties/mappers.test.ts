import { describe, expect, it, vi } from "vitest";
import type { NavigateFunction } from "react-router";
import { mapPropertyDtoToProperty } from "./mappers";
import type { PropertyDTO } from "../../services/property.service";
import { buildContractIssues } from "../../utils/contractIssues";

// Reproduces the prod row for "Apartamento luxo" (eb9b8b9a…): the property has
// a real owner in `owners[]`, but the legacy `owner` field is a Go zero-value
// struct ({ id: "" }). Before the fix, mapPropertyDtoToProperty produced
// ownerId: "" and the `ownerId ?? owners[0].id` chains short-circuited on the
// empty string — so the rental charge config wrongly reported
// "Imóvel sem proprietário cadastrado".
function buildDto(): PropertyDTO {
  return {
    id: "eb9b8b9a-a075-40cb-b62c-f47dfef84df2",
    title: "Apartamento luxo",
    status: "rented",
    property_type: "apartment",
    listing_type: "rent",
    address: {},
    pricing: {},
    areas: {},
    features: {},
    owner: { id: "", type: "", since: "0001-01-01T00:00:00Z" },
    owners: [
      {
        id: "c5bbc36e-cbf8-49d3-8380-942fca94a2bf",
        type: "person",
        since: "0001-01-01T00:00:00Z",
        percentage: 100,
      },
    ],
    capturing_brokers: [
      { id: "09d60e89-18bf-45ba-884c-00f62a6c5d13", role: "captador" },
    ],
  } as unknown as PropertyDTO;
}

describe("mapPropertyDtoToProperty — owner resolution", () => {
  it("normalizes the empty legacy owner struct to an undefined ownerId", () => {
    const p = mapPropertyDtoToProperty(buildDto());
    // The bug: this used to be "" (empty string), which is not nullish and so
    // poisoned every `ownerId ?? owners[0].id` fallback downstream.
    expect(p.ownerId).toBeUndefined();
    expect(p.owners?.[0]?.id).toBe("c5bbc36e-cbf8-49d3-8380-942fca94a2bf");
  });

  it("resolves the real owner id from owners[] when the legacy owner is empty", () => {
    const p = mapPropertyDtoToProperty(buildDto());
    const ownerId = p.ownerId || p.owners?.[0]?.id || "";
    expect(ownerId).toBe("c5bbc36e-cbf8-49d3-8380-942fca94a2bf");
  });
});

describe("mapPropertyDtoToProperty — legacy media", () => {
  it("normalizes media without images instead of crashing the property list", () => {
    const dto = buildDto();
    dto.media = {};

    const property = mapPropertyDtoToProperty(dto);

    expect(property.image).toBe("");
    expect(property.images).toEqual([]);
    expect(property.videos).toEqual([]);
  });

  it("keeps mapping the full list when legacy and modern properties are mixed", () => {
    const legacy = { ...buildDto(), id: "legacy", media: {} };
    const modern = {
      ...buildDto(),
      id: "modern",
      media: { images: [{ url: "https://example.test/property.jpg", order: 0 }] },
    };

    const properties = [legacy, modern].map(mapPropertyDtoToProperty);

    expect(properties).toHaveLength(2);
    expect(properties[0].images).toEqual([]);
    expect(properties[1].image).toBe("https://example.test/property.jpg");
  });

  it("normalizes persisted internal Supabase image URLs at the mapping boundary", () => {
    const runtimeGlobal = globalThis as typeof globalThis & {
      __TOCA_ENV__?: Record<string, string | undefined>;
    };
    const previous = runtimeGlobal.__TOCA_ENV__;
    runtimeGlobal.__TOCA_ENV__ = {
      VITE_SUPABASE_URL: "http://localhost:54321",
    };

    try {
      const dto = {
        ...buildDto(),
        media: {
          images: [
            {
              url: "http://host.docker.internal:54321/storage/v1/object/public/toca-property-images/property.jpg",
              order: 0,
            },
          ],
        },
      };

      const property = mapPropertyDtoToProperty(dto);

      expect(property.image).toBe(
        "http://localhost:54321/storage/v1/object/public/toca-property-images/property.jpg",
      );
      expect(property.images?.[0]?.url).toBe(property.image);
    } finally {
      runtimeGlobal.__TOCA_ENV__ = previous;
    }
  });

  it("normalizes malformed optional collections from legacy records", () => {
    const dto = {
      ...buildDto(),
      address: undefined,
      pricing: undefined,
      areas: undefined,
      features: undefined,
      amenities: null,
      media: { images: null, videos: null },
      owners: null,
      capturing_brokers: null,
      available_for: null,
      interested_leads: null,
    } as unknown as PropertyDTO;

    const property = mapPropertyDtoToProperty(dto);

    expect(property.address).toMatchObject({ street: "", city: "", stateCode: "" });
    expect(property.price).toBe(0);
    expect(property.area).toBe(0);
    expect(property.amenities).toEqual([]);
    expect(property.images).toEqual([]);
    expect(property.videos).toEqual([]);
  });
});

describe("buildContractIssues — rental charge config", () => {
  it("does not flag a missing owner when only owners[] is populated", () => {
    const property = mapPropertyDtoToProperty(buildDto());
    const contract = {
      id: "599f11cd-fc6a-495b-9a72-368c27590d7a",
      property_id: "eb9b8b9a-a075-40cb-b62c-f47dfef84df2",
      customer_id: "tenant-1",
    };

    const issues = buildContractIssues(
      contract,
      property,
      vi.fn() as unknown as NavigateFunction,
    );

    expect(issues.find((i) => i.ref === "owner")).toBeUndefined();
    expect(issues).toHaveLength(0);
  });
});
