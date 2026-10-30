import { test, expect, type Page } from "@playwright/test";
async function seed(
  page: Page,
  status: "pending" | "active",
  failure = false,
  incomingProposal = false,
  missingFeePayer = false,
) {
  await page.addInitScript(() => localStorage.setItem("auth_has_session", "1"));
  const writes: Array<{ path: string; body: any }> = [];
  const partnership: any = {
    version: 1,
    companies: {
      "company-other": {
        id: "company-other",
        name: "Imobiliária Parceira",
        tax_id: "12.345.678/0001-90",
        city: "Recife",
        state: "PE",
        active: true,
      },
    },
    grants: {
      "company-other": {
        sharing_config: {
          share_owner_contact: false,
          share_lead_contact: false,
          allow_co_brokering: true,
          shared_property_types: [],
          shared_regions: [],
        },
        permissions: [{ scope: "shared_portfolio", access_level: "view" }],
      },
      "company-1": {
        sharing_config: {
          share_owner_contact: false,
          allow_co_brokering: false,
        },
        permissions: null,
      },
    },
    id: "partnership-qa",
    requester_id: "company-other",
    target_id: "company-1",
    type: "partner",
    status,
    financial_config: {
      fee_payer: "closing_pays_source",
      royalty_percentage: 10,
      admin_fee: 100,
      transaction_fee: 50,
      commission_model: { owner_company_pct: 60, closing_company_pct: 40 },
    },
    sharing_config: {
      share_owner_contact: false,
      allow_co_brokering: false,
      shared_property_types: [],
      shared_regions: [],
    },
    permissions: [{ scope: "shared_portfolio", access_level: "view" }],
    audit_logs: [],
  };
  if (missingFeePayer) delete partnership.financial_config.fee_payer;
  if (incomingProposal)
    partnership.pending_financial = {
      proposed_by: "company-other",
      version: 1,
      financial_config: {
        ...partnership.financial_config,
        commission_model: { owner_company_pct: 70, closing_company_pct: 30 },
      },
    };
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
    const method = route.request().method();
    if (path === "/client-errors") {
      await route.fulfill({ status: 204 });
      return;
    }
    if (path === "/companies/company-other") {
      await route.fulfill({
        status: 404,
        json: { message: "company not found" },
      });
      return;
    }
    if (method === "PUT" && path.startsWith("/partnerships/")) {
      const body = route.request().postDataJSON();
      writes.push({ path, body });
      if (failure) {
        await route.fulfill({
          status: 409,
          json: {
            message: "O acordo foi alterado. Recarregue a versão atual.",
          },
        });
        return;
      }
      if (body.expected_version !== partnership.version) {
        await route.fulfill({
          status: 409,
          json: { message: "Versão desatualizada" },
        });
        return;
      }
      if (body.action === "accept") partnership.status = "active";
      if (body.action === "suspend") partnership.status = "suspended";
      if (body.action === "terminate") partnership.status = "inactive";
      if (body.grant) partnership.grants["company-1"] = body.grant;
      if (
        body.action === "save" &&
        body.financial_config?.commission_model.owner_company_pct !== 60
      )
        partnership.pending_financial = {
          proposed_by: "company-1",
          version: partnership.version + 1,
          financial_config: body.financial_config,
        };
      if (body.action === "accept_financial") {
        partnership.financial_config =
          partnership.pending_financial.financial_config;
        delete partnership.pending_financial;
      }
      partnership.version++;
      await route.fulfill({ json: { success: true, data: partnership } });
      return;
    }
    let data: unknown = [];
    const user = {
      id: "fake-user-id",
      email: "qa@example.test",
      firstname: "Ana",
      lastname: "QA",
      role: "user",
      functional_role: "admin",
      permission: "owner",
      account_type: "individual",
      current_company_id: "company-1",
    };
    if (/^\/auth\/(refresh|switch-)/.test(path))
      data = { access_token: "synthetic-qa-token" };
    if (path === "/auth/me") data = user;
    if (path === "/auth/companies")
      data = [
        {
          id: "company-1",
          type: "company",
          company: {
            id: "company-1",
            name: "Imobiliária QA",
            type: "imobiliaria",
            is_active: true,
            owner_id: user.id,
            members: [{ account_id: user.id, profile: "owner" }],
          },
        },
      ];
    if (path === "/me/permissions")
      data = { functional_role: "admin", profile: "owner", permissions: [] };
    if (path === "/settings/preferences")
      data = { consent: { terms_version: "2.0" } };
    if (path === "/partnerships") data = [partnership];
    await route.fulfill({
      json: {
        success: true,
        data,
        meta: { total: Array.isArray(data) ? data.length : 1 },
      },
    });
  });
  return writes;
}

