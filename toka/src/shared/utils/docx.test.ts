import PizZip from 'pizzip';
import { describe, expect, it, vi } from 'vitest';
import {
  DOCX_SCALAR_OVERRIDES_KEY,
  extractDocxTemplateVariables,
  fillDocxTemplate,
  inspectDocxTemplate,
  replaceDocxText,
  replaceDocxVariable,
  resolveTemplateValue,
} from './docx';

const CONTENT_TYPES = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`;

const ROOT_RELS = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`;

function createDocxBase64(bodyXml: string): string {
  const zip = new PizZip();
  zip.file('[Content_Types].xml', CONTENT_TYPES);
  zip.file('_rels/.rels', ROOT_RELS);
  zip.file(
    'word/document.xml',
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
    <w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
      <w:body>${bodyXml}<w:sectPr/></w:body>
    </w:document>`,
  );
  zip.file(
    'word/_rels/document.xml.rels',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>',
  );

  return zip.generate({ type: 'base64' });
}

function createDocxWithHeaderBase64(bodyXml: string, headerXml: string): string {
  const zip = new PizZip(createDocxBase64(bodyXml), { base64: true });
  zip.file(
    '[Content_Types].xml',
    zip.file('[Content_Types].xml')!.asText().replace(
      '</Types>',
      '<Override PartName="/word/header4.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/></Types>',
    ),
  );
  zip.file(
    'word/document.xml',
    zip.file('word/document.xml')!.asText()
      .replace(
        '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">',
        '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">',
      )
      .replace(
        '<w:sectPr/>',
        '<w:sectPr><w:headerReference w:type="default" r:id="rIdHeader4"/></w:sectPr>',
      ),
  );
  zip.file(
    'word/_rels/document.xml.rels',
    zip.file('word/_rels/document.xml.rels')!.asText().replace(
      '</Relationships>',
      '<Relationship Id="rIdHeader4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header4.xml"/></Relationships>',
    ),
  );
  zip.file(
    'word/header4.xml',
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
    <w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">${headerXml}</w:hdr>`,
  );
  return zip.generate({ type: 'base64' });
}

function readDocumentXml(base64: string): string {
  const payload = base64.includes(',') ? base64.split(',')[1] : base64;
  return new PizZip(payload, { base64: true }).file('word/document.xml')!.asText();
}

