import { describe, expect, it } from "vitest";
import { dealQueryKey, dealsListQueryKey } from "../deals/queries";
import { leadQueryKey, leadsListQueryKey } from "../leads/queries";
import {
  propertiesListQueryKey,
  propertyFallbackQueryKey,
  propertyQueryKey,
} from "../properties/queries";

describe("tenant-scoped query keys", () => {
  it.each([
    ["lead detail", leadQueryKey("lead-1", "tenant-a"), leadQueryKey("lead-1", "tenant-b")],
    ["lead list", leadsListQueryKey({}, "tenant-a"), leadsListQueryKey({}, "tenant-b")],
    ["property detail", propertyQueryKey("property-1", "tenant-a"), propertyQueryKey("property-1", "tenant-b")],
    ["property fallback", propertyFallbackQueryKey("property-1", "tenant-a"), propertyFallbackQueryKey("property-1", "tenant-b")],
    ["property list", propertiesListQueryKey({}, "tenant-a"), propertiesListQueryKey({}, "tenant-b")],
    ["deal detail", dealQueryKey("deal-1", "tenant-a"), dealQueryKey("deal-1", "tenant-b")],
    ["deal list", dealsListQueryKey(100, 0, "tenant-a"), dealsListQueryKey(100, 0, "tenant-b")],
  ])("%s cannot collide across tenants", (_label, tenantAKey, tenantBKey) => {
    expect(tenantAKey).not.toEqual(tenantBKey);
  });
});
