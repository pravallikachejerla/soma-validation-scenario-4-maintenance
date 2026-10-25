import { test, expect } from '@playwright/test';

// Verifies the operator can browse the batch dashboard.
test('operator can browse batch dashboard', async ({ page }) => {
  await page.goto('/batch');
  await expect(page.getByRole('heading', { name: /batch/i })).toBeVisible();
});
