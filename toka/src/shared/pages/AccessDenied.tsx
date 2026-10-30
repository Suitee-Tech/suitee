import { ShieldAlert } from "lucide-react";
import { useNavigate } from "react-router";
import { Button } from "../ui/button";

export default function AccessDenied() {
  const navigate = useNavigate();

  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] text-center p-6">
      <ShieldAlert className="w-16 h-16 text-destructive mb-6" />
      <h1 className="text-2xl font-bold tracking-tight mb-2">Acesso Negado</h1>
      <p className="text-muted-foreground max-w-md mb-8">
        Você não tem permissão para visualizar esta página. Se você acredita que isso é um erro, entre em contato com o administrador da sua conta.
      </p>
      <Button onClick={() => navigate(-1)} variant="outline">
        Voltar
      </Button>
    </div>
  );
}

