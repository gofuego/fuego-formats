import { test, expect } from '@playwright/test';

/* Golden fixture: nested describes, @tags in titles, declaration and in-body
   annotations, and every dynamic-title degradation the v1 scanner promises. */

test.describe('Checkout @checkout', () => {
  test('guest can pay with card @smoke', async ({ page }) => {
    await page.goto('/checkout');
    await expect(page.getByRole('button', { name: 'Pay' })).toBeVisible();
  });

  test.skip('applies gift card', async ({ page }) => {
    // Not implemented yet.
  });

  test.describe('Discounts', () => {
    test('rejects an expired coupon @Validation', async ({ page }) => {
      test.slow();
      await page.goto('/checkout?coupon=EXPIRED');
    });

    test.fixme('stacks seasonal discounts', async () => {});
  });
});

test.describe.skip('Legacy flow', () => {
  test('redirects to the old cart', async () => {});
});

test('top-level smoke @smoke', async ({ page }) => {
  await page.goto('/');
});

// Dynamic titles degrade to best-effort text, never a parse failure.
const region = 'emea';
test(`prices load for ${region}`, async () => {});

for (const currency of ['usd', 'eur']) {
  test('formats ' + currency, async () => {});
}
