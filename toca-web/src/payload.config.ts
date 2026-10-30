import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { postgresAdapter } from '@payloadcms/db-postgres'
import { lexicalEditor } from '@payloadcms/richtext-lexical'
import { s3Storage } from '@payloadcms/storage-s3'
import { multiTenantPlugin } from '@payloadcms/plugin-multi-tenant'
import { buildConfig } from 'payload'
import sharp from 'sharp'
import { userHasAccessToAllTenants } from './access'
import { Media } from './collections/Media'
import { Listings } from './collections/Listings'
import { Pages } from './collections/Pages'
import { Tenants } from './collections/Tenants'
import { Users } from './collections/Users'
import {
  getDatabaseUri,
  getPayloadPublicServerUrl,
  getPayloadSecret,
  isS3Configured,
} from './lib/env'

const dirname = path.dirname(fileURLToPath(import.meta.url))

/**
 * `pg` dá precedência ao sslmode da connection string e transforma
 * `sslmode=require` em verificação full; o objeto `ssl` do Pool não vence.
 * Em beta o pooler do Supabase usa certificado self-signed, então trocamos o
 * modo para `no-verify` apenas quando o compose de beta habilita a exceção.
 * Produção mantém a URI e a verificação originais.
 */
function databaseConnectionString(): string {
  const uri = getDatabaseUri()
  if (process.env.DATABASE_SSL_REJECT_UNAUTHORIZED !== 'false') return uri
  try {
    const parsed = new URL(uri)
    parsed.searchParams.set('sslmode', 'no-verify')
    return parsed.toString()
  } catch {
    return uri
  }
}

const serverURL = getPayloadPublicServerUrl()
const payloadSecret = getPayloadSecret()

const s3Configured = isS3Configured()

/**
 * The S3 plugin is always registered with `alwaysInsertFields` so the database
 * schema is identical with or without object storage; only the adapter is
 * toggled. Local development writes to MEDIA_DIR, production to S3/R2.
 */
const storagePlugin = s3Storage({
  enabled: s3Configured,
  alwaysInsertFields: true,
  bucket: process.env.S3_BUCKET?.trim() || 'toca-web-media-disabled',
  collections: {
    media: {
      prefix: process.env.S3_PREFIX?.trim() || 'media',
    },
  },
  config: {
    region: process.env.S3_REGION?.trim() || 'us-east-1',
    endpoint: process.env.S3_ENDPOINT?.trim() || undefined,
    forcePathStyle: process.env.S3_FORCE_PATH_STYLE === 'true',
    credentials: {
      accessKeyId: process.env.S3_ACCESS_KEY_ID?.trim() || 'disabled',
      secretAccessKey: process.env.S3_SECRET_ACCESS_KEY?.trim() || 'disabled',
    },
  },
  disableLocalStorage: true,
})

export default buildConfig({
  serverURL,
  secret: payloadSecret,
  cors: [serverURL],
  csrf: [serverURL],
  telemetry: false,
  admin: {
    user: 'users',
    meta: {
      titleSuffix: ' | Toka Sites',
      icons: [{ rel: 'icon', type: 'image/svg+xml', url: '/favicon.svg' }],
    },
    importMap: {
      baseDir: path.resolve(dirname),
    },
  },
  collections: [Users, Tenants, Pages, Listings, Media],
  editor: lexicalEditor(),
  sharp,
  typescript: {
    outputFile: path.resolve(dirname, 'payload-types.ts'),
  },
  db: postgresAdapter({
    pool: {
      connectionString: databaseConnectionString(),
    },
    schemaName: 'payload',
    push:
      process.env.PAYLOAD_DB_PUSH === 'true' ||
      (process.env.NODE_ENV !== 'production' && process.env.PAYLOAD_DB_PUSH !== 'false'),
  }),
  plugins: [
    multiTenantPlugin({
      tenantsSlug: 'tenants',
      collections: {
        listings: {},
        pages: {},
        media: {},
      },
      tenantsArrayField: {
        includeDefaultField: false,
      },
      useUsersTenantFilter: false,
      userHasAccessToAllTenants,
    }),
    storagePlugin,
  ],
})