function decodeWordXmlText(xml: string): string {
  return xml
    .replace(/<[^>]+>/g, '')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&#(?:0*123|x0*7b);/gi, '{')
    .replace(/&#(?:0*125|x0*7d);/gi, '}');
}

function readDocumentText(base64: string): string {
  return decodeWordXmlText(readDocumentXml(base64));
}

function readPartText(base64: string, partName: string): string {
  const payload = base64.includes(',') ? base64.split(',')[1] : base64;
  return decodeWordXmlText(
    new PizZip(payload, { base64: true }).file(partName)!.asText(),
  );
}

function base64ToArrayBuffer(base64: string): ArrayBuffer {
  const binary = atob(base64.includes(',') ? base64.split(',')[1] : base64);
  const buffer = new ArrayBuffer(binary.length);
  const bytes = new Uint8Array(buffer);
  for (let index = 0; index < binary.length; index++) {
    bytes[index] = binary.charCodeAt(index);
  }
  return buffer;
}

function bufferToBase64(buffer: ArrayBuffer): string {
  return new PizZip(buffer).generate({ type: 'base64' });
}

describe('fillDocxTemplate', () => {
  it('detects and renders computed dates from root and local loop bases', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{current_date_plus_30_days}} ' +
      '{{#buyers}}{{name}}: {{current_date_plus_5_business_days}} ' +
      '{{contract_start_date_plus_1_months_iso}}; {{/buyers}}</w:t></w:r></w:p>',
    );
    const data = {
      current_date: '2026-09-17',
      contract_start_date: '2026-01-31',
      buyers: [{ name: 'Maria', contract_start_date: '2026-03-31' }, { name: 'João' }],
    };

    expect(extractDocxTemplateVariables(template, data)).toEqual([
      'current_date_plus_30_days', 'buyers', 'current_date_plus_5_business_days',
    ]);
    const text = readDocumentText(fillDocxTemplate(template, data));
    expect(text).toContain('17/10/2026');
    expect(text).toContain('Maria: 24/09/2026 2026-04-30;');
    expect(text).toContain('João: 24/09/2026 2026-02-28;');
  });

  it('preserves explicit computed-date overrides and unresolved tokens', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{current_date_plus_30_days}} ' +
      '{{current_date_plus_1_days_iso}} {{contract_end_date_plus_1_days}}</w:t></w:r></w:p>',
    );
    const text = readDocumentText(fillDocxTemplate(template, {
      current_date: '2026-09-17',
      current_date_plus_30_days: 'Vencimento acordado',
      [DOCX_SCALAR_OVERRIDES_KEY]: { current_date_plus_1_days_iso: 'Data revisada' },
    }));

    expect(text).toContain('Vencimento acordado Data revisada {{contract_end_date_plus_1_days}}');
  });

  it('renders a variable immediately followed by document text', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{guarantee_partner}}</w:t></w:r><w:r><w:t>Para Parceiros de GarantiaAuto</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { guarantee_partner: 'Suitee' });

    expect(readDocumentText(output)).toContain('SuiteePara Parceiros de GarantiaAuto');
  });

  it('handles the reported extra closing delimiter without aborting preview', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{name}}</w:t></w:r><w:r><w:t>Para Parceiros de GarantiaAutomatica}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { name: 'Suitee' });

    expect(readDocumentText(output)).toContain('SuiteePara Parceiros de GarantiaAutomatica}}');
  });

  it('preserves nested JSON while rendering adjacent template variables', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{name}}</w:t></w:r>' +
      '<w:r><w:t> payload: {"provider":{"rules":{"active":true}}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { name: 'Suitee' });

    expect(readDocumentText(output)).toContain(
      'Suitee payload: {"provider":{"rules":{"active":true}}}',
    );
  });

  it('preserves adjacent JSON closing braces split across Word runs', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{"provider":{"active":true}</w:t></w:r>' +
      '<w:r><w:t>}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {});

    expect(readDocumentText(output)).toContain('{"provider":{"active":true}}');
  });

  it('does not turn braces from separate paragraphs into a template variable', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{literal</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { literal: 'NÃO SUBSTITUIR' });
    const xml = readDocumentXml(output);

    expect(readDocumentText(output)).toContain('{{literal}}');
    expect(readDocumentText(output)).not.toContain('NÃO SUBSTITUIR');
    expect(xml.match(/<w:p\b/g)).toHaveLength(4);
    expect(xml).toContain('&#123;literal');
    expect(xml).toContain('&#125;');
  });

  it.each([
    ['line breaks', 'br'],
    ['carriage returns', 'cr'],
    ['tabs', 'tab'],
  ])(
    'does not turn braces separated by Word %s into a template variable',
    (_label, boundaryTag) => {
      const boundary = `<w:${boundaryTag}/>`;
      const template = createDocxBase64(
        '<w:p>' +
        `<w:r><w:t>{</w:t>${boundary}</w:r>` +
        `<w:r><w:t>{literal</w:t>${boundary}</w:r>` +
        `<w:r><w:t>}</w:t>${boundary}</w:r>` +
        '<w:r><w:t>}</w:t></w:r>' +
        '</w:p>',
      );

      const output = fillDocxTemplate(template, { literal: 'NÃO SUBSTITUIR' });
      const xml = readDocumentXml(output);

      expect(readDocumentText(output)).toContain('{{literal}}');
      expect(readDocumentText(output)).not.toContain('NÃO SUBSTITUIR');
      expect(xml.match(new RegExp(`<w:${boundaryTag}\\/>`, 'g'))).toHaveLength(3);
    },
  );

  it('does not normalize a loop-like tag synthesized across a Word break', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t><w:br/></w:r>' +
      '<w:r><w:t>{ # buyers }}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyers: [{ name: 'Maria' }],
    });

    expect(readDocumentText(output)).toContain('{{ # buyers }}');
    expect(readDocumentText(output)).not.toContain('Maria');
    expect(readDocumentXml(output).match(/<w:br\/>/g)).toHaveLength(1);
  });

  it('renders tags split across Word runs', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{guarantee_</w:t></w:r><w:r><w:t>partner}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { guarantee_partner: 'Suitee' });

    expect(readDocumentText(output)).toContain('Suitee');
    expect(readDocumentText(output)).not.toContain('guarantee_partner');
  });

  it('renders a tag whose delimiter characters are each in a separate run', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t></w:r><w:r><w:t>{buyer.name</w:t></w:r><w:r><w:t>}</w:t></w:r><w:r><w:t>}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { buyer: { name: 'Maria' } });

    expect(readDocumentText(output)).toContain('Maria');
  });

  it('keeps orphan closing braces as literal text instead of failing preview', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>}}Para Parceiros de GarantiaAuto</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {});

    expect(readDocumentText(output)).toContain('}}Para Parceiros de GarantiaAuto');
  });

  it('keeps an orphan closing delimiter split across runs', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>}</w:t></w:r><w:r><w:t>}Para Parceiros</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {});

    expect(readDocumentText(output)).toContain('}}Para Parceiros');
  });

  it('keeps orphan opening braces as literal text instead of corrupting the DOCX', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{Texto literal</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {});

    expect(readDocumentText(output)).toContain('{{Texto literal');
    expect(() => new DOMParser().parseFromString(readDocumentXml(output), 'application/xml')).not.toThrow();
    expect(readDocumentXml(output)).not.toContain('<parsererror');
  });

  it('resolves dotted variables from object data', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{buyer.name}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { buyer: { name: 'Maria' } });

    expect(readDocumentText(output)).toContain('Maria');
  });

  it('preserves arrays used by loops even when loop tags are split across runs', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{#buyers}}</w:t></w:r><w:r><w:t>{{name}}; </w:t></w:r><w:r><w:t>{{/buy</w:t></w:r><w:r><w:t>ers}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyers: [{ name: 'Maria' }, { name: 'Joao' }],
    });

    expect(readDocumentText(output)).toContain('Maria; Joao;');
  });

  it('falls back to root variables from inside an item loop', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{#buyers}}{{custom_note}}: {{name}};{{/buyers}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyers: [{ name: 'Maria' }, { name: 'Joao' }],
      custom_note: 'Aprovado',
    });

    expect(readDocumentText(output)).toContain('Aprovado: Maria;Aprovado: Joao;');
  });

  it('supports the same array as a scalar summary and as a loop', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{buyers}}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{#buyers}}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{name}}; </w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{/buyers}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyers: [{ name: 'Maria' }, { name: 'Joao' }],
    });
    const text = readDocumentText(output);

    expect(text).toContain('Maria, Joao');
    expect(text).toContain('Maria; Joao;');
  });

  it('uses a scalar override without breaking the same array as a loop', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{buyers}}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{#buyers}}{{name}};{{/buyers}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyers: [{ name: 'Maria' }, { name: 'Joao' }],
      [DOCX_SCALAR_OVERRIDES_KEY]: { buyers: 'Partes qualificadas' },
    });
    const text = readDocumentText(output);

    expect(text).toContain('Partes qualificadas');
    expect(text).toContain('Maria;Joao;');
  });

  it('renders the current primitive item with the dot tag', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{#values}}{{.}};{{/values}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { values: ['A', 'B'] });

    expect(readDocumentText(output)).toContain('A;B;');
  });

  it('normalizes whitespace before loop markers in legacy templates', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{ # buyers }}{{name}};{{ / buyers }}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyers: [{ name: 'Maria' }, { name: 'Joao' }],
    });

    expect(readDocumentText(output)).toContain('Maria;Joao;');
  });

  it('keeps zero and false values when an array is rendered as a summary', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{values}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { values: [0, false, 'texto'] });

    expect(readDocumentText(output)).toContain('0, false, texto');
  });

  it('preserves false, zero and missing placeholders', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{active}}|{{amount}}|{{missing}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, { active: false, amount: 0 });

    expect(readDocumentText(output)).toContain('false|0|{{missing}}');
  });

  it('keeps mismatched loops as a structural error', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{#buyers}}{{name}}{{/sellers}}</w:t></w:r></w:p>',
    );
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);

    try {
      expect(() => fillDocxTemplate(template, { buyers: [] })).toThrow();
    } finally {
      consoleError.mockRestore();
    }
  });

  it('renders variables in dynamically numbered headers', () => {
    const template = createDocxWithHeaderBase64(
      '<w:p><w:r><w:t>{{buyer_name}}</w:t></w:r></w:p>',
      '<w:p><w:r><w:t>{{company_</w:t></w:r><w:r><w:t>name}}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyer_name: 'Maria',
      company_name: 'Suitee',
    });

    expect(readDocumentText(output)).toContain('Maria');
    expect(readPartText(output, 'word/header4.xml')).toContain('Suitee');
  });

  it('does not synthesize variables across paragraphs in headers', () => {
    const template = createDocxWithHeaderBase64(
      '<w:p><w:r><w:t>{{buyer_name}}</w:t></w:r></w:p>',
      '<w:p><w:r><w:t>{</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{header_literal</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>}</w:t></w:r></w:p>',
    );

    const output = fillDocxTemplate(template, {
      buyer_name: 'Maria',
      header_literal: 'NÃO SUBSTITUIR',
    });
    const headerText = readPartText(output, 'word/header4.xml');

    expect(readDocumentText(output)).toContain('Maria');
    expect(headerText).toContain('{{header_literal}}');
    expect(headerText).not.toContain('NÃO SUBSTITUIR');
  });
});

