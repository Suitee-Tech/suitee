import path from 'node:path';
import { parse } from '@babel/parser';

export function findBoundaryViolations(source, file, root, forbiddenRoots) {
  const ast = parse(source, { sourceType: 'unambiguous', plugins: ['typescript', 'jsx'], createImportExpressions: true });
  const violations = [];
  const inspect = node => {
    if (!node) return;
    let literal;
    if (['ImportDeclaration', 'ExportNamedDeclaration', 'ExportAllDeclaration'].includes(node.type)) literal = node.source;
    if (node.type === 'ImportExpression') literal = node.source;
    if (node.type === 'CallExpression' && node.callee?.type === 'Identifier' && node.callee.name === 'require') literal = node.arguments[0];
    const spec = literal?.type === 'StringLiteral' ? literal.value : literal?.type === 'TemplateLiteral' && literal.expressions.length === 0 ? literal.quasis[0].value.cooked : null;
    if (spec) {
      const target = spec.startsWith('@/') ? path.resolve(root, 'src', spec.slice(2))
        : spec.startsWith('src/') ? path.resolve(root, spec)
        : spec.startsWith('.') ? path.resolve(path.dirname(file), spec) : null;
      if (target && forbiddenRoots.some(name => {
        const relative = path.relative(path.resolve(root, 'src', name), target);
        return relative === '' || (!relative.startsWith('..') && !path.isAbsolute(relative));
      })) violations.push({ spec, line: literal.loc.start.line });
    }
    for (const [key, value] of Object.entries(node)) {
      if (key === 'loc' || key === 'comments' || key === 'tokens') continue;
      if (Array.isArray(value)) value.forEach(child => { if (child?.type) inspect(child); });
      else if (value?.type) inspect(value);
    }
  };
  inspect(ast.program);
  return violations;
}
