// Troca única do refresh token, compartilhada por todos os caminhos do app.
//
// Três lugares renovam a sessão: o interceptor do httpClient (ao ver um 401), o
// boot do AuthProvider (aba aberta sem token em memória) e o
// SessionRecoveryProvider (volta do foco depois de muito tempo parado). Se dois
// deles dispararem juntos, o segundo `/auth/refresh` chega com o cookie que o
// primeiro acabou de rotacionar — e o Supabase trata reuso de refresh token como
// vazamento, derrubando a sessão do usuário.
//
// Aqui a renovação é single-flight de verdade (uma promise compartilhada) e os
// interessados podem observar "tem renovação em andamento" para mostrar
// carregamento em vez de erro enquanto ela acontece.
//
// A frota tem DOIS ciclos de sessão independentes — o app admin
// (`/auth/refresh`) e o portal do cliente (`/auth/portal/refresh`) — que
// NUNCA podem compartilhar o mesmo single-flight: um refresh de portal
// chegando no meio de um refresh do admin (ou vice-versa) reusaria o cookie
// do outro e derrubaria a sessão. Cada ciclo cria o próprio hub com
// `createSessionRefreshHub`; as exportações singleton abaixo são o hub do
// app admin (compatibilidade).
type SessionRefreshListener = (active: boolean) => void;

export type SessionRefreshHub = {
  /** Há renovação de sessão em andamento neste momento? */
  isActive: () => boolean;
  /** Observa o começo/fim de uma renovação de sessão. */
  subscribe: (listener: SessionRefreshListener) => () => void;
  /**
   * Executa `work` uma única vez por vez. Chamadas concorrentes recebem a MESMA
   * promise (a primeira é quem de fato troca o refresh token).
   */
  run: <T>(work: () => Promise<T>) => Promise<T>;
  /** Só para testes: zera o estado do hub entre casos. */
  resetForTests: () => void;
};

export function createSessionRefreshHub(): SessionRefreshHub {
  let inFlight: Promise<unknown> | null = null;
  let active = false;
  const listeners = new Set<SessionRefreshListener>();

  function notify(next: boolean) {
    active = next;
    listeners.forEach((listener) => {
      try {
        listener(next);
      } catch {
        // Um listener com defeito não pode derrubar a renovação da sessão.
      }
    });
  }

  function run<T>(work: () => Promise<T>): Promise<T> {
    if (inFlight) return inFlight as Promise<T>;

    notify(true);
    const promise = (async () => {
      try {
        return await work();
      } finally {
        inFlight = null;
        notify(false);
      }
    })();

    inFlight = promise;
    return promise;
  }

  return {
    isActive: () => active,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    run,
    resetForTests: () => {
      inFlight = null;
      active = false;
      listeners.clear();
    },
  };
}

// Hub padrão do app admin. O portal cria o próprio hub em
// apps/portal/services/portal-api.ts (ciclo de sessão independente).
const adminHub = createSessionRefreshHub();

export function isSessionRefreshActive(): boolean {
  return adminHub.isActive();
}

export function subscribeSessionRefresh(listener: SessionRefreshListener): () => void {
  return adminHub.subscribe(listener);
}

export function runSessionRefresh<T>(work: () => Promise<T>): Promise<T> {
  return adminHub.run(work);
}

export function resetSessionRefreshForTests() {
  adminHub.resetForTests();
}
