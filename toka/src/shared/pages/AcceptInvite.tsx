import React, { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { motion } from 'motion/react';
import { Mail, User, Lock, Building2, ChevronRight, AlertCircle, CheckCircle } from 'lucide-react';
import { getInvitation, acceptPublicInvitation } from '../services/auth.service';
import { setAccessToken } from '../services/api';
import suiteeLogo from "../../assets/logo.png";
import { Button } from '../ui/button';
import { Spinner } from '../ui/spinner';

export const AcceptInvite = () => {
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token');
  const navigate = useNavigate();

  const [loading, setLoading] = useState(true);
  const [invite, setInvite] = useState<any>(null);
  const [pageError, setPageError] = useState('');
  const [formError, setFormError] = useState('');
  const [isRegistered, setIsRegistered] = useState(false);
  
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [password, setPassword] = useState('');
  const [accepting, setAccepting] = useState(false);

  useEffect(() => {
    if (!token) {
      setPageError('O token do convite não foi fornecido ou é inválido.');
      setLoading(false);
      return;
    }

    getInvitation(token)
      .then((data) => {
        setInvite(data);
        if (data.is_registered) {
          setIsRegistered(true);
        }
      })
      .catch((err) => {
        setPageError('O link do convite é inválido ou já expirou.');
      })
      .finally(() => {
        setLoading(false);
      });
  }, [token]);

  const handleAccept = async (e: React.FormEvent) => {
    e.preventDefault();
    setAccepting(true);
    setFormError('');

    try {
      if (token) {
        const data = await acceptPublicInvitation(token, {
          firstname: firstName,
          lastname: lastName,
          password: password,
        });

        setAccessToken(data.access_token);
        window.location.href = '/';
      }
    } catch (err: any) {
      let errorMessage = err.response?.data?.message || 'Falha ao aceitar convite. Tente novamente mais tarde.';
      
      const lowerError = errorMessage.toLowerCase();
      if (lowerError.includes('invalid credentials') || lowerError.includes('password does not match')) {
        errorMessage = 'Credenciais inválidas. Verifique seus dados e tente novamente.';
      } else if (lowerError.includes('password and firstname are required')) {
        errorMessage = 'A senha e o nome são obrigatórios para criar sua conta.';
      } else if (lowerError.includes('password is required')) {
        errorMessage = 'A senha é obrigatória para acessar sua conta existente.';
      } else if (lowerError.includes('invitation is not pending')) {
        errorMessage = 'Este convite já foi aceito ou não está mais pendente.';
      } else if (lowerError.includes('invitation expired')) {
        errorMessage = 'Este convite já expirou.';
      } else if (lowerError.includes('invalid invitation token')) {
        errorMessage = 'O link do convite é inválido.';
      }
      
      setFormError(errorMessage);
    } finally {
      setAccepting(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="flex flex-col items-center space-y-4">
          <Spinner className="size-12 text-primary" />
          <p className="text-muted-foreground font-medium">Carregando convite...</p>
        </div>
      </div>
    );
  }

  if (pageError || !invite) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background py-12 px-4 sm:px-6 lg:px-8">
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="max-w-md w-full space-y-8 bg-card p-8 rounded-2xl shadow-xl border border-border text-center"
        >
          <div className="mx-auto flex items-center justify-center h-16 w-16 rounded-full bg-destructive/10">
            <AlertCircle className="h-8 w-8 text-destructive" />
          </div>
          <h2 className="mt-6 text-3xl font-extrabold text-foreground">Ops!</h2>
          <p className="mt-2 text-sm text-muted-foreground">{pageError}</p>
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
    <div className="min-h-screen flex flex-col md:flex-row bg-background">
      {/* Left Side - Form */}
      <div className="flex-1 flex flex-col justify-center px-4 sm:px-6 lg:px-20 xl:px-24">
        <div className="mx-auto w-full max-w-sm lg:max-w-md">
          <motion.div
            initial={{ opacity: 0, y: -20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.5 }}
            className="flex flex-col items-center sm:items-start"
          >
            <img src={suiteeLogo} alt="Suitee Logo" className="h-8 w-auto mb-8" />
            <h2 className="text-3xl font-extrabold text-foreground tracking-tight">
              Você foi convidado!
            </h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {isRegistered 
                ? `Aceite o convite para acessar a empresa como ` 
                : `Crie sua conta para acessar como `}
              <span className="font-bold text-primary-600 capitalize">{invite.role}</span>.
            </p>
          </motion.div>

          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.5, delay: 0.1 }}
            className="mt-8"
          >
            {formError && (
              <div className="mb-4 p-3 bg-destructive/10 border border-destructive/20 text-destructive text-sm rounded-lg flex items-center gap-2">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <p className="font-medium">{formError}</p>
              </div>
            )}

            <form onSubmit={handleAccept} className="space-y-5">
              <div>
                <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">E-mail (Convite)</label>
                <div className="relative">
                  <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
                  <input
                    disabled
                    value={invite.email}
                    className="w-full pl-10 pr-4 py-3 bg-muted border border-border rounded-xl text-sm outline-none cursor-not-allowed font-medium text-muted-foreground"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                {!isRegistered && (
                  <>
                    <div>
                      <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Nome</label>
                      <div className="relative">
                        <User className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
                        <input
                          name="firstName"
                          type="text"
                          required={!isRegistered}
                          placeholder="Ex: João"
                          className="w-full pl-10 pr-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium"
                          value={firstName}
                          onChange={(e) => setFirstName(e.target.value)}
                        />
                      </div>
                    </div>

                    <div>
                      <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Sobrenome</label>
                      <input
                        name="lastName"
                        type="text"
                        required={!isRegistered}
                        placeholder="Ex: Silva"
                        className="w-full px-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium"
                        value={lastName}
                        onChange={(e) => setLastName(e.target.value)}
                      />
                    </div>
                  </>
                )}
              </div>

              <div>
                <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">
                  {isRegistered ? 'Sua Senha' : 'Crie uma Senha'}
                </label>
                <div className="relative">
                  <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
                  <input
                    name="password"
                    type="password"
                    required
                    placeholder={isRegistered ? 'Sua senha atual' : 'Mínimo de 8 caracteres'}
                    className="w-full pl-10 pr-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                </div>
                {isRegistered && (
                  <p className="mt-2 text-xs text-muted-foreground">
                    Como você já possui uma conta, insira sua senha para confirmar sua identidade e ingressar na empresa.
                  </p>
                )}
              </div>

              <Button
                type="submit"
                loading={accepting}
                loadingText="Processando..."
                size="lg"
                className="w-full mt-6"
              >
                {isRegistered ? 'Aceitar Convite e Acessar' : 'Criar conta e Acessar'}
                <ChevronRight className="h-4 w-4" />
              </Button>
            </form>
          </motion.div>
        </div>
      </div>

      {/* Right Side - Visual */}
      <div className="hidden md:flex flex-1 bg-muted relative overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-br from-primary-50 to-primary-100/50" />
        
        {/* Decorative elements */}
        <div className="absolute -top-24 -right-24 w-96 h-96 bg-primary-400/10 rounded-full blur-3xl" />
        <div className="absolute -bottom-24 -left-24 w-96 h-96 bg-primary-500/10 rounded-full blur-3xl" />
        
        <div className="relative w-full flex flex-col justify-center items-center p-12 text-center">
          <motion.div
            initial={{ opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ duration: 0.6, delay: 0.2 }}
            className="w-full max-w-md bg-card/60 backdrop-blur-xl border border-border/50 p-8 rounded-3xl shadow-2xl"
          >
            <div className="w-16 h-16 bg-primary-100 rounded-2xl flex items-center justify-center mx-auto mb-6 shadow-inner">
              <Building2 className="h-8 w-8 text-primary-600" />
            </div>
            <h3 className="text-2xl font-bold text-foreground mb-2">
              Junte-se à Equipe
            </h3>
            <p className="text-muted-foreground mb-8 leading-relaxed">
              Você está a um passo de acessar a plataforma imobiliária que vai transformar o seu dia a dia.
            </p>
            
            <div className="space-y-4">
              {[
                "Gestão inteligente de Leads",
                "Controle completo de Imóveis",
                "Acompanhamento Financeiro",
                "Comunicação Integrada"
              ].map((feature, i) => (
                <motion.div
                  key={i}
                  initial={{ opacity: 0, x: -20 }}
                  animate={{ opacity: 1, x: 0 }}
                  transition={{ duration: 0.4, delay: 0.4 + i * 0.1 }}
                  className="flex items-center space-x-3 bg-card/80 p-3 rounded-xl border border-border/50"
                >
                  <CheckCircle className="h-5 w-5 text-status-ok flex-shrink-0" />
                  <span className="text-sm font-semibold text-foreground">{feature}</span>
                </motion.div>
              ))}
            </div>
          </motion.div>
        </div>
      </div>
    </div>
  );
};
