import api, { unwrapApiData } from './api';

export type ReminderTriggerType = 'BEFORE_DUE' | 'ON_DUE' | 'AFTER_DUE';
export type ReminderChannel = 'email' | 'in_app' | 'whatsapp';

export interface ReminderRule {
  id: string;
  tenant_id: string;
  name: string;
  description?: string;
  trigger_type: ReminderTriggerType;
  offset_days: number;
  channels: ReminderChannel[];
  template: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateReminderRuleDTO {
  name: string;
  description?: string;
  trigger_type: ReminderTriggerType;
  offset_days: number;
  channels: ReminderChannel[];
  template: string;
  enabled?: boolean;
}

export interface UpdateReminderRuleDTO {
  name?: string;
  description?: string;
  trigger_type?: ReminderTriggerType;
  offset_days?: number;
  channels?: ReminderChannel[];
  template?: string;
  enabled?: boolean;
}

export interface RunNowResult {
  tenant_id: string;
  rules_evaluated: number;
  charges_matched: number;
  notifies_sent: number;
  notify_skipped: number;
  errors?: string[];
}

export const reminderRuleService = {
  list: async (): Promise<ReminderRule[]> => {
    const res = await api.get<any>('/reminder-rules');
    return unwrapApiData<ReminderRule[]>(res) || [];
  },
  getById: async (id: string): Promise<ReminderRule> => {
    const res = await api.get<any>(`/reminder-rules/${id}`);
    return unwrapApiData<ReminderRule>(res);
  },
  create: async (data: CreateReminderRuleDTO): Promise<ReminderRule> => {
    const res = await api.post<any>('/reminder-rules', data);
    return unwrapApiData<ReminderRule>(res);
  },
  update: async (id: string, data: UpdateReminderRuleDTO): Promise<ReminderRule> => {
    const res = await api.put<any>(`/reminder-rules/${id}`, data);
    return unwrapApiData<ReminderRule>(res);
  },
  remove: async (id: string): Promise<void> => {
    await api.delete<any>(`/reminder-rules/${id}`);
  },
  runNow: async (): Promise<RunNowResult> => {
    const res = await api.post<any>('/reminder-rules/run-now', {});
    return unwrapApiData<RunNowResult>(res);
  },
};

