import path from 'node:path'
import type { CollectionConfig } from 'payload'
import {
  internalCreateAccess,
  internalUpdateAccess,
  superAdminAccess,
} from '@/access'
import { forceTenantFromInternal } from '@/hooks/tenant'
import { getMediaDirectory } from '@/lib/env'

export const Media: CollectionConfig = {
  slug: 'media',
  labels: {
    singular: 'Mídia',
    plural: 'Mídias',
  },
  admin: {
    useAsTitle: 'alt',
    group: 'Sites',
    defaultColumns: ['alt', 'filename', 'mimeType', 'updatedAt'],
  },
  access: {
    // Uploaded brand assets are public by design. Writes remain internal/admin.
    read: () => true,
    create: internalCreateAccess,
    update: internalUpdateAccess,
    delete: superAdminAccess,
  },
  hooks: {
    beforeValidate: [forceTenantFromInternal],
  },
  upload: {
    staticDir: path.resolve(/* turbopackIgnore: true */ process.cwd(), getMediaDirectory()),
    mimeTypes: ['image/*'],
    focalPoint: true,
    adminThumbnail: 'thumbnail',
    imageSizes: [
      {
        name: 'thumbnail',
        width: 320,
        height: 320,
        position: 'centre',
      },
      {
        name: 'card',
        width: 768,
        height: 576,
        position: 'centre',
      },
      {
        name: 'hero',
        width: 1920,
        height: 1080,
        position: 'centre',
      },
    ],
  },
  fields: [
    {
      name: 'alt',
      label: 'Texto alternativo',
      type: 'text',
      required: true,
    },
    {
      name: 'caption',
      label: 'Legenda',
      type: 'text',
    },
  ],
}
