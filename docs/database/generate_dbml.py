#!/usr/bin/env python3
"""Render a PostgreSQL catalog (catalog.sql JSON output) as physical DBML.
Usage: python3 docs/database/generate_dbml.py /tmp/catalog.json
No database connections or mutations are performed by this script.
"""
import argparse
import json
import re
from pathlib import Path

from layout_dbml import decorate

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent

def quote(value):
    return "'" + str(value).replace('\\', '\\\\').replace("'", "\\'").replace('\n', '\\n').replace('\r', '') + "'"

def ident(value):
    return '"' + value.replace('"', '\\"') + '"'

def render(tables):
    lines = ['// Gerado por generate_dbml.py. Modelo fisico das migrations versionadas.',
             '// Relacoes logicas/JSONB: consulte platform.logical.dbml e README.md.',
             "Project suitee_physical {", "  database_type: 'PostgreSQL'",
             "  Note: 'Catalogo reconstruido das migrations; nao e um snapshot de producao. Indices parciais e metodos nao suportados sao preservados nas notas SQL.'", '}', '']
    refs = []
    for t in tables:
        name = t['schema'] + '.' + t['name']
        sources = []
        for p in sorted((ROOT/'supabase/migrations').glob('*.sql')):
            if re.search(r'CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?'+re.escape(name)+r'\s*\(', p.read_text(), re.I):
                sources.append(str(p.relative_to(ROOT)))
        notes = ['Origem: '+', '.join(sources), 'RLS habilitado: '+str(t['rls']).lower()]
        if t['partition_key']:
            notes.append('Particionamento: '+t['partition_key']+'; particoes filhas omitidas.')
        if t['comment']:
            notes.append(t['comment'])
        lines.append('Table '+name+' {')
        constraints = t['constraints'] or []
        pks = [c['columns'] for c in constraints if c['type']=='p']
        single_pk = pks[0][0] if pks and len(pks[0])==1 else None
        for c in t['columns']:
            settings = []
            if c['name']==single_pk: settings.append('pk')
            if c['not_null']: settings.append('not null')
            if c['default'] is not None: settings.append('default: `'+c['default'].replace('`','\\`')+'`')
            if c['comment']: settings.append('note: '+quote(c['comment']))
            typ = ident(c['type']) if re.search(r'\s', c['type']) else c['type']
            lines.append('  '+ident(c['name'])+' '+typ+(' ['+', '.join(settings)+']' if settings else ''))
        indices=[]
        for i in t['indexes'] or []:
            # DBML has no partial-index predicate or GIN/GiST semantics: preserve
            # exact SQL in Notes instead of misrepresenting global uniqueness.
            if i['predicate'] or i['method'] not in ('btree','hash'):
                notes.append('Indice SQL: '+i['definition'])
                continue
            if i['primary'] and single_pk: continue
            keys=[]
            for k in i['keys']:
                keys.append(ident(k.strip('"')) if re.fullmatch(r'"?[a-zA-Z_][a-zA-Z_0-9]*"?',k) else '`'+k.replace('`','\\`')+'`')
            settings=['name: '+quote(i['name'])]
            if i['primary']: settings.append('pk')
            elif i['unique']: settings.append('unique')
            settings.append('type: '+i['method'])
            indices.append('    ('+', '.join(keys)+') ['+', '.join(settings)+']')
        if indices: lines+=['  indexes {']+indices+['  }']
        for c in constraints:
            if c['type']=='c': notes.append('CHECK '+c['name']+': '+c['definition'])
            if c['type']=='f':
                cols=lambda cs: '('+', '.join(ident(x) for x in cs)+')'
                actions={'a':'no action','r':'restrict','c':'cascade','n':'set null','d':'set default'}
                refs.append('Ref '+t['schema']+'_'+c['name']+': '+name+'.'+cols(c['columns'])+' > '+c['target']+'.'+cols(c['target_columns'])+' [delete: '+actions[c['delete']]+', update: '+actions[c['update']]+']')
        lines.append('  Note: '+quote('\n'.join(notes)))
        lines+=['}', '']
    lines+=refs+['']
    for schema in sorted({t['schema'] for t in tables}):
        lines+=['TableGroup '+schema+' {']+['  '+schema+'.'+t['name'] for t in tables if t['schema']==schema]+['}', '']
    return decorate('\n'.join(lines))

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('catalog',type=Path)
    args=parser.parse_args()
    tables=json.loads(args.catalog.read_text())
    (OUT/'platform.dbml').write_text(render(tables))
    print(f"{len(tables)} tables; {sum(len(t['columns']) for t in tables)} columns; "+str(sum(c['type']=='f' for t in tables for c in t['constraints'] or []))+' foreign keys')
