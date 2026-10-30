import { collectPages, CollectionAbortedError } from './collectPages';
import { chargeService } from './charge.service';
import { transactionService } from './transaction.service';
import { transferService } from './transfer.service';
import { commissionService } from './commission.service';
import { payoutOrderService, type PayoutListFilter } from './payoutOrder.service';

/**
 * Leituras completas da carteira financeira.
 *
 * Todas aceitam `AbortSignal`: a coleta percorre páginas sequenciais e, sem o
 * sinal, segue varrendo a carteira inteira depois que a tela que pediu a leitura
 * já saiu. O cancelamento rejeita com `CollectionAbortedError`, distinto de
 * falha de leitura — quem trata erro não deve oferecer "tentar novamente" para
 * uma consulta cancelada de propósito.
 */
export type ListOptions = { signal?: AbortSignal };
export { CollectionAbortedError };

type Paging = { limit: number; offset: number };

export const listAllCharges = (options: ListOptions = {}) =>
  collectPages(paging => chargeService.getAll(paging), options);

export const listAllTransactions = (options: ListOptions = {}) =>
  collectPages(paging => transactionService.getAll(paging), options);

export const listAllTransfers = (options: ListOptions = {}) =>
  collectPages(paging => transferService.getAll(paging), options);

export const listAllCommissions = (options: ListOptions = {}) =>
  collectPages(({ limit, offset }: Paging) => commissionService.list(limit, offset), options);

export const listAllPayouts = (filter: PayoutListFilter = {}, options: ListOptions = {}) =>
  collectPages(async (paging: Paging) => {
    const page = await payoutOrderService.list({ ...filter, ...paging });
    return { data: page.items, total: page.total };
  }, options);
