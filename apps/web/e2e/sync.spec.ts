import { expect, test, type Page } from '@playwright/test'

const note = {
  path: 'Welcome.md', title: 'Welcome', content: '# Before', contentHash: 'initial-hash',
  size: 8, modTime: '2026-01-01T00:00:00Z', access: 'write' as const,
}

type NetworkState = { offline: boolean }

async function mockAPI(page: Page, state: NetworkState, push: (body: { operations: Array<{ opId: string }> }) => { status: number; body: unknown }) {
  await page.route('**/api/**', async (route) => {
    if (state.offline) return route.abort('internetdisconnected')
    const url = new URL(route.request().url())
    const path = url.pathname
    let body: unknown = {}
    if (path === '/api/settings') body = { sync: { enabled: true }, notes: { defaultFolder: 'Inbox', typeFolders: {}, autoFrontmatter: false, showProperties: true, hiddenProperties: [], propertyLabels: {} }, vault: { templatesDir: 'Templates', attachmentsDir: 'attachments' } }
    else if (path === '/api/note/Welcome') body = note
    else if (path === '/api/tree') body = { name: '', path: '', isDir: true, children: [] }
    else if (path === '/api/notes' || path === '/api/recent' || path === '/api/plugins') body = []
    else if (path === '/api/sync/pull') body = { changes: [], nextCursor: 0, hasMore: false }
    else if (path === '/api/sync/push') {
      const result = push(JSON.parse(route.request().postData() ?? '{}'))
      await route.fulfill({ status: result.status, contentType: 'application/json', body: JSON.stringify(result.body) })
      return
    }
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) })
  })
}

async function openEditor(page: Page) {
  await page.goto('/n/Welcome')
  await expect(page.getByRole('button', { name: 'Edit' })).toBeVisible()
  await page.getByRole('button', { name: 'Edit' }).click()
  await page.locator('.cm-content').click()
  await page.locator('.cm-content').press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
  await page.locator('.cm-content').press('Backspace')
  await page.locator('.cm-content').pressSequentially('# Offline edit')
}

test('offline edit is durably queued and pushed after reconnect', async ({ page }) => {
  let pushes = 0
  const network = { offline: false }
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'onLine', { configurable: true, get: () => (window as unknown as { __syncOnline: boolean }).__syncOnline })
    ;(window as unknown as { __syncOnline: boolean }).__syncOnline = true
  })
  await mockAPI(page, network, (request) => {
    pushes++
    return { status: 200, body: { acknowledged: request.operations.map((op) => op.opId), cursor: pushes } }
  })
  await openEditor(page)
  network.offline = true
  await page.evaluate(() => { (window as unknown as { __syncOnline: boolean }).__syncOnline = false })
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Offline')).toBeVisible()
  network.offline = false
  await page.evaluate(() => {
    ;(window as unknown as { __syncOnline: boolean }).__syncOnline = true
    window.dispatchEvent(new Event('online'))
  })
  await expect.poll(() => pushes).toBe(1)
  await expect(page.getByText('Synced')).toBeVisible()
})

test('a 409 shows all three conflict actions', async ({ page }) => {
  await mockAPI(page, { offline: false }, () => ({ status: 409, body: { code: 'conflict', opId: 'conflict-op', path: 'Welcome.md', contentHash: 'server-hash', content: '# Server' } }))
  await openEditor(page)
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Save conflict')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Overwrite with my version' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Take the new version' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save my version as a copy' })).toBeVisible()
})
