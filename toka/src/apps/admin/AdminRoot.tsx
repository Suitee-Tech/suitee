import { Outlet } from "react-router";
import { RouteTitle } from "../../shared/components/RouteTitle";
import { AdminProviders } from "./AdminProviders";

export function AdminRoot() {
  return (
    <AdminProviders>
      <RouteTitle />
      <Outlet />
    </AdminProviders>
  );
}

