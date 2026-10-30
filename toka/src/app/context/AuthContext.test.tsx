import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AuthProvider, useAuth } from './AuthContext';
import * as authService from '../../shared/services/auth.service';
import * as apiModule from '../../shared/services/api';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import { useState } from 'react';

// Mock auth service
vi.mock('../../shared/services/auth.service', () => ({
 login: vi.fn(),
 logout: vi.fn(),
 getCurrentUser: vi.fn(),
 getUserCompanies: vi.fn(),
 register: vi.fn(),
 switchContext: vi.fn(),
 refreshTokenRequest: vi.fn(),
}));

// Mock api module
vi.mock('../../shared/services/api', async () => {
 const actual = await vi.importActual('../../shared/services/api');
 return {
 ...actual,
 setAccessToken: vi.fn(),
 getAccessToken: vi.fn(() => null),
 };
});

// Mock window.location
const originalLocation = window.location;

const TestComponent = () => {
 const { user, isAuthenticated, login, logout, isLoading } = useAuth();
 const [error, setError] = useState('');

 const handleLogin = async () => {
 try {
 await login('test@example.com', 'password');
 } catch (err) {
 setError('Login Failed');
 }
 };

 if (isLoading) return <div>Loading...</div>;

 return (
 <div>
 <div data-testid="auth-status">{isAuthenticated ? 'Authenticated' : 'Not Authenticated'}</div>
 {user && <div data-testid="user-name">{user.fullname || `${user.firstname} ${user.lastname}`}</div>}
 {error && <div data-testid="error-message">{error}</div>}
 <button onClick={handleLogin}>Login</button>
 <button onClick={logout}>Logout</button>
 </div>
 );
};

describe('AuthContext', () => {
 beforeEach(() => {
 vi.clearAllMocks();
 localStorage.clear();
 // Reset window.location mock
 delete (window as any).location;
 (window as any).location = { href: '' };
 // Default: no existing session (silent refresh fails)
 vi.mocked(authService.refreshTokenRequest).mockRejectedValue(new Error('no session'));
 });

 afterEach(() => {
 (window as any).location = originalLocation;
 });

 it('provides initial state', async () => {
 render(
 <AuthProvider>
 <TestComponent />
 </AuthProvider>
 );

 await waitFor(() => {
 expect(screen.getByTestId('auth-status')).toHaveTextContent('Not Authenticated');
 });
 });

 it('logs in successfully', async () => {
 const mockUser = { id: '1', firstname: 'Test', lastname: 'User', fullname: 'Test User', email: 'test@example.com', role: 'admin' };
 const mockResponse = { access_token: 'new-access-token' };

 vi.mocked(authService.login).mockResolvedValue(mockResponse);
 vi.mocked(authService.getCurrentUser).mockResolvedValue(mockUser as any);
 vi.mocked(authService.getUserCompanies).mockResolvedValue([]);

 render(
 <AuthProvider>
 <TestComponent />
 </AuthProvider>
 );

 // Wait for loading to finish
 await waitFor(() => expect(screen.queryByText('Loading...')).not.toBeInTheDocument());

 const loginButton = screen.getByText('Login');
 await userEvent.click(loginButton);

 await waitFor(() => {
 expect(screen.getByTestId('auth-status')).toHaveTextContent('Authenticated');
 expect(screen.getByTestId('user-name')).toHaveTextContent('Test User');
 });

 // Token stored in memory, not localStorage
 expect(apiModule.setAccessToken).toHaveBeenCalledWith('new-access-token');
 });

 it('handles login failure', async () => {
 vi.mocked(authService.login).mockRejectedValue(new Error('Login failed'));

 render(
 <AuthProvider>
 <TestComponent />
 </AuthProvider>
 );

 await waitFor(() => expect(screen.queryByText('Loading...')).not.toBeInTheDocument());

 const loginButton = screen.getByText('Login');
 await userEvent.click(loginButton);

 await waitFor(() => {
 expect(screen.getByTestId('error-message')).toHaveTextContent('Login Failed');
 expect(screen.getByTestId('auth-status')).toHaveTextContent('Not Authenticated');
 });
 });

 it('logs out successfully', async () => {
 // Setup: silent refresh succeeds (user has an active session)
 const mockUser = { id: '1', firstname: 'Test', lastname: 'User', fullname: 'Test User', email: 'test@example.com', role: 'admin' };
 vi.mocked(authService.refreshTokenRequest).mockResolvedValue({ access_token: 'session-token' });
 vi.mocked(authService.getCurrentUser).mockResolvedValue(mockUser as any);
 vi.mocked(authService.getUserCompanies).mockResolvedValue([]);

 render(
 <AuthProvider>
 <TestComponent />
 </AuthProvider>
 );

 await waitFor(() => {
 expect(screen.getByTestId('auth-status')).toHaveTextContent('Authenticated');
 });

 const logoutButton = screen.getByText('Logout');
 await userEvent.click(logoutButton);

 await waitFor(() => {
 expect(screen.getByTestId('auth-status')).toHaveTextContent('Not Authenticated');
 });

 expect(authService.logout).toHaveBeenCalled();
 expect(window.location.href).toBe('/login');
 });
});

/**
 * Boot depois de muito tempo parado (F5/aba nova): a restauração pelo cookie de
 * refresh pode falhar de forma transitória (gateway reiniciando, timeout). Isso
 * não pode mandar o usuário para o login — tenta de novo e só desiste quando a
 * recusa é definitiva.
 */
describe('AuthContext — restauração de sessão no boot', () => {
  afterEach(() => {
    // Um provider da prova anterior pode ter um backoff em voo; zerar a
    // implementação impede que ele chame o provedor de novo e contamine a
    // próxima asserção.
    vi.mocked(authService.refreshTokenRequest).mockReset();
  });

  it('repete a renovação quando a falha é transitória e mantém o usuário logado', async () => {
    const mockUser = { id: '1', firstname: 'Test', lastname: 'User', fullname: 'Test User', email: 'test@example.com', role: 'admin' };
    vi.mocked(apiModule.getAccessToken).mockReturnValue(null);
    // Dica de sessão gravada por um login anterior (é ela que autoriza a
    // tentativa de restauração no boot).
    localStorage.setItem('auth_has_session', '1');
    vi.mocked(authService.refreshTokenRequest)
      .mockRejectedValueOnce({ response: { status: 503 } })
      .mockResolvedValueOnce({ access_token: 'session-token' } as any);
    vi.mocked(authService.getCurrentUser).mockResolvedValue(mockUser as any);
    vi.mocked(authService.getUserCompanies).mockResolvedValue([]);

    render(
      <AuthProvider>
        <TestComponent />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('Authenticated');
    });
    expect(authService.refreshTokenRequest).toHaveBeenCalledTimes(2);
  });

  it('não insiste quando a recusa é definitiva', async () => {
    vi.mocked(apiModule.getAccessToken).mockReturnValue(null);
    localStorage.setItem('auth_has_session', '1');
    vi.mocked(authService.refreshTokenRequest).mockRejectedValue({ response: { status: 400 } });

    render(
      <AuthProvider>
        <TestComponent />
      </AuthProvider>,
    );

    // Asserção no momento em que a tela se resolve: a recusa definitiva não
    // dispara nenhuma tentativa seguinte (as duas linhas abaixo medem o que
    // ESTE provider fez, ignorando backoffs de provas anteriores).
    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('Not Authenticated');
    });
    expect(authService.refreshTokenRequest).toHaveBeenCalledTimes(1);
  });
});
