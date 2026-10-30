import type { Property } from "../../data";

export function propertyNeedsHydration(property: Property | null | undefined): boolean {
  return (
    !property ||
    property.images === undefined ||
    property.videos === undefined ||
    property.owners === undefined ||
    property.capturingBrokers === undefined
  );
}

/**
 * Detail endpoints contain the relationship/media collections that may be
 * omitted from list responses. List snapshots, however, can carry fresher
 * scalar values after a mutation or realtime refresh. Merge both while the
 * context item is partial so a cached detail response never shadows a newer
 * price, title, status or address.
 */
export function resolvePropertyDetailSnapshot(
  contextProperty: Property | null | undefined,
  detailProperty: Property | null | undefined,
): Property | null {
  if (!contextProperty) return detailProperty ?? null;
  if (!detailProperty) return contextProperty;
  if (!propertyNeedsHydration(contextProperty)) return contextProperty;

  return {
    ...detailProperty,
    ...contextProperty,
    images: detailProperty.images ?? contextProperty.images,
    videos: detailProperty.videos ?? contextProperty.videos,
    owners: detailProperty.owners ?? contextProperty.owners,
    capturingBrokers: detailProperty.capturingBrokers ?? contextProperty.capturingBrokers,
  };
}
