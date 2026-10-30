import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { ChevronDown, Phone, RefreshCw } from 'lucide-react';
import { ExpandableSidePanel } from '../ExpandableSidePanel';
import { Button } from '../../ui/button';
import { collectionService, type Collection } from '../../services/collection.service';
import { useCollectionCase } from '../../hooks/useDelinquencyData';
import { serverMessage } from '../../lib/serverMessage';
import { DelinquencyPanelHeader } from './DelinquencyPanelHeader';
import { PanelNotice } from './PanelNotice';
import { BODY_CLASS, FIELD_CLASS, FOOTER_CLASS, LABEL_CLASS, TEXTAREA_CLASS, formatDateTime } from './delinquencyUi';

const ACTION_TYPES = ['Ligação Telefônica', 'Mensagem WhatsApp', 'E-mail', 'Notificação Extrajudicial'];

/**
 * Aba/painel Cobrança: a timeline vem do caso COMPLETO (`GET /collections/:id`,
 * campo `actions[]`) e o registro usa `POST /collections/:id/actions` enviando
 * só `{type, description}` — o servidor deriva o autor do token.
 */
export function ProcedurePanel({
  open,
  collection,
  canWrite,
  onClose,
  onSaved,
}: {
  open: boolean;
  collection: Collection | null;
  canWrite: boolean;
  onClose: () => void;
  onSaved: (collection: Collection) => void;
}) {
  const caseId = open ? (collection?.id ?? null) : null;
  const { collection: detailed, loading, error, reload, setCollection } = useCollectionCase(caseId);
  const [actionType, setActionType] = useState(ACTION_TYPES[0]);
  const [description, setDescription] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setDescription('');
    setActionType(ACTION_TYPES[0]);
  }, [caseId]);

  const current = detailed ?? collection;
  const actions = [...(current?.actions ?? [])].sort(
    (a, b) => new Date(b.date).getTime() - new Date(a.date).getTime(),
  );

  const submit = useCallback(async () => {
    if (!caseId) return;
    if (!description.trim()) {
      toast.error('Descreva o retorno obtido no contato.');
      return;
    }
    setSaving(true);
    try {
      const returned = await collectionService.addAction(caseId, {
        type: actionType,
        description: description.trim(),
      });
      // O servidor pode devolver o caso atualizado; quando não devolve,
      // relê o caso — a tela nunca inventa a timeline.
      const updated = returned ?? (await collectionService.getById(caseId));
      setCollection(updated);
      onSaved(updated);
      toast.success('Ação registrada com sucesso!');
      if (actionType === 'Mensagem WhatsApp' && updated) {
        const template = `Olá ${updated.debtor_name}, identificamos uma pendência no valor de ${updated.total_debt.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })} referente ao contrato ${updated.contract_id}. Por favor, entre em contato conosco para regularização.`;
        toast.info('Acesse a aba de Mensagens para enviar a cobrança ao cliente.', {
          description: 'O texto sugerido foi copiado para a área de transferência.',
          duration: 8000,
        });
        navigator.clipboard?.writeText(template).catch(() => {});
      }
      setDescription('');
      onClose();
    } catch (err) {
      // 4xx/5xx: mostra a causa do servidor e mantém o painel aberto.
      toast.error(serverMessage(err, 'Erro ao registrar a ação de cobrança.'));
    } finally {
      setSaving(false);
    }
  }, [actionType, caseId, description, onClose, onSaved, setCollection]);

  return (
    <ExpandableSidePanel
      open={open}
      onClose={onClose}
      ariaLabel="Procedimento de Cobrança"
      defaultSize="lg"
      renderHeader={({ expandButton, closeButton }) => (
        <DelinquencyPanelHeader
          title="Procedimento de Cobrança"
          icon={<Phone className="w-5 h-5" />}
          contractId={current?.contract_id}
          debtorName={current?.debtor_name}
          expandButton={expandButton}
          closeButton={closeButton}
        />
      )}
    >
      <div className={BODY_CLASS}>
        <section className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-bold text-foreground">Histórico de ações do caso</h3>
            <Button variant="ghost" size="sm" onClick={reload} loading={loading} loadingText="Carregando...">
              <RefreshCw className="w-4 h-4" /> Atualizar
            </Button>
          </div>
          {error && <PanelNotice tone="critical">{error}</PanelNotice>}
          {!error && actions.length === 0 && (
            <p className="text-sm text-muted-foreground">
              {loading ? 'Carregando histórico…' : 'Nenhuma ação registrada para este caso até agora.'}
            </p>
          )}
          <ol className="space-y-3">
            {actions.map((action, index) => (
              <li key={`${action.date}-${index}`} className="bg-muted/60 border border-border rounded-xl p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-bold text-foreground">{action.type}</span>
                  <span className="text-xs text-muted-foreground">{formatDateTime(action.date)}</span>
                </div>
                <p className="text-sm text-muted-foreground mt-1 whitespace-pre-line">{action.description}</p>
                {action.user_id && <p className="text-xs text-muted-foreground mt-2">Registrado por {action.user_id}</p>}
              </li>
            ))}
          </ol>
        </section>

        {canWrite ? (
          <section className="space-y-4 pt-4 border-t border-border">
            <h3 className="text-sm font-bold text-foreground">Registrar novo procedimento</h3>
            <div>
              <label className={LABEL_CLASS} htmlFor="procedure-type">
                Tipo de Procedimento *
              </label>
              <div className="relative">
                <select
                  id="procedure-type"
                  value={actionType}
                  onChange={(event) => setActionType(event.target.value)}
                  className={`${FIELD_CLASS} appearance-none`}
                >
                  {ACTION_TYPES.map((type) => (
                    <option key={type} value={type}>
                      {type}
                    </option>
                  ))}
                </select>
                <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
              </div>
            </div>
            <div>
              <label className={LABEL_CLASS} htmlFor="procedure-description">
                Resposta/Retorno *
              </label>
              <textarea
                id="procedure-description"
                rows={4}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="Descreva o retorno obtido no contato..."
                className={TEXTAREA_CLASS}
              />
            </div>
          </section>
        ) : (
          <PanelNotice>
            Seu perfil não tem <strong>charges:write</strong>: o histórico é somente leitura e o registro de
            procedimentos está indisponível.
          </PanelNotice>
        )}
      </div>

      {canWrite && (
        <div className={FOOTER_CLASS}>
          <Button variant="outline" onClick={onClose} className="flex-1">
            Cancelar
          </Button>
          <Button onClick={submit} loading={saving} loadingText="Registrando..." className="flex-2">
            Registrar Procedimento
          </Button>
        </div>
      )}
    </ExpandableSidePanel>
  );
}
