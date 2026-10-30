import type { Access, CollectionConfig } from 'payload'
import { isSuperAdminUser } from '@/access'
import { internalServiceStrategy } from '@/access/internal-auth'

/**
 * Users are Suitee internal staff only, never agencies. The agency panel lives
 * in Toka. There is intentionally no public signup path once the first
 * super-admin bootstraps the installation.
 */
const bootstrapOrSuperAdminCreate: Access = async ({ req }) => {
  if (isSuperAdminUser(req.user)) {
    return true
  }

  if (req.user) {
    return false
  }

  const existing = await req.payload.count({
    collection: 'users',
    overrideAccess: true,
  })

  return existing.totalDocs === 0
}

export const Users: CollectionConfig = {
  slug: 'users',
  labels: {
    singular: 'Usuário interno',
    plural: 'Usuários internos',
  },
  admin: {
    useAsTitle: 'email',
    group: 'Acesso',
    defaultColumns: ['email', 'role', 'updatedAt'],
    description: 'Equipe Suitee/Toka. O acesso é restrito a super-admins.',
  },
  auth: {
    tokenExpiration: 7200,
    useAPIKey: false,
    strategies: [internalServiceStrategy],
    cookies: {
      sameSite: 'Lax',
    },
  },
  access: {
    admin: ({ req }) => (req.user ? isSuperAdminUser(req.user) : true),
    create: bootstrapOrSuperAdminCreate,
    read: ({ req }) => isSuperAdminUser(req.user),
    update: ({ req }) => isSuperAdminUser(req.user),
    delete: ({ req }) => isSuperAdminUser(req.user),
  },
  fields: [
    {
      name: 'role',
      label: 'Papel',
      type: 'select',
      required: true,
      defaultValue: 'super-admin',
      saveToJWT: true,
      access: {
        create: ({ req }) => isSuperAdminUser(req.user) || !req.user,
        update: ({ req }) => isSuperAdminUser(req.user),
      },
      options: [{ label: 'Super-admin', value: 'super-admin' }],
      admin: {
        description: 'Somente super-admins acessam Payload Admin e os sites.',
      },
    },
  ],
  timestamps: true,
}
