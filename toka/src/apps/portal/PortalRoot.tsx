import { Outlet } from "react-router";
import { RouteTitle } from "../../shared/components/RouteTitle";
import { PortalProviders } from "./PortalProviders";

export function PortalRoot() {
  return (
    <PortalProviders>
      <RouteTitle />
      <Outlet />
    </PortalProviders>
  );
}
