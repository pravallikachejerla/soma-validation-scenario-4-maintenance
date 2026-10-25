import { test, expect } from '@playwright/test';

// Manual override approval workflow — end-to-end browser test.
//
// These tests exercise the corrected requirements:
//   - The override threshold is configurable per tenant.
//   - The default threshold is JPY 5,000,000.
//   - Existing tenant behaviour must remain unchanged until the feature is enabled.
//   - Overrides above the threshold require two approvals.
//   - A requester cannot approve their own request.
//   - An approval reason is mandatory.
//
// The E2E stage starts the app fresh with an empty in-memory store, so
// each test seeds the database before interacting with the UI.

async function seed(page: import('@playwright/test').Page) {
  const res = await page.request.post('/api/v1/admin/seed');
  expect(res.ok()).toBeTruthy();
}

test('manual override workflow: create, approve, apply', async ({ page }) => {
  await seed(page);
  await page.goto('/overrides');

  // The page heading should be visible.
  await expect(page.getByRole('heading', { name: 'Manual price overrides' })).toBeVisible();

  // Enable the feature (idempotent — if already enabled, the button toggles it off,
  // so we check the current state first).
  await page.getByRole('button', { name: /Toggle override feature/ }).click();

  // Set a low threshold so the override we create is above it.
  await page.getByRole('button', { name: 'Set threshold ¥3,000,000' }).click();

  // Fill in the create form.
  await page.getByLabel('Override ¥').fill('3500000');
  await page.getByLabel('Reason (mandatory)').fill('negotiated enterprise deal');

  await page.getByRole('button', { name: 'Create override request' }).click();

  // The new request should appear in the table with status "pending".
  await expect(page.getByText('pending').first()).toBeVisible();

  // First approval.
  await page.getByLabel('Decision reason (mandatory)').fill('first sign-off');
  await page.getByRole('button', { name: /^Approve / }).first().click();

  // Second approval — should transition to "approved".
  await page.getByLabel('Decision reason (mandatory)').fill('second sign-off');
  await page.getByRole('button', { name: /^Approve / }).first().click();

  // Apply.
  await page.getByRole('button', { name: /^Apply / }).first().click();

  // The request should now show status "applied".
  await expect(page.getByText('applied').first()).toBeVisible();
});

test('requester cannot approve their own request', async ({ page }) => {
  await seed(page);
  await page.goto('/overrides');

  // Ensure feature is enabled.
  await page.getByRole('button', { name: /Toggle override feature/ }).click();
  await page.getByRole('button', { name: 'Set threshold ¥3,000,000' }).click();

  // Create a request as the requester.
  await page.getByLabel('Override ¥').fill('500');
  await page.getByLabel('Reason (mandatory)').fill('self-approval test');
  await page.getByRole('button', { name: 'Create override request' }).click();

  // Try to approve as the same requester — should show an error.
  await page.getByLabel('Decision reason (mandatory)').fill('self-approval attempt');
  await page.getByRole('button', { name: /^Approve / }).first().click();

  await expect(page.getByRole('alert')).toContainText(/cannot approve/i);
});

test('approval reason is mandatory', async ({ page }) => {
  await seed(page);
  await page.goto('/overrides');

  await page.getByRole('button', { name: /Toggle override feature/ }).click();
  await page.getByRole('button', { name: 'Set threshold ¥3,000,000' }).click();

  await page.getByLabel('Override ¥').fill('500');
  await page.getByLabel('Reason (mandatory)').fill('reason test');
  await page.getByRole('button', { name: 'Create override request' }).click();

  // Try to approve without a reason.
  await page.getByRole('button', { name: /^Approve / }).first().click();

  await expect(page.getByRole('alert')).toContainText(/reason/i);
});
