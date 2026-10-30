#!/usr/bin/env python3
"""Apply DBML presentation and regenerate focused views; no database access.
Run from any directory: python3 docs/database/layout_dbml.py
Supports the line-oriented DBML emitted by this documentation's generator.
"""
import re
from pathlib import Path

OUT = Path(__file__).resolve().parent
# Ordered by the reading journey, not alphabetically.
GROUPS = {
    'toca_auth': ('01 Identidade', '#475569'),
    'crm': ('02 CRM e atendimento', '#2563EB'),
    'core': ('03 Imoveis e empresa', '#0891B2'),
    'deal': ('04 Negocios e contratos', '#7C3AED'),
    'signature': ('05 Assinatura', '#9333EA'),
    'financial_billing': ('06 Financeiro - cobranca', '#0F766E'),
    'financial_settlement': ('07 Financeiro - repasses e comissoes', '#15803D'),
    'financial_accounting': ('08 Financeiro - razao e fiscal', '#B45309'),
    'financial_settings': ('09 Financeiro - configuracao', '#64748B'),
    'ops': ('10 Operacoes', '#C2410C'),
    'calendar': ('11 Agenda', '#BE123C'),
    'comm': ('12 Comunicacao', '#0369A1'),
    'ai': ('13 Assistente IA', '#4F46E5'),
    'portal': ('14 Integracoes com portais', '#0E7490'),
    'events': ('15 Eventos e filas', '#475569'),
    'audit': ('16 Auditoria', '#57534E'),
    'exports': ('17 Exportacoes', '#52525B'),
}
BILLING = {'charges', 'collections', 'invoices', 'payments', 'rentals', 'rental_charge_configs', 'sale_billing_plan', 'sale_billing_plan_drafts', 'utility_accounts'}
SETTLEMENT = {'commissions', 'managed_bills', 'partner_royalties', 'payout_orders', 'transfers', 'transfer_deduction_claims', 'withdrawals'}
ACCOUNTING = {'transactions', 'journal_entries', 'nfse', 'dimob', 'informes', 'fiscal_deadlines', 'report_history'}

def group(name):
    schema, table = name.split('.')
    if schema != 'financial': return schema
    if table in BILLING: return 'financial_billing'
    if table in SETTLEMENT: return 'financial_settlement'
    if table in ACCOUNTING: return 'financial_accounting'
    return 'financial_settings'

def table_blocks(text):
    return {m[1]: m[0] for m in re.finditer(r'^Table ([\w.]+)(?: \[[^\n]*\])? \{\n.*?^\}', text, re.M | re.S)}

def endpoints(ref):
    m = re.search(r': ([\w]+\.[\w]+)\.(\([^)]*\)|\w+) > ([\w]+\.[\w]+)\.(\([^)]*\)|\w+)', ref)
    if not m: raise ValueError('Unsupported reference: '+ref)
    columns = lambda value: [c.strip(' "') for c in value.strip('()').split(',')]
    return m[1], columns(m[2]), m[3], columns(m[4])

