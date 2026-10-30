import { test } from 'node:test';
import assert from 'node:assert/strict';
import { findBoundaryViolations } from './import-boundaries.mjs';
const find = source => findBoundaryViolations(source, '/repo/src/shared/view.tsx', '/repo', ['app']);
test('blocks aliases, side effects, reexports, dynamic imports and require', () => {
  const result = find(`import '@/app/a';\nexport { b } from '../app/b';\nconst c = import('src/app/c');\nconst d = require('@/app/d');\nconst e = import(\`@/app/e\`);`);
  assert.equal(result.length, 5);
  assert.deepEqual(result.map(r => r.line), [1,2,3,4,5]);
});
test('ignores comments, strings, external packages and normalized allowed paths', () => {
  assert.deepEqual(find(`// import '@/app/fake';\nconst text = "import '@/app/fake'";\nimport a from 'app/package';\nimport b from '@/shared/app/model';\nexport { c } from '../app/../shared/c';`), []);
});
