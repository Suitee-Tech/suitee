import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PreviewVariableForm } from './PreviewVariableForm';
import PizZip from 'pizzip';
import { DOCX_SCALAR_OVERRIDES_KEY, fillDocxTemplate } from '../../utils/docx';

function renderReviewedWordText(keys: string[], data: Record<string, unknown>): string {
  const zip = new PizZip();
  zip.file('[Content_Types].xml', '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>');
  zip.file('word/document.xml', `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>${keys.map((key) => `{{${key}}}`).join(' | ')}</w:t></w:r></w:p></w:body></w:document>`);
  const filled = fillDocxTemplate(zip.generate({ type: 'base64' }), data);
  return new PizZip(filled.includes(',') ? filled.split(',')[1] : filled, { base64: true })
    .file('word/document.xml')!.asText().replace(/<[^>]+>/g, '');
}

function changeDateFormat(key: string, format: 'br' | 'iso' | 'written') {
  fireEvent.change(screen.getByRole('combobox', { name: `Formato da data no contrato: ${key}` }), { target: { value: format } });
}

describe('PreviewVariableForm', () => {
  it('preserves the full automatic context and resolves dotted, false and zero values', () => {
    const onConfirm = vi.fn();
    const buyer = { name: 'Maria', email: 'maria@example.com' };

    render(
      <PreviewVariableForm
        templateVariables={['buyer.name', 'active', 'amount', 'missing']}
        autoFilledData={{
          buyer,
          active: false,
          amount: 0,
          hidden_runtime_value: 'mantido',
        }}
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onConfirm).toHaveBeenCalledWith({
      buyer,
      'buyer.name': 'Maria',
      active: false,
      amount: 0,
      missing: '______________________',
      hidden_runtime_value: 'mantido',
    });
  });

  it('allows a custom variable with no automatic value to be filled manually', () => {
    const onConfirm = vi.fn();

    render(
      <PreviewVariableForm
        templateVariables={['custom_clause']}
        autoFilledData={{}}
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText('Preencher valor...'), {
      target: { value: 'Valor informado' },
    });
    expect(screen.getByText('1 preenchidas')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));

    expect(onConfirm).toHaveBeenCalledWith({ custom_clause: 'Valor informado' });
  });

  it('preserves numeric and boolean types when reviewing automatic fields', () => {
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={['amount', 'active']}
      autoFilledData={{ amount: 0, active: false }} onConfirm={onConfirm} onCancel={vi.fn()} />);
    fireEvent.change(document.getElementById('contract-variable-amount')!, { target: { value: '15' } });
    fireEvent.change(document.getElementById('contract-variable-active')!, { target: { value: 'Sim' } });
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    expect(onConfirm).toHaveBeenCalledWith({ amount: 15, active: true });
  });

  it('keeps structured loop data while allowing a textual scalar override', () => {
    const onConfirm = vi.fn();
    const buyers = [{ email: 'maria@example.com' }];

    render(
      <PreviewVariableForm
        templateVariables={['buyers']}
        autoFilledData={{ buyers }}
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText(/Resumo textual opcional/), {
      target: { value: 'Compradores qualificados' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));

    expect(onConfirm).toHaveBeenCalledWith({
      buyers,
      [DOCX_SCALAR_OVERRIDES_KEY]: { buyers: 'Compradores qualificados' },
    });
  });

  it('does not present a textual summary as items added to an empty loop', () => {
    render(
      <PreviewVariableForm
        templateVariables={['buyers']}
        autoFilledData={{ buyers: [] }}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText(/Resumo textual opcional/), {
      target: { value: 'Maria' },
    });

    expect(screen.getByText('0 preenchidas')).toBeInTheDocument();
    expect(screen.getByText('1 pendentes')).toBeInTheDocument();
  });

  it('recalculates deadlines and aliases from the reviewed base independently of tag order', () => {
    const onConfirm = vi.fn();
    render(<PreviewVariableForm
      templateVariables={['contract_start_date_plus_1_months', 'contract_start_date_iso', 'contract_start_date']}
      autoFilledData={{ contract_start_date: '31/01/2026', contract_start_date_iso: '2026-01-31T00:00:00.000Z' }}
      onConfirm={onConfirm} onCancel={vi.fn()}
    />);

    const deadline = document.getElementById('contract-variable-contract_start_date_plus_1_months');
    expect(deadline).toHaveValue('28/02/2026');
    fireEvent.change(document.getElementById('contract-variable-contract_start_date')!, {
      target: { value: '31/03/2026' },
    });
    expect(deadline).toHaveValue('30/04/2026');
    expect(document.getElementById('contract-variable-contract_start_date_iso')).toHaveValue('2026-03-31');
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    expect(onConfirm).toHaveBeenCalledWith({
      contract_start_date: '31/03/2026', contract_start_date_iso: '2026-03-31',
      contract_start_date_plus_1_months: '30/04/2026',
    });
  });

  it('exposes a missing source date and preserves an explicitly reviewed deadline', () => {
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={['current_date_plus_30_days']}
      autoFilledData={{}} emptyValue="" onConfirm={onConfirm} onCancel={vi.fn()} />);
    expect(screen.getByText('2 pendentes')).toBeInTheDocument();
    fireEvent.change(document.getElementById('contract-variable-current_date')!, { target: { value: '18/09/2026' } });
    expect(document.getElementById('contract-variable-current_date_plus_30_days')).toHaveValue('18/10/2026');
    fireEvent.change(document.getElementById('contract-variable-current_date_plus_30_days')!, { target: { value: '20/10/2026' } });
    fireEvent.change(document.getElementById('contract-variable-current_date')!, { target: { value: '19/09/2026' } });
    expect(document.getElementById('contract-variable-current_date_plus_30_days')).toHaveValue('20/10/2026');
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    expect(onConfirm).toHaveBeenCalledWith({ current_date: '19/09/2026', current_date_plus_30_days: '20/10/2026' });
  });

  it('keeps legacy ISO-only source dates available when refreshing aliases', () => {
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={['contract_start_date_plus_1_months', 'contract_start_date_iso']}
      autoFilledData={{ contract_start_date_iso: '2026-01-31' }} onConfirm={onConfirm} onCancel={vi.fn()} />);
    expect(document.getElementById('contract-variable-contract_start_date')).toHaveValue('31/01/2026');
    expect(document.getElementById('contract-variable-contract_start_date_plus_1_months')).toHaveValue('28/02/2026');
    expect(screen.getByText('3 preenchidas')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    expect(onConfirm).toHaveBeenCalledWith({ contract_start_date: '31/01/2026',
      contract_start_date_iso: '2026-01-31', contract_start_date_plus_1_months: '28/02/2026' });
  });

  it('renders the reviewed date into a real Word document using the digital workflow renderer', () => {
    const zip = new PizZip();
    zip.file('[Content_Types].xml', '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>');
    zip.file('word/document.xml', '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Vencimento: {{current_date_plus_30_days}}</w:t></w:r></w:p></w:body></w:document>');
    const template = zip.generate({ type: 'base64' });
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={['current_date_plus_30_days']}
      autoFilledData={{ current_date: '18/09/2026' }} onConfirm={onConfirm} onCancel={vi.fn()} />);
    fireEvent.change(document.getElementById('contract-variable-current_date')!, { target: { value: '20/09/2026' } });
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    const filled = fillDocxTemplate(template, onConfirm.mock.calls[0][0]);
    const payload = filled.includes(',') ? filled.split(',')[1] : filled;
    const xml = new PizZip(payload, { base64: true }).file('word/document.xml')!.asText();
    expect(xml).toContain('Vencimento: 20/10/2026');
    expect(xml).not.toContain('{{current_date');
  });

  it.each([
    ['default', '2029-03-18'],
    ['br', '18/03/2029'],
    ['written', '18 de março de 2029'],
  ] as const)('applies %s output to an existing ISO token in the actual Word document', (format, expected) => {
    const key = 'contract_end_date_iso';
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={[key]} autoFilledData={{ [key]: '2029-03-18' }}
      onConfirm={onConfirm} onCancel={vi.fn()} />);

    expect(screen.getByRole('combobox', { name: `Formato da data no contrato: ${key}` })).toHaveValue('iso');
    if (format !== 'default') changeDateFormat(key, format);
    expect(screen.getByLabelText(`Data no contrato: ${key}`)).toHaveTextContent(expected);
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));

    const reviewed = onConfirm.mock.calls[0][0];
    expect(reviewed[key]).toBe(expected);
    expect(reviewed).not.toHaveProperty(DOCX_SCALAR_OVERRIDES_KEY);
    expect(renderReviewedWordText([key], reviewed)).toBe(expected);
  });

  it('keeps a formatted deadline automatic when its source date is reviewed', () => {
    const key = 'current_date_plus_30_days_iso';
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={[key]} autoFilledData={{ current_date: '18/09/2026' }}
      onConfirm={onConfirm} onCancel={vi.fn()} />);

    changeDateFormat(key, 'written');
    expect(screen.getByLabelText(`Data no contrato: ${key}`)).toHaveTextContent('18 de outubro de 2026');
    fireEvent.change(document.getElementById('contract-variable-current_date')!, { target: { value: '20/09/2026' } });
    expect(document.getElementById(`contract-variable-${key}`)).toHaveValue('2026-10-20');
    expect(screen.getByLabelText(`Data no contrato: ${key}`)).toHaveTextContent('20 de outubro de 2026');
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));

    expect(renderReviewedWordText([key], onConfirm.mock.calls[0][0])).toBe('20 de outubro de 2026');
  });

  it('formats the base by extenso after calculating dependent dates from its canonical value', () => {
    const deadline = 'current_date_plus_30_days_iso';
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={[deadline, 'current_date']}
      autoFilledData={{ current_date: '18/09/2026' }} onConfirm={onConfirm} onCancel={vi.fn()} />);

    changeDateFormat('current_date', 'written');
    fireEvent.change(document.getElementById('contract-variable-current_date')!, { target: { value: '20/09/2026' } });
    expect(screen.getByLabelText(`Data no contrato: ${deadline}`)).toHaveTextContent('2026-10-20');
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));

    const reviewed = onConfirm.mock.calls[0][0];
    expect(reviewed.current_date).toBe('20 de setembro de 2026');
    expect(reviewed[deadline]).toBe('2026-10-20');
    expect(renderReviewedWordText(['current_date', deadline], reviewed)).toBe('20 de setembro de 2026 | 2026-10-20');
  });

  it('converts a manually reviewed deadline without applying its offset a second time', () => {
    const key = 'current_date_plus_30_days';
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={[key]} autoFilledData={{ current_date: '18/09/2026' }}
      onConfirm={onConfirm} onCancel={vi.fn()} />);

    fireEvent.change(document.getElementById(`contract-variable-${key}`)!, { target: { value: '20/10/2026' } });
    changeDateFormat(key, 'written');
    fireEvent.change(document.getElementById('contract-variable-current_date')!, { target: { value: '19/09/2026' } });
    expect(screen.getByLabelText(`Data no contrato: ${key}`)).toHaveTextContent('20 de outubro de 2026');
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));

    expect(renderReviewedWordText([key], onConfirm.mock.calls[0][0])).toBe('20 de outubro de 2026');
  });

  it('preserves opaque reviewed deadline text across all date output formats', () => {
    const key = 'current_date_plus_30_days_iso';
    const onConfirm = vi.fn();
    const manualText = 'Pagamento após vistoria';
    render(<PreviewVariableForm templateVariables={[key]} autoFilledData={{ current_date: '18/09/2026' }}
      onConfirm={onConfirm} onCancel={vi.fn()} />);

    fireEvent.change(document.getElementById(`contract-variable-${key}`)!, { target: { value: manualText } });
    for (const format of ['br', 'written', 'iso'] as const) {
      changeDateFormat(key, format);
      expect(screen.getByLabelText(`Data no contrato: ${key}`)).toHaveTextContent(manualText);
    }
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    expect(renderReviewedWordText([key], onConfirm.mock.calls[0][0])).toBe(manualText);
  });

  it.each(['iso', 'written'] as const)('switches formats repeatedly from an automatic default %s date', (initialFormat) => {
    const key = `contract_end_date_${initialFormat}`;
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={[key]}
      autoFilledData={{ contract_end_date: '18/03/2029', contract_end_date_iso: '2029-03-18', contract_end_date_written: '18 de março de 2029' }}
      onConfirm={onConfirm} onCancel={vi.fn()} />);

    expect(screen.getByRole('combobox', { name: `Formato da data no contrato: ${key}` })).toHaveValue(initialFormat);
    for (const [format, expected] of [
      ['iso', '2029-03-18'], ['br', '18/03/2029'], ['written', '18 de março de 2029'],
      ['iso', '2029-03-18'], ['written', '18 de março de 2029'], ['br', '18/03/2029'],
    ] as const) {
      changeDateFormat(key, format);
      expect(screen.getByLabelText(`Data no contrato: ${key}`)).toHaveTextContent(expected);
    }
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    expect(renderReviewedWordText([key], onConfirm.mock.calls[0][0])).toBe('18/03/2029');
  });

  it.each([
    ['iso', '2026-10-20'], ['written', '20 de outubro de 2026'],
  ] as const)('uses the default selected %s format for a manually reviewed date', (format, expected) => {
    const key = `current_date_plus_30_days_${format}`;
    const onConfirm = vi.fn();
    render(<PreviewVariableForm templateVariables={[key]} autoFilledData={{ current_date: '18/09/2026' }}
      onConfirm={onConfirm} onCancel={vi.fn()} />);

    fireEvent.change(document.getElementById(`contract-variable-${key}`)!, { target: { value: '20/10/2026' } });
    expect(screen.getByLabelText(`Data no contrato: ${key}`)).toHaveTextContent(expected);
    fireEvent.click(screen.getByRole('button', { name: 'Gerar Preview' }));
    expect(renderReviewedWordText([key], onConfirm.mock.calls[0][0])).toBe(expected);
  });
});