describe('inspectDocxTemplate', () => {
  it('extracts variables from logical DOCX text, including split runs and loops', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t></w:r><w:r><w:t>{buyer.name}}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{#buyers}}{{name}}{{/buyers}}</w:t></w:r></w:p>',
    );

    const inspection = inspectDocxTemplate(template);

    expect(inspection).toMatchObject({ valid: true, issues: [] });
    expect(inspection.variables).toEqual(['buyer.name', 'buyers']);
  });

  it('accepts nested JSON closing braces as literal document content', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{"provider":{"name":"Garantia","rules":{"active":true}}</w:t></w:r>' +
      '<w:r><w:t>}</w:t></w:r></w:p>',
    );

    const inspection = inspectDocxTemplate(template);

    expect(inspection).toEqual({ valid: true, variables: [], issues: [] });
  });

  it('accepts consecutive single opening braces separated by paragraphs', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{</w:t></w:r></w:p>',
    );

    expect(inspectDocxTemplate(template)).toEqual({
      valid: true,
      variables: [],
      issues: [],
    });
  });

  it('does not extract a variable whose braces are on separate lines', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{literal</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>}</w:t></w:r></w:p>',
    );

    expect(inspectDocxTemplate(template)).toEqual({
      valid: true,
      variables: [],
      issues: [],
    });
  });

  it('does not validate a loop synthesized across a Word break', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t><w:br/></w:r>' +
      '<w:r><w:t>{ # buyers }}</w:t></w:r></w:p>',
    );

    expect(inspectDocxTemplate(template)).toEqual({
      valid: true,
      variables: [],
      issues: [],
    });
  });

  it('extracts real variables without interpreting adjacent JSON braces as tags', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{partner.name}}</w:t></w:r>' +
      '<w:r><w:t> {"items":[{"meta":{"active":true}},{"meta":{"active":false}}]}</w:t></w:r></w:p>',
    );

    expect(inspectDocxTemplate(template)).toEqual({
      valid: true,
      variables: ['partner.name'],
      issues: [],
    });
  });

  it('still rejects mismatched loop tags as structural errors', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{#buyers}}{{name}}{{/sellers}}</w:t></w:r></w:p>',
    );

    const inspection = inspectDocxTemplate(template);

    expect(inspection.valid).toBe(false);
    expect(inspection.issues.map((issue) => issue.id)).toContain(
      'closing_tag_does_not_match_opening_tag',
    );
  });

  it.each([
    ['an unclosed loop', '{{#buyers}}{{name}}', 'unclosed_loop'],
    ['a loop closing tag without an opening', '{{/buyers}}', 'unopened_loop'],
  ])('still rejects %s', (_description, templateText, expectedIssue) => {
    const template = createDocxBase64(
      `<w:p><w:r><w:t>${templateText}</w:t></w:r></w:p>`,
    );

    const inspection = inspectDocxTemplate(template);

    expect(inspection.valid).toBe(false);
    expect(inspection.issues.map((issue) => issue.id)).toContain(expectedIssue);
  });

  it('still rejects an incomplete variable opening when saving a template', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{buyer_name</w:t></w:r></w:p>',
    );

    const inspection = inspectDocxTemplate(template);

    expect(inspection.valid).toBe(false);
    expect(inspection.issues.map((issue) => issue.id)).toContain('unclosed_tag');
  });

  it('extracts variables from dynamically numbered headers', () => {
    const template = createDocxWithHeaderBase64(
      '<w:p><w:r><w:t>{{buyer_name}}</w:t></w:r></w:p>',
      '<w:p><w:r><w:t>{{company_name}}</w:t></w:r></w:p>',
    );

    const inspection = inspectDocxTemplate(template);

    expect(inspection.valid).toBe(true);
    expect(inspection.variables).toEqual(expect.arrayContaining(['buyer_name', 'company_name']));
    expect(inspection.variables).toHaveLength(2);
  });

  it('keeps custom keys that start with a digit', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{2026_contract}}</w:t></w:r></w:p>',
    );

    expect(inspectDocxTemplate(template)).toMatchObject({
      valid: true,
      variables: ['2026_contract'],
    });
  });
});

