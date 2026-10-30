import { useEffect, useState } from 'react';
import { Plus, PlayCircle, Edit2, Trash2, Loader2, ToggleLeft, ToggleRight, Bell } from 'lucide-react';
import { toast } from 'sonner';
import { Card } from '../ui/card';
import { Button } from '../ui/button';
import { ReminderRuleModal } from '../components/ReminderRuleModal';
import { reminderRuleService, type ReminderRule } from '../services/reminderRule.service';
import { FinanceAreaNav } from '../components/finance/FinanceAreaNav';
import { useConfirmAction } from '../hooks/useConfirmAction';

const TRIGGER_LABEL: Record<ReminderRule['trigger_type'], string> = {
  BEFORE_DUE: 'Antes do vencimento',
  ON_DUE: 'No dia do vencimento',
  AFTER_DUE: 'Após vencimento',
};

export default function Regua() {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const [rules, setRules] = useState<ReminderRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<ReminderRule | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [running, setRunning] = useState(false);

  const refresh = async () => {
    setLoading(true);
    try {
      const data = await reminderRuleService.list();
      setRules(data);
    } catch (err) {
      console.error(err);
      toast.error('Não foi possível carregar as regras.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  const handleToggle = async (rule: ReminderRule) => {
    try {
      await reminderRuleService.update(rule.id, { enabled: !rule.enabled });
      toast.success(rule.enabled ? 'Regra desativada' : 'Regra ativada');
      refresh();
    } catch (err) {
      console.error(err);
      toast.error('Erro ao atualizar regra.');
    }
  };

  const handleDelete = (rule: ReminderRule) => {
    confirmAction(
      `Remover a regra "${rule.name}"? Esta ação não pode ser desfeita.`,
      async () => {
        try {
          await reminderRuleService.remove(rule.id);
          toast.success('Regra removida');
          refresh();
        } catch (err) {
          console.error(err);
          toast.error('Erro ao remover.');
          throw err;
        }
      },
      { title: 'Remover regra', confirmLabel: 'Remover', variant: 'danger' },
    );
  };

  const handleRunNow = async () => {
    setRunning(true);
    try {
      const res = await reminderRuleService.runNow();
      toast.success(
        `Avaliadas ${res.rules_evaluated} regras • ${res.charges_matched} matches • ${res.notifies_sent} envios`,
        { duration: 6000 }
      );
    } catch (err) {
      console.error(err);
      toast.error('Erro ao executar régua.');
    } finally {
      setRunning(false);
    }
  };

  const handleModalClose = () => {
    setShowModal(false);
    setEditing(null);
    refresh();
  };

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      <FinanceAreaNav />
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Bell className="w-6 h-6 text-primary-600" />
            Régua de cobrança
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Lembretes automáticos enviados aos pagadores antes e depois do vencimento. Disparados diariamente pelo serviço.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={handleRunNow} disabled={running}>
            {running ? <Loader2 className="w-4 h-4 animate-spin" /> : <PlayCircle className="w-4 h-4" />}
            <span className="ml-2">Executar agora</span>
          </Button>
          <Button
            onClick={() => {
              setEditing(null);
              setShowModal(true);
            }}
          >
            <Plus className="w-4 h-4" />
            <span className="ml-2">Nova regra</span>
          </Button>
        </div>
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
        </div>
      ) : rules.length === 0 ? (
        <Card className="p-12 text-center">
          <Bell className="w-12 h-12 mx-auto text-muted-foreground mb-3" />
          <p className="text-sm text-muted-foreground">Nenhuma regra criada ainda.</p>
          <p className="text-xs text-muted-foreground mt-1">
            Clique em "Nova regra" para configurar o primeiro lembrete.
          </p>
        </Card>
      ) : (
        <div className="space-y-2">
          {rules.map((rule) => (
            <Card key={rule.id} className="p-4 flex items-center gap-4">
              <button
                type="button"
                onClick={() => handleToggle(rule)}
                title={rule.enabled ? 'Desativar' : 'Ativar'}
                className="text-primary-600 hover:text-primary-700 transition-colors"
              >
                {rule.enabled ? <ToggleRight className="w-7 h-7" /> : <ToggleLeft className="w-7 h-7 text-muted-foreground" />}
              </button>
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <p className="font-semibold">{rule.name}</p>
                  {!rule.enabled && (
                    <span className="text-xs px-2 py-0.5 rounded-full bg-muted text-muted-foreground">Inativa</span>
                  )}
                </div>
                <p className="text-xs text-muted-foreground">
                  {TRIGGER_LABEL[rule.trigger_type]}
                  {rule.trigger_type !== 'ON_DUE' && ` • ${rule.offset_days} dia(s)`} • Canais:{' '}
                  {rule.channels.join(', ') || '—'}
                </p>
                {rule.description && <p className="text-xs text-muted-foreground mt-1">{rule.description}</p>}
              </div>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => {
                  setEditing(rule);
                  setShowModal(true);
                }}
              >
                <Edit2 className="w-4 h-4" />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => handleDelete(rule)} aria-label="Remover regra" title="Remover regra">
                <Trash2 className="w-4 h-4 text-status-critical" />
              </Button>
            </Card>
          ))}
        </div>
      )}

      {showModal && <ReminderRuleModal rule={editing} onClose={handleModalClose} />}
      {confirmationDialog}
    </div>
  );
}
