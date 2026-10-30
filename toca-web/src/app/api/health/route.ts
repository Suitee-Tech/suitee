/**
 * DB-independent liveness probe used by Docker/compose and load balancers.
 * It intentionally never imports Payload or opens a database connection.
 */
export const dynamic = 'force-dynamic'
export const revalidate = false

export function GET(): Response {
  return Response.json(
    { status: 'ok', service: 'toca-web' },
    {
      status: 200,
      headers: {
        'Cache-Control': 'no-store',
      },
    },
  )
}