describe('extractDocxTemplateVariables', () => {
  it('finds stale custom metadata inside boolean sections but skips array item fields', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{#show_clause}}{{custom_clause}}{{/show_clause}}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{#buyers}}{{custom_note}} — {{name}}{{/buyers}}</w:t></w:r></w:p>',
    );

    const variables = extractDocxTemplateVariables(template, {
      show_clause: true,
      buyers: [{ name: 'Maria' }],
    });

    expect(variables).toEqual(['show_clause', 'custom_clause', 'buyers', 'custom_note']);
  });

  it('extracts variables even when a legacy template has an orphan delimiter', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{custom_clause}}Texto}}</w:t></w:r></w:p>',
    );

    expect(extractDocxTemplateVariables(template, {})).toEqual(['custom_clause']);
  });
});

describe('replaceDocxText', () => {
  it('remaps a variable whose delimiters are split across Word runs', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{</w:t></w:r><w:r><w:t>{old_</w:t></w:r><w:r><w:t>key}</w:t></w:r><w:r><w:t>}</w:t></w:r></w:p>',
    );

    const result = replaceDocxText(
      base64ToArrayBuffer(template),
      '{{old_key}}',
      '{{new_key}}',
    );
    const updatedBase64 = bufferToBase64(result.content);

    expect(result.replacements).toBe(1);
    expect(readDocumentText(updatedBase64)).toContain('{{new_key}}');
    expect(inspectDocxTemplate(updatedBase64)).toMatchObject({
      valid: true,
      variables: ['new_key'],
    });
  });

  it('maps selected text with XML entities across runs', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>Parceiros &amp; </w:t></w:r><w:r><w:t>Garantia</w:t></w:r></w:p>',
    );

    const result = replaceDocxText(
      base64ToArrayBuffer(template),
      'Parceiros & Garantia',
      '{{guarantee_partner}}',
      false,
    );
    const inspection = inspectDocxTemplate(bufferToBase64(result.content));

    expect(result.replacements).toBe(1);
    expect(inspection).toMatchObject({
      valid: true,
      variables: ['guarantee_partner'],
    });
  });

  it('maps selected text when XML uses numeric character entities', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>Parceiros &#38; </w:t></w:r><w:r><w:t>Garantia</w:t></w:r></w:p>',
    );

    const result = replaceDocxText(
      base64ToArrayBuffer(template),
      'Parceiros & Garantia',
      '{{guarantee_partner}}',
      false,
    );

    expect(result.replacements).toBe(1);
    expect(inspectDocxTemplate(bufferToBase64(result.content))).toMatchObject({
      valid: true,
      variables: ['guarantee_partner'],
    });
  });

  it('replaces text after a nested textbox paragraph', () => {
    const template = createDocxBase64(
      '<w:p>' +
      '<w:r><w:drawing><w:txbxContent><w:p><w:r><w:t>Caixa</w:t></w:r></w:p></w:txbxContent></w:drawing></w:r>' +
      '<w:r><w:t>{{old_key}}</w:t></w:r>' +
      '</w:p>',
    );

    const result = replaceDocxText(
      base64ToArrayBuffer(template),
      '{{old_key}}',
      '{{new_key}}',
    );

    expect(result.replacements).toBe(1);
    expect(readDocumentText(bufferToBase64(result.content))).toContain('Caixa{{new_key}}');
  });

  it('maps selected text containing literal quotes and apostrophes', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>Cláusula "Especial" </w:t></w:r><w:r><w:t>d\'água</w:t></w:r></w:p>',
    );

    const result = replaceDocxText(
      base64ToArrayBuffer(template),
      'Cláusula "Especial" d\'água',
      '{{special_clause}}',
      false,
    );

    expect(result.replacements).toBe(1);
    expect(inspectDocxTemplate(bufferToBase64(result.content))).toMatchObject({
      valid: true,
      variables: ['special_clause'],
    });
  });

  it('distinguishes replacing one occurrence from replacing all occurrences', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{old_key}}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{old_key}}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{old_key}}</w:t></w:r></w:p>',
    );

    const single = replaceDocxText(
      base64ToArrayBuffer(template),
      '{{old_key}}',
      '{{new_key}}',
      false,
      1,
    );
    const singleText = readDocumentText(bufferToBase64(single.content));
    const all = replaceDocxText(
      base64ToArrayBuffer(template),
      '{{old_key}}',
      '{{new_key}}',
    );
    const allText = readDocumentText(bufferToBase64(all.content));

    expect(single.replacements).toBe(1);
    expect(singleText.match(/\{\{new_key\}\}/g)).toHaveLength(1);
    expect(singleText).toContain('{{old_key}}{{new_key}}{{old_key}}');
    expect(all.replacements).toBe(3);
    expect(allText.match(/\{\{new_key\}\}/g)).toHaveLength(3);
    expect(allText).not.toContain('{{old_key}}');
  });
});

