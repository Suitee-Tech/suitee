import { expect, test } from '@playwright/test';

test('production artifact renders a public route without page errors', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  await page.route(/\/api\/v1\/auth\/refresh\b.*/, (route) =>
    route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({
        success: false,
        statusCode: 401,
        message: 'No seeded browser session',
      }),
    }),
  );

  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Bem-vindo de volta!' })).toBeVisible();
  expect(pageErrors).toEqual([]);
});
