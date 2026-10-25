import { test, expect } from '@playwright/test';

test.describe('Manual override workflow', () => {
  test('settings page loads and shows default threshold', async ({ page }) => {
    await page.goto('/overrides');
    await expect(page.getByRole('heading', { name: 'Manual price overrides' })).toBeVisible();
    await expect(page.getByText('Default threshold JPY 5,000,000')).toBeVisible();
  });

  test('create override request below threshold auto-applies', async ({ page }) => {
    await page.goto('/overrides');
    // Enable overrides first
    await page.getByLabel('Override enabled').check();
    await page.getByRole('button', { name: 'Save settings' }).click();
    // Fill form with small override (below 5M threshold)
    await page.getByLabel('Base (JPY)').fill('10000');
    await page.getByLabel('Override (JPY)').fill('8000');
    await page.getByLabel('Reason (mandatory)').first().fill('VIP retention');
    await page.getByRole('button', { name: 'Submit override request' }).click();
    await expect(page.getByText(/Created override request/)).toBeVisible();
  });

  test('test identities are displayed', async ({ page }) => {
    await page.goto('/overrides');
    await expect(page.getByText('alice.requester')).toBeVisible();
    await expect(page.getByText('bob.approver')).toBeVisible();
    await expect(page.getByText('carol.approver')).toBeVisible();
    await expect(page.getByText('dave.admin')).toBeVisible();
  });
});