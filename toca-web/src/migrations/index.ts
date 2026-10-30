import * as migration_20261026_155217_initial from './20261026_155217_initial';
import * as migration_20261026_181053_add_tenant_creci from './20261026_181053_add_tenant_creci';
import * as migration_20261027_120000_grant_payload_runtime from './20261027_120000_grant_payload_runtime';

export const migrations = [
  {
    up: migration_20261026_155217_initial.up,
    down: migration_20261026_155217_initial.down,
    name: '20261026_155217_initial',
  },
  {
    up: migration_20261026_181053_add_tenant_creci.up,
    down: migration_20261026_181053_add_tenant_creci.down,
    name: '20261026_181053_add_tenant_creci'
  },
  {
    up: migration_20261027_120000_grant_payload_runtime.up,
    down: migration_20261027_120000_grant_payload_runtime.down,
    name: '20261027_120000_grant_payload_runtime'
  },
];