describe('replaceDocxVariable', () => {
  it('remaps scalar, regular loop and inverted loop tags together', () => {
    const template = createDocxBase64(
      '<w:p><w:r><w:t>{{ buyers }}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{ #buyers }}</w:t></w:r><w:r><w:t>{{name}}{{ /buyers }}</w:t></w:r></w:p>' +
      '<w:p><w:r><w:t>{{ ^buyers }}Sem compradores{{ /buyers }}</w:t></w:r></w:p>',
    );

    const result = replaceDocxVariable(
      base64ToArrayBuffer(template),
      'buyers',
      'tenants',
    );
    const text = readDocumentText(bufferToBase64(result.content));

    expect(result.replacements).toBe(5);
    expect(text).toContain('{{ tenants }}');
    expect(text).toContain('{{ #tenants }}{{name}}{{ /tenants }}');
    expect(text).toContain('{{ ^tenants }}Sem compradores{{ /tenants }}');
    expect(text).not.toContain('buyers');
  });
});

describe('resolveTemplateValue', () => {
  it('calculates dates without replacing own properties or ordinary dotted variables', () => {
    expect(resolveTemplateValue({ current_date: '2026-09-17' }, 'current_date_plus_30_days')).toBe('17/10/2026');
    expect(resolveTemplateValue({ current_date: '2026-09-17', current_date_plus_30_days: '' }, 'current_date_plus_30_days')).toBe('');
    expect(resolveTemplateValue({ current_date: '2026-09-17', current_date_plus_30_days: undefined }, 'current_date_plus_30_days')).toBeUndefined();
  });

  it('prefers explicit dotted overrides and otherwise traverses objects', () => {
    expect(resolveTemplateValue({ buyer: { name: 'Maria' } }, 'buyer.name')).toBe('Maria');
    expect(resolveTemplateValue({ buyer: { name: 'Maria' }, 'buyer.name': 'Ana' }, 'buyer.name')).toBe('Ana');
  });
});
