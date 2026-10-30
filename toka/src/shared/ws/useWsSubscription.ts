import { useEffect } from "react";
import { wsManager, type WSEvent } from "./wsManager";

export function useWsSubscription(eventType: string, handler: (event: WSEvent) => void) {
  useEffect(() => {
    const id = wsManager.subscribe(eventType, handler);
    return () => wsManager.unsubscribe(eventType, id);
  }, [eventType, handler]);
}
