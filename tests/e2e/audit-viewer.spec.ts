import { test, expect } from '@playwright/test';

// Verifies the operator can browse the audit viewer.
test('operator can browse audit viewer', async ({ page }) => {
  await page.goto('/audit');
  await expect(page.getByRole('heading', { name: /audit/i })).toBeVisible();
});
