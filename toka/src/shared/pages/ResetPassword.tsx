import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { motion } from 'motion/react';
import { Lock, AlertCircle, CheckCircle2, ChevronRight, XCircle } from 'lucide-react';
import { getRecoverySessionFromUrl, resetPasswordWithRecoverySession, type RecoverySession } from '../services/auth.service';
import suiteeLogo from "../../assets/logo.png";
import { Button } from '../ui/button';
import { Spinner } from '../ui/spinner';

// P1/A14 — a tela tem UM caminho de redefinição: a sessão de recuperação do
// Supabase (fragmento `#access_token=…&type=recovery` do link enviado por
// e-mail). O caminho antigo por `?token=` (token local em `toca_auth.
// password_resets`) foi removido: o backend nunca criava esse token, então a
// tela só sabia dizer "link inválido" — e manter um segundo emissor de token de
// recuperação seria uma credencial mais fraca que a do provedor.
export const ResetPassword = () => {
  const navigate = useNavigate();
  const [recoverySession, setRecoverySession] = useState<RecoverySession | null>(null);

  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [verifying, setVerifying] = useState(true);
  const [validToken, setValidToken] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    getRecoverySessionFromUrl()
      .then((sess) => {
        if (sess) {
          setRecoverySession(sess);
          setValidToken(true);
        } else {
          setValidToken(false);
        }
      })
      .catch(() => {
        setValidToken(false);
      })
      .finally(() => setVerifying(false));
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password !== confirmPassword) {
      setError('As senhas não coincidem');
      return;
    }

    setLoading(true);
    setError('');
    
    try {
      if (recoverySession) {
        await resetPasswordWithRecoverySession(
          recoverySession.accessToken,
          recoverySession.refreshToken,
          password,
          recoverySession.tokenHash,
          recoverySession.expiresAt,
          recoverySession.signature,
        );
        // O token já foi consumido; remove o segredo da URL/histórico.
        window.history.replaceState(null, '', window.location.pathname + window.location.search);
        setSuccess(true);
      }
    } catch (err: any) {
      setError(err.response?.data?.message || err.message || 'Falha ao redefinir a senha');
    } finally {
      setLoading(false);
    }
  };

  if (verifying) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="flex flex-col items-center space-y-4">
          <Spinner className="size-12 text-primary" />
          <p className="text-muted-foreground font-medium">Verificando link de recuperação...</p>
        </div>
      </div>
    );
  }

  if (!validToken && !success) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background py-12 px-4 sm:px-6 lg:px-8">
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="max-w-md w-full space-y-8 bg-card p-8 rounded-2xl shadow-xl border border-border text-center"
        >
          <div className="mx-auto flex items-center justify-center h-16 w-16 rounded-full bg-status-critical-soft">
            <XCircle className="h-8 w-8 text-status-critical" />
          </div>
          <h2 className="mt-6 text-2xl font-bold text-foreground">Link Inválido ou Expirado</h2>
          <p className="mt-2 text-sm text-muted-foreground">Este link de recuperação de senha não é mais válido. Por favor, solicite um novo link.</p>
          <div className="mt-6">
            <Button onClick={() => navigate('/login')} size="lg" className="w-full">
              Voltar para o Login
            </Button>
          </div>
        </motion.div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background py-12 px-4 sm:px-6 lg:px-8">
      <div className="absolute top-8 left-8 hidden sm:block">
        <img src={suiteeLogo} alt="Suitee Logo" className="h-8 w-auto" />
      </div>
      
      <motion.div 
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="max-w-md w-full space-y-8 bg-card p-8 rounded-2xl shadow-xl border border-border"
      >
        <div className="text-center">
          <div className="mx-auto flex items-center justify-center h-16 w-16 rounded-full bg-primary-50 mb-6">
            <Lock className="h-8 w-8 text-primary-600" />
          </div>
          <h2 className="text-2xl font-bold text-foreground mb-2">
            Criar nova senha
          </h2>
          <p className="text-sm text-muted-foreground">
            Sua nova senha deve ser diferente das senhas usadas anteriormente.
          </p>
        </div>

        {success ? (
          <motion.div 
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            className="text-center space-y-6 mt-8"
          >
            <div className="mx-auto flex items-center justify-center h-16 w-16 rounded-full bg-status-ok-soft text-status-ok mb-4">
              <CheckCircle2 className="h-8 w-8" />
            </div>
            <h3 className="text-xl font-bold text-foreground">Senha redefinida!</h3>
            <p className="text-sm text-muted-foreground">Sua senha foi alterada com sucesso. Você já pode acessar sua conta com a nova senha.</p>
            <Button onClick={() => navigate('/login')} size="lg" className="w-full mt-6">
              Fazer Login
            </Button>
          </motion.div>
        ) : (
          <form className="mt-8 space-y-5" onSubmit={handleSubmit}>
            {error && (
              <div className="p-3 bg-status-critical-soft border border-status-critical/30 text-status-critical text-sm rounded-lg flex items-center gap-2">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <p className="font-medium">{error}</p>
              </div>
            )}
            
            <div>
              <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Nova Senha</label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
                <input
                  name="password"
                  type="password"
                  required
                  placeholder="Mínimo de 8 caracteres"
                  className="w-full pl-10 pr-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Confirmar Nova Senha</label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
                <input
                  name="confirmPassword"
                  type="password"
                  required
                  placeholder="Repita a nova senha"
                  className="w-full pl-10 pr-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                />
              </div>
            </div>

            <Button
              type="submit"
              loading={loading}
              loadingText="Salvando..."
              size="lg"
              className="w-full mt-6"
            >
              Salvar nova senha
              <ChevronRight className="h-4 w-4" />
            </Button>
          </form>
        )}
      </motion.div>
    </div>
  );
};