def decorate(text):
    # Replace only generated presentation blocks, preserving table/ref contents.
    text = re.sub(r'^TableGroup .*?^\}\n?', '', text, flags=re.M | re.S)
    text = re.sub(r'^Note legenda_layout .*?^\}\n?', '', text, flags=re.M | re.S)
    text = re.sub(r'^DiagramView leitura_.*?^\}\n?', '', text, flags=re.M | re.S)
    blocks = table_blocks(text)
    order = {key: i for i, key in enumerate(GROUPS)}
    names = sorted(blocks, key=lambda n: (order[group(n)], n))
    for name, block in blocks.items(): text = text.replace(block, '', 1)
    pos = text.index('\n}')+2  # Project ends before any table.
    colored = []
    for name in names:
        block = re.sub(r'^Table [^\n]+', f'Table {name} [headercolor: {GROUPS[group(name)][1]}] {{', blocks[name], count=1)
        colored.append(block)
    text = text[:pos]+'\n\n'+'\n\n'.join(colored)+'\n'+text[pos:]
    def ref_color(m):
        line=m[0]
        # Muted logical edges; dark physical FKs. Names remain the text legend.
        color='#94A3B8' if line.startswith('Ref logical_') else '#334155'
        line=re.sub(r',?\s*color: #[0-9A-Fa-f]+', '', line)
        line=re.sub(r'\[\s*\]', '', line).rstrip()
        return line[:-1]+', color: '+color+']' if line.endswith(']') else line+' [color: '+color+']'
    text=re.sub(r'^Ref .*$', ref_color, text, flags=re.M)
    for key,(title,color) in GROUPS.items():
        members=[n for n in names if group(n)==key]
        if members:
            text+='\nTableGroup "'+title+'" [color: '+color+'] {\n'+'\n'.join('  '+n for n in members)+'\n}\n'
    text+='\nNote legenda_layout {\n  \'Cores dos cabecalhos = dominios. Ref logical_* em cinza claro = vinculo de aplicacao; demais Ref em cinza escuro = FK SQL. json__ = projecao JSONB. Use views/01-overview.dbml para iniciar a leitura.\'\n}\n'
    return re.sub(r'\n{3,}', '\n\n', text).strip()+'\n'

VIEWS = {
 '01-overview': ('Visao geral', ['crm.leads','core.properties','deal.deals','deal.contracts','signature.envelopes','financial.charges','financial.transfers','financial.commissions','financial.payout_orders'], {
   ('deal.deals','json__customer_id'), ('deal.deals','json__property_id'),
   ('deal.contracts','deal_id'), ('signature.envelopes','contract_id'),
   ('financial.charges','json__contract_id'), ('financial.transfers','json__source_charge_id'),
   ('financial.commissions','json__contract_id'), ('financial.commissions','json__payout_order_id'),
 }),
 '02-commercial': ('CRM, negocios e assinatura', ['toca_auth.accounts','crm.leads','crm.brokers','core.properties','deal.deals','deal.contracts','deal.documents','signature.envelopes'], None),
 '03-billing': ('Locacao e cobranca', ['deal.contracts','core.properties','financial.rentals','financial.charges','financial.rental_charge_configs','financial.utility_accounts','financial.collections'], None),
 '04-settlement': ('Comissoes, repasses e razao', ['deal.contracts','financial.charges','financial.commissions','financial.transfers','financial.payout_orders','financial.managed_bills','financial.transfer_deduction_claims','financial.transactions','financial.journal_entries'], None),
 '05-operations': ('Operacoes e agenda', ['crm.leads','core.properties','deal.contracts','financial.rentals','ops.maintenance','ops.suppliers','ops.inspections','ops.keys','calendar.appointments'], None),
 '06-communication': ('Identidade, comunicacao e IA', ['toca_auth.accounts','toca_auth.credentials','toca_auth.user_preferences','comm.channels','comm.messages','comm.inapp_notifications','ai.conversations','ai.messages','ai.tool_invocations'], None),
}

def focused(text, title, names, selected):
    blocks=table_blocks(text)
    refs=[]; used={name:set() for name in names}
    for ref in re.findall(r'^Ref .*$',text,re.M):
        a,ac,b,bc=endpoints(ref)
        if a not in used or b not in used: continue
        if selected is not None and (a,ac[0]) not in selected: continue
        refs.append(ref);used[a].update(ac);used[b].update(bc)
    pieces=["// Recorte gerado; editar a fonte platform.logical.dbml e executar layout_dbml.py.",
            "Project suitee_view {", "  database_type: 'PostgreSQL'",
            "  Note: '"+title+". Recorte de leitura: apenas chaves e campos das relacoes visiveis. Modelo logico; nao exportar como migration.'", '}']
    for name in names:
        lines=blocks[name].splitlines(); keep=[lines[0]]
        for line in lines[1:-1]:
            col=re.match(r'  (\w+) ',line)
            if '[pk,' in line or (col and col[1] in used[name]): keep.append(line)
        keep.append('}');pieces.append('\n'.join(keep))
    pieces+=refs
    result=decorate('\n\n'.join(pieces)+'\n')
    result=result.replace('Use views/01-overview.dbml para iniciar a leitura.',f'{len(names)} tabelas; {len(refs)} relacoes neste recorte. Modelo completo: platform.logical.dbml.')
    return result,len(refs)


