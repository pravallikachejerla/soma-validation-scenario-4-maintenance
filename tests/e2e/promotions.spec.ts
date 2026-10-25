import { test, expect } from '@playwright/test';

// Verifies the operator can browse the promotions list.
test('operator can browse promotions', async ({ page }) => {
  await page.goto('/promotions');
  await expect(page.getByRole('heading', { name: /promotions/i })).toBeVisible();
  // The seeded dataset includes promo-0001; the table should render at least one row.
  await expect(page.getByText('promo-0001').first()).toBeVisible();
});
