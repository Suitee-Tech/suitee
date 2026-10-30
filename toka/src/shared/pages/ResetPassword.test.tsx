import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';

// P1/A14 — a tela de redefinição tem UM caminho: a sessão de recuperação do
// Supabase (fragmento `#access_token=…&type=recovery`). O caminho por `?token=`
// foi removido junto com os endpoints locais `/auth/reset-password[/verify]`,
// que nunca conseguiam validar nada (`passwordResetRepo.Create` não era chamado).

const getRecoverySessionFromUrl = vi.fn();
const resetPasswordWithRecoverySession = vi.fn();
const navigate = vi.fn();

vi.mock('../services/auth.service', () => ({
  getRecoverySessionFromUrl: (...args: unknown[]) => getRecoverySessionFromUrl(...args),
  resetPasswordWithRecoverySession: (...args: unknown[]) => resetPasswordWithRecoverySession(...args),
}));

vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return { ...actual, useNavigate: () => navigate };
});

import { ResetPassword } from './ResetPassword';

const renderPage = (initialUrl = '/reset-password') =>
  render(
    <MemoryRouter initialEntries={[initialUrl]}>
      <ResetPassword />
    </MemoryRouter>,
  );

describe('ResetPassword (P1/A14)', () => {
  beforeEach(() => {
    getRecoverySessionFromUrl.mockReset();
    resetPasswordWithRecoverySession.mockReset();
    navigate.mockReset();
  });

  afterEach(() => vi.clearAllMocks());

  it('usa a sessão de recuperação do link e troca a senha pelo provedor', async () => {
    getRecoverySessionFromUrl.mockResolvedValue({ accessToken: 'acc-token', refreshToken: 'ref-token' });
    resetPasswordWithRecoverySession.mockResolvedValue(undefined);

    renderPage();
    await waitFor(() => expect(getRecoverySessionFromUrl).toHaveBeenCalled());

    const password = await screen.findByPlaceholderText('Mínimo de 8 caracteres');
    const confirm = screen.getByPlaceholderText('Repita a nova senha');
    await userEvent.type(password, 'NovaSenha1!');
    await userEvent.type(confirm, 'NovaSenha1!');
    await userEvent.click(screen.getByRole('button', { name: /salvar nova senha/i }));

    await waitFor(() =>
      expect(resetPasswordWithRecoverySession).toHaveBeenCalledWith('acc-token', 'ref-token', 'NovaSenha1!', undefined, undefined, undefined),
    );
  });

  it('aceita o token_hash do link próprio e o envia ao backend', async () => {
    getRecoverySessionFromUrl.mockResolvedValue({ accessToken: '', refreshToken: '', tokenHash: 'hash-abc', expiresAt: 1893456000, signature: 'sig-abc' });
    resetPasswordWithRecoverySession.mockResolvedValue(undefined);

    renderPage();
    await waitFor(() => expect(getRecoverySessionFromUrl).toHaveBeenCalled());

    const password = await screen.findByPlaceholderText('Mínimo de 8 caracteres');
    const confirm = screen.getByPlaceholderText('Repita a nova senha');
    await userEvent.type(password, 'NovaSenha1!');
    await userEvent.type(confirm, 'NovaSenha1!');
    await userEvent.click(screen.getByRole('button', { name: /salvar nova senha/i }));

    await waitFor(() =>
      expect(resetPasswordWithRecoverySession).toHaveBeenCalledWith('', '', 'NovaSenha1!', 'hash-abc', 1893456000, 'sig-abc'),
    );
  });

  it('envia expiração e assinatura do link assinado ao backend', async () => {
    getRecoverySessionFromUrl.mockResolvedValue({
      accessToken: '',
      refreshToken: '',
      tokenHash: 'hash-abc',
      expiresAt: 1893456000,
      signature: 'sig-abc',
    });
    resetPasswordWithRecoverySession.mockResolvedValue(undefined);

    renderPage();
    await waitFor(() => expect(getRecoverySessionFromUrl).toHaveBeenCalled());

    const password = await screen.findByPlaceholderText('Mínimo de 8 caracteres');
    const confirm = screen.getByPlaceholderText('Repita a nova senha');
    await userEvent.type(password, 'NovaSenha1!');
    await userEvent.type(confirm, 'NovaSenha1!');
    await userEvent.click(screen.getByRole('button', { name: /salvar nova senha/i }));

    await waitFor(() =>
      expect(resetPasswordWithRecoverySession).toHaveBeenCalledWith(
        '',
        '',
        'NovaSenha1!',
        'hash-abc',
        1893456000,
        'sig-abc',
      ),
    );
  });

  it('não tenta mais o caminho por ?token= (removido no A14)', async () => {
    getRecoverySessionFromUrl.mockResolvedValue(null);

    renderPage('/reset-password?token=abc123');
    await waitFor(() => expect(getRecoverySessionFromUrl).toHaveBeenCalled());

    // Com token na URL e sem sessão de recuperação, a tela acusa link inválido —
    // e não existe chamada alguma ao backend local de reset.
    expect(await screen.findByText(/não é mais válido/i)).toBeTruthy();
    expect(resetPasswordWithRecoverySession).not.toHaveBeenCalled();
  });
});
