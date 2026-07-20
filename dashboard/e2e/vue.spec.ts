import { test, expect } from '@playwright/test'

const serviceKey = `kv2_${'A'.repeat(24)}.${'B'.repeat(42)}A`

test('starts at the production-safe credential boundary', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'Connect to a namespace' })).toBeVisible()
  const credentialInput = page.getByLabel('Service key', { exact: true })
  await expect(credentialInput).toHaveAttribute('type', 'password')
  await expect(credentialInput).toHaveAttribute('autocomplete', 'new-password')
  expect(await page.evaluate(() => localStorage.length)).toBe(0)

  await credentialInput.fill('not-a-service-key')
  await page.getByRole('button', { name: 'Open console' }).click()
  await expect(page.getByRole('alert')).toContainText('canonical Kave V2 service key')
})

test('keeps the default credential in memory and calls only the typed reporting surface', async ({
  page,
}) => {
  let reportingAuthorization = ''
  await page.route('**/kave.kernel.v2.KernelService/ListTenants', async (route) => {
    reportingAuthorization = route.request().headers().authorization ?? ''
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ tenants: [], nextPageToken: '' }),
    })
  })
  await page.goto('/')

  await page.getByLabel('Service key', { exact: true }).fill(serviceKey)
  await page.getByRole('button', { name: 'Open console' }).click()

  await expect(page.getByRole('heading', { name: 'Operations overview' })).toBeVisible()
  expect(
    await page.evaluate(() => ({ local: localStorage.length, session: sessionStorage.length })),
  ).toEqual({ local: 0, session: 0 })

  const scopeButton = page.getByRole('button', { name: /Set scope/ })
  await scopeButton.click()
  const dialog = page.getByRole('dialog', { name: 'Reporting scope' })
  await expect(dialog).toBeVisible()
  await expect(page.getByLabel('Tenant reference')).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(scopeButton).toBeFocused()

  await page.getByRole('link', { name: 'Tenants' }).click()
  await expect(page.getByRole('heading', { name: 'Tenant scopes' })).toBeVisible()
  await expect(page.getByText('No observed tenants')).toBeVisible()
  expect(reportingAuthorization).toBe(`Bearer ${serviceKey}`)
})

test('keeps navigation operable across desktop and mobile breakpoints', async ({ page }) => {
  await page.setViewportSize({ width: 800, height: 700 })
  await page.goto('/')
  await page.getByLabel('Service key', { exact: true }).fill(serviceKey)
  await page.getByRole('button', { name: 'Open console' }).click()

  const navigation = page.getByRole('navigation', { name: 'Primary navigation' })
  await expect(navigation).toBeVisible()
  await expect(navigation.locator('..')).not.toHaveAttribute('aria-hidden', 'true')

  await page.setViewportSize({ width: 600, height: 700 })
  const toggle = page.getByRole('button', { name: 'Toggle navigation' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(navigation).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
})
