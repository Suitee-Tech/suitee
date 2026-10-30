import React, { useState } from 'react';
import { useNavigate } from 'react-router';
import { forgotPassword } from '../services/auth.service';
import { Button } from '../ui/button';
import { Input } from '../ui/input';

export const ForgotPassword = () => {
  const [email, setEmail] = useState('');
  const [submitted, setSubmitted] = useState(false);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await forgotPassword(email);
      setSubmitted(true);
    } catch (err) {
      // Show success anyway to prevent email enumeration
      setSubmitted(true);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-background py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-md w-full space-y-8 bg-card p-8 rounded-xl shadow-lg border border-border">
        <div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-foreground">
            Recuperar senha
          </h2>
          <p className="mt-2 text-center text-sm text-muted-foreground">
            Digite seu email para receber um link de redefinição
          </p>
        </div>

        {submitted ? (
          <div className="rounded-md bg-status-ok-soft p-4">
            <div className="flex">
              <div className="ml-3">
                <h3 className="text-sm font-medium text-status-ok">Email enviado</h3>
                <div className="mt-2 text-sm text-status-ok">
                  <p>Se o email existir em nossa base, você receberá um link de recuperação em instantes.</p>
                </div>
                <div className="mt-4">
                  <Button variant="link" onClick={() => navigate('/login')} className="h-auto p-0">
                    Voltar para o login
                  </Button>
                </div>
              </div>
            </div>
          </div>
        ) : (
          <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
            <div>
              <label htmlFor="email-address" className="sr-only">Email</label>
              <Input
                id="email-address"
                name="email"
                type="email"
                required
                placeholder="Seu email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>

            <Button type="submit" loading={loading} loadingText="Enviando..." className="w-full">
              Enviar link
            </Button>

            <div className="text-center">
              <Button
                type="button"
                variant="link"
                onClick={() => navigate('/login')}
                className="h-auto p-0"
              >
                Voltar para o login
              </Button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
};
