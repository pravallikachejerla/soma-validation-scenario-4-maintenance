import { test, expect } from '@playwright/test';

// Verifies the operator can browse the products list and see seeded SKUs.
test('operator can browse products', async ({ page }) => {
  await page.goto('/products');
  await expect(page.getByRole('heading', { name: /products/i })).toBeVisible();
  // The seeded dataset includes SKU-00001; the table should render at least one row.
  await expect(page.getByText('SKU-00001').first()).toBeVisible();
});
