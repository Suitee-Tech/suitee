import assert from 'node:assert/strict';
import test from 'node:test';

import { findCopyIssues } from './check-copy.mjs';

const issues = (source) => findCopyIssues(source, 'fixture.tsx');

test('flagra acentuação em texto JSX, label e toast', () => {
  assert.equal(issues('<p>Nao foi possivel carregar</p>').length, 1);
  assert.equal(issues('<Field label="Configuracoes" />').length, 1);
  assert.equal(issues('toast.error("Erro ao salvar configuracao")').length, 1);
  assert.equal(issues('const x = { title: "Relatorio" };').length, 1);
});

test('flagra jargão em copy sem punir rota ou valor técnico', () => {
  assert.equal(issues('<p>Audit Trail</p>').length, 1);
  assert.equal(issues('const to = "/configuracoes";').length, 0);
  assert.equal(issues('const kind = { value: "locacao" };').length, 0);
  assert.equal(issues('<Field placeholder="Ex: contrato_locacao.pdf" />').length, 0);
  assert.equal(issues('const msg = "tenant:123";').length, 0);
  assert.equal(issues('console.error("configuracoes")').length, 0);
});

test('não considera comentários como copy', () => {
  assert.equal(issues('// Configuracoes antigas\nconst x = 1;').length, 0);
});

test('detecta acentuação em frase de propriedade de objeto de UI', () => {
  const result = issues('const tabs = [{ id: "x", label: "Notificacoes" }];');
  assert.equal(result.length, 1);
  assert.match(result[0].text, /Notificacoes/);
});
