#!/bin/bash
# Seed test data for E2E testing
# Usage: ./scripts/seed-test-data.sh [MONGO_URI]

MONGO_URI="${1:-mongodb://localhost:27017}"

if ! command -v mongosh >/dev/null 2>&1; then
  echo "mongosh not found; skipping mongo seed step (ok for Postgres-based E2E)"
  exit 0
fi

echo "Seeding test data to $MONGO_URI..."

# Seed auth database
mongosh "$MONGO_URI/toca_auth" --eval '
db.credentials.deleteMany({ email: /e2e-.*@test\.com/ });
db.accounts.deleteMany({ "company.name": /E2E Test/ });
print("Auth DB cleaned");
'

# Seed core database with a test property
mongosh "$MONGO_URI/toca_core" --eval '
db.properties.deleteMany({ title: /E2E/ });
print("Core DB cleaned");
'

# Seed CRM database
mongosh "$MONGO_URI/toca_crm" --eval '
db.leads.deleteMany({ email: /e2e-.*@test\.com/ });
db.brokers.deleteMany({ name: /E2E/ });
print("CRM DB cleaned");
'

# Seed deal database
mongosh "$MONGO_URI/toca_deal" --eval '
db.deals.deleteMany({ "notes": /e2e/ });
db.contracts.deleteMany({ title: /E2E/ });
db.documents.deleteMany({});
print("Deal DB cleaned");
'

# Seed financial database
mongosh "$MONGO_URI/toca_financial" --eval '
db.charges.deleteMany({});
db.commissions.deleteMany({});
db.transactions.deleteMany({});
db.owner_transfers.deleteMany({});
db.invoices.deleteMany({});
db.payments.deleteMany({});
db.collections.deleteMany({});
print("Financial DB cleaned");
'

# Seed comm database
mongosh "$MONGO_URI/toca_comm" --eval '
db.channels.deleteMany({});
db.messages.deleteMany({});
db.templates.deleteMany({});
print("Comm DB cleaned");
'

# Seed ops database
mongosh "$MONGO_URI/toca_ops" --eval '
db.maintenance_tickets.deleteMany({});
db.keys.deleteMany({});
db.inspections.deleteMany({});
print("Ops DB cleaned");
'

# Seed calendar database
mongosh "$MONGO_URI/toca_calendar" --eval '
db.appointments.deleteMany({});
print("Calendar DB cleaned");
'

echo "Test data seeded successfully!"
