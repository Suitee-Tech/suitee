import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { useConfirmAction } from './useConfirmAction';
function Harness({ action }: { action: () => Promise<void> }) {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  return <><button onClick={() => confirmAction('Excluir o registro selecionado?', action, { title: 'Excluir registro', confirmLabel: 'Excluir', variant: 'danger' })}>Solicitar</button>{confirmationDialog}</>;
}
describe('confirmações compartilhadas', () => {
  it('cancela com Escape e devolve o foco sem executar a ação', async () => {
    const user = userEvent.setup(); const action = vi.fn().mockResolvedValue(undefined);
    render(<Harness action={action} />);
    const opener = screen.getByRole('button', { name: 'Solicitar' });
    await user.click(opener);
    expect(screen.getByRole('alertdialog', { name: 'Excluir registro' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(action).not.toHaveBeenCalled();
    expect(opener).toHaveFocus();
  });
  it('bloqueia envio duplicado e cancelamento enquanto aguarda a operação', async () => {
    let resolve!: () => void;
    const action = vi.fn(() => new Promise<void>(done => { resolve = done; }));
    render(<Harness action={action} />);
    fireEvent.click(screen.getByText('Solicitar'));
    const confirm = screen.getByRole('button', { name: 'Excluir' });
    fireEvent.click(confirm); fireEvent.click(confirm);
    expect(action).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeDisabled();
    await act(async () => resolve());
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });
  it('mantém o erro visível e permite tentar novamente', async () => {
    const action = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined);
    render(<Harness action={action} />);
    fireEvent.click(screen.getByText('Solicitar'));
    fireEvent.click(screen.getByRole('button', { name: 'Excluir' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível concluir');
    fireEvent.click(screen.getByRole('button', { name: 'Excluir' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(action).toHaveBeenCalledTimes(2);
  });
});