def save_positions(path, blocks):
    """Write the version-1 sidecar format already used by the local viewer.

    Reserve 460px width and 32px per field (plus header/footer) for each card.
    Coordinates are presentation only; preserve other existing sidecar settings.
    """
    import json
    names = list(blocks)
    positions = {}
    heights = {n: 100 + 32 * sum(bool(re.match(r'  (?:"[^"]+"|\w+) \S+', line)) and not line.startswith('  Note:') for line in blocks[n].splitlines()) for n in names}
    if path.stem == '01-overview':
        grid = {
            'core.properties': (0, 0), 'crm.leads': (0, 270),
            'deal.deals': (520, 130), 'deal.contracts': (1040, 130),
            'signature.envelopes': (1560, 0), 'financial.charges': (1560, 300),
            'financial.transfers': (2080, 300), 'financial.commissions': (1560, 600),
            'financial.payout_orders': (2080, 600),
        }
        positions = {f'table-{n}': dict(zip(('x','y'), grid[n])) for n in names}
    else:
        # Three reading lanes: identities/assets; business/finance; support.
        lanes = [
            ['toca_auth','crm','core','comm','ai'],
            ['deal','signature','financial_billing','financial_settlement','financial_accounting','financial_settings'],
            ['ops','calendar','portal','events','audit','exports'],
        ]
        # Compact focused views into only the occupied lanes.
        active = [lane for lane in lanes if any(group(n) in lane for n in names)]
        for lane_index,lane in enumerate(active):
            top = 0
            for key in lane:
                members = [n for n in names if group(n)==key]
                if not members: continue
                # Keep each domain contiguous and aligned in rows of two cards.
                for i in range(0,len(members),2):
                    row = members[i:i+2]
                    for j,n in enumerate(row):
                        positions['table-'+n]={'x':lane_index*1100+j*520,'y':top}
                    top += max(heights[n] for n in row)+100
                top += 120  # separate domains, not merely individual tables
    sidecar=path.with_name(path.name+'.layout.json')
    payload=json.loads(sidecar.read_text()) if sidecar.exists() else {'version':1}
    if payload.get('version')!=1: raise ValueError('Unsupported layout sidecar version')
    payload['positions']=positions
    sidecar.write_text(json.dumps(payload,indent=2)+'\n')
    # Verify reserved card rectangles never overlap.
    for i,a in enumerate(names):
        ap=positions['table-'+a]
        for b in names[i+1:]:
            bp=positions['table-'+b]
            assert not (ap['x']<bp['x']+460 and bp['x']<ap['x']+460 and ap['y']<bp['y']+heights[b] and bp['y']<ap['y']+heights[a]), (a,b)


if __name__=='__main__':
    for name in ('platform.dbml','platform.logical.dbml'):
        p=OUT/name;p.write_text(decorate(p.read_text()))
    source=(OUT/'platform.logical.dbml').read_text()
    (OUT/'views').mkdir(exist_ok=True)
    for slug,(title,names,selected) in VIEWS.items():
        result,count=focused(source,title,names,selected)
        (OUT/'views'/f'{slug}.dbml').write_text(result)
        print(f'{slug}: {len(names)} tables, {count} refs')
    for path in [OUT/'platform.dbml',OUT/'platform.logical.dbml',*sorted((OUT/'views').glob('*.dbml'))]:
        save_positions(path,table_blocks(path.read_text()))
