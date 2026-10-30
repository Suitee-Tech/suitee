import { getEnvBool } from "../../lib/runtime-env";

export const flags = {
  multiPartyUI: getEnvBool("VITE_MULTI_PARTY_UI"),
};
