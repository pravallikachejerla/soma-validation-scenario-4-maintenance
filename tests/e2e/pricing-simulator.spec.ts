import { test, expect } from '@playwright/test';

// Verifies the operator can run a pricing quote and see the result.
test('operator can run a pricing quote', async ({ page }) => {
  await page.goto('/simulator');
  await expect(page.getByRole('heading', { name: /pricing simulator/i })).toBeVisible();

  // Fill the form with seeded values.
  await page.getByLabel(/sku/i).fill('SKU-00001');
  await page.getByLabel(/customer/i).fill('cust-001');
  await page.getByLabel(/quantity/i).fill('1');

  // Submit.
  await page.getByRole('button', { name: /quote|run|calculate/i }).click();

  // The result panel should show the base and final prices.
  await expect(page.getByText(/base/i).first()).toBeVisible();
  await expect(page.getByText(/final/i).first()).toBeVisible();
});
