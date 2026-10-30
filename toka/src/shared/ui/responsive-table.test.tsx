import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import { ResponsiveTable } from "./responsive-table";

const rows = [{ id: "1", name: "Contrato 1", value: "R$ 10,00" }];

describe("ResponsiveTable", () => {
  it("troca a tabela por cards no mobile, sem perder o conteúdo", () => {
    render(
      <ResponsiveTable
        data={rows}
        forceLayout="cards"
        caption="Contratos de locação"
        getRowKey={(row) => row.id}
        columns={[
          { key: "name", header: "Contrato", primary: true, cell: (row) => row.name },
          { key: "value", header: "Valor", cell: (row) => row.value },
        ]}
      />,
    );

    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.getByText("Contrato 1")).toBeInTheDocument();
    expect(screen.getByText("R$ 10,00")).toBeInTheDocument();
  });

  it("expõe caption e escopo de coluna no layout de tabela", () => {
    render(
      <ResponsiveTable
        data={rows}
        forceLayout="table"
        caption="Contratos de locação"
        getRowKey={(row) => row.id}
        columns={[
          { key: "name", header: "Contrato", primary: true, cell: (row) => row.name },
          { key: "value", header: "Valor", cell: (row) => row.value },
        ]}
      />,
    );

    expect(screen.getByRole("table", { name: "Contratos de locação" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Contrato" })).toHaveAttribute("scope", "col");
    expect(screen.getByRole("cell", { name: "R$ 10,00" })).toBeInTheDocument();
  });
});

describe("ResponsiveTable com grupos", () => {
  it("renderiza cabeçalho de grupo e mantém as linhas", () => {
    render(
      <ResponsiveTable
        data={[]}
        groups={[
          { key: "g1", label: "Setembro", data: rows },
          { key: "g2", label: "Outubro", data: [{ id: "2", name: "Contrato 2", value: "R$ 20,00" }] },
        ]}
        forceLayout="table"
        getRowKey={(row) => row.id}
        columns={[
          { key: "name", header: "Contrato", cell: (row) => row.name },
          { key: "value", header: "Valor", cell: (row) => row.value },
        ]}
      />,
    );

    expect(screen.getByText("Setembro")).toHaveAttribute("scope", "colgroup");
    expect(screen.getByText("Contrato 2")).toBeInTheDocument();
  });

  it("mostra grupos também no layout de cards", () => {
    render(
      <ResponsiveTable
        data={[]}
        groups={[{ key: "g1", label: "Setembro", data: rows }]}
        forceLayout="cards"
        getRowKey={(row) => row.id}
        columns={[{ key: "name", header: "Contrato", cell: (row) => row.name }]}
      />,
    );
    expect(screen.getByRole("heading", { name: "Setembro" })).toBeInTheDocument();
    expect(screen.getByText("Contrato 1")).toBeInTheDocument();
  });
});