for (const width of [390, 1440]) {
  test(`convite exige revisão e aceita somente as concessões da própria empresa ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    const writes = await seed(page, "pending");
    await page.goto("/parceiros");
    await expect(
      page.getByRole("heading", { name: "Imobiliária Parceira", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Revisar convite", exact: true })
      .click();
    const panel = page.getByRole("dialog");
    await expect(
      panel.getByText("Condições financeiras do convite"),
    ).toBeVisible();
    await expect(panel.getByText(/60% \/ 40%/)).toBeVisible();
    expect(writes).toHaveLength(0);
    await panel
      .getByRole("switch", { name: "Compartilhar portfólio", exact: true })
      .click();
    await panel
      .getByRole("button", {
        name: "Aceitar condições e parceria",
        exact: true,
      })
      .click();
    await page
      .getByRole("alertdialog")
      .getByRole("button", {
        name: "Aceitar condições e parceria",
        exact: true,
      })
      .click();
    await expect.poll(() => writes.length).toBe(1);
    expect(writes[0].path).toBe("/partnerships/partnership-qa/agreement");
    expect(writes[0].body).toMatchObject({
      action: "accept",
      expected_version: 1,
      grant: {
        permissions: [{ scope: "shared_portfolio", access_level: "view" }],
      },
    });
    await expect(
      page.getByRole("button", { name: "Ver portfólio", exact: true }),
    ).toBeEnabled();
  });
  test(`erro mantém edição e envia somente uma operação ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    const writes = await seed(page, "active", true);
    await page.goto("/parceiros");
    await page
      .getByRole("button", { name: "Gerenciar acordo", exact: true })
      .click();
    const panel = page.getByRole("dialog");
    await panel
      .getByRole("switch", {
        name: "Compartilhar contato de leads",
        exact: true,
      })
      .click();
    await panel
      .getByRole("button", { name: "Salvar acordo", exact: true })
      .click();
    await expect(
      panel.getByText("O acordo foi alterado. Recarregue a versão atual."),
    ).toBeVisible();
    await expect(
      panel.getByRole("switch", {
        name: "Compartilhar contato de leads",
        exact: true,
      }),
    ).toBeChecked();
    expect(writes).toHaveLength(1);
    expect(writes[0].body).toMatchObject({
      action: "save",
      expected_version: 1,
      grant: { sharing_config: { share_lead_contact: true } },
    });
    await page.screenshot({
      path: `../docs/diagnostics/evidence/2026-09-27-parcerias/acordo-validado-${width}.png`,
      fullPage: true,
    });
  });
  test(`suspensão exige motivo e revoga acesso na tela ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    const writes = await seed(page, "active");
    await page.goto("/parceiros");
    await page
      .getByRole("button", { name: "Gerenciar acordo", exact: true })
      .click();
    const panel = page.getByRole("dialog");
    await expect(
      panel.getByRole("button", { name: "Suspender parceria", exact: true }),
    ).toBeDisabled();
    await panel
      .getByRole("textbox", { name: "Motivo", exact: true })
      .fill("Revisão do acordo");
    await panel
      .getByRole("button", { name: "Suspender parceria", exact: true })
      .click();
    await page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Suspender parceria", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Ver portfólio", exact: true }),
    ).toBeDisabled();
    expect(writes[0].body).toMatchObject({
      action: "suspend",
      reason: "Revisão do acordo",
      expected_version: 1,
    });
  });
}

test("proposta recebida exige confirmação antes de alterar condições aceitas", async ({
  page,
}) => {
  const writes = await seed(page, "active", false, true);
  await page.goto("/parceiros");
  await page
    .getByRole("button", { name: "Gerenciar acordo", exact: true })
    .click();
  const panel = page.getByRole("dialog");
  await expect(panel.getByText(/60% \/ 40%/)).toBeVisible();
  await expect(panel.getByText(/70% \/ 30%/)).toBeVisible();
  await panel
    .getByRole("button", { name: "Aceitar proposta financeira", exact: true })
    .click();
  expect(writes).toHaveLength(0);
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Aceitar proposta financeira", exact: true })
    .click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].body).toMatchObject({
    action: "accept_financial",
    expected_version: 1,
  });
  await page
    .getByRole("button", { name: "Gerenciar acordo", exact: true })
    .click();
  await expect(page.getByRole("dialog").getByText(/70% \/ 30%/)).toBeVisible();
  await expect(
    page.getByRole("dialog").getByText("Proposta financeira aguardando aceite"),
  ).toHaveCount(0);
});

test("taxas sem responsável bloqueiam gravação até a escolha explícita", async ({
  page,
}) => {
  const writes = await seed(page, "active", false, false, true);
  await page.goto("/parceiros");
  await page
    .getByRole("button", { name: "Gerenciar acordo", exact: true })
    .click();
  const panel = page.getByRole("dialog");
  await expect(
    panel.getByRole("button", { name: "Salvar acordo", exact: true }),
  ).toBeDisabled();
  await panel
    .getByLabel("Quem paga as taxas?", { exact: true })
    .selectOption("source_pays_closing");
  await expect(
    panel.getByText(/não descontam a comissão nem o repasse/),
  ).toBeVisible();
  await panel
    .getByRole("button", { name: "Salvar acordo", exact: true })
    .click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].body.financial_config.fee_payer).toBe("source_pays_closing");
  expect(writes[0].body.action).toBe("save");
});
