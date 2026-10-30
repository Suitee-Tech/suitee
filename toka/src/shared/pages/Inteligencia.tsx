import { useNavigate } from "react-router";
import { AIChat } from "../components/ai/AIChat";
import { usePermissionsContext } from "../../lib/permissions/PermissionsContext";

// P1/A8 — a página `/inteligencia` não tinha guarda nenhuma (achado M19): qualquer
// usuário autenticado abria o assistente, mesmo com `ai:use` revogado no tenant. O
// servidor já exige a permissão nas rotas do agente; aqui a tela passa a refletir
// isso em vez de abrir um painel que só devolveria 403.
export default function Inteligencia() {
  const navigate = useNavigate();
  const { can } = usePermissionsContext();

  if (!can("ai:use")) {
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-3 bg-background p-6 text-center">
        <h1 className="text-xl font-bold text-foreground">Acesso restrito</h1>
        <p className="max-w-md text-sm text-muted-foreground">
          Você não tem a permissão <span className="font-mono">ai:use</span> nesta empresa.
          Peça ao administrador para liberar o acesso à Inteligência Artificial.
        </p>
      </div>
    );
  }

  return <AIChat isOpen={true} mode="page" onClose={() => navigate(-1)} />;
}
