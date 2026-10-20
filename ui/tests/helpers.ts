import { expect, type Page, type Locator } from '@playwright/test';

export async function openWorkbenchAction(
  page: Page,
  action:
    'Tasks' | 'Memory' | 'Session activity' | 'Settings' | 'Workspace settings' | 'New session',
) {
  if (action === 'Settings' || action === 'Workspace settings') {
    await openConfiguration(page);
    await page
      .getByRole('menu', { name: 'Configuration menu', exact: true })
      .getByRole('menuitem', { name: action, exact: true })
      .click();
    await expect(page.getByRole('dialog', { name: action, exact: true })).toBeVisible();
    return;
  }
  const headerNames = {
    Tasks: 'Open tasks panel',
    Memory: 'Open memory panel',
    'Session activity': 'Session activity',
    'New session': 'New session',
  };
  const headerControl = page
    .locator('#conversation-header')
    .getByRole('button', { name: headerNames[action], exact: true });
  if (page.viewportSize()!.width >= 1024) {
    await headerControl.click();
    return;
  }
  // A closing mobile sheet is still rendered during its exit transition.
  await expect(page.locator('[data-slot="sheet-content"][data-state="closed"]')).toHaveCount(0);
  const navigation = page.getByRole('navigation', { name: 'Workspaces and sessions', exact: true });
  if (!(await navigation.isVisible()))
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  await navigation.getByRole('button', { name: action, exact: true }).click();
}

export async function openConfiguration(page: Page) {
  if (page.viewportSize()!.width >= 1024) {
    await page
      .locator('#conversation-header')
      .getByRole('button', { name: 'Configuration', exact: true })
      .click();
  } else {
    await expect(page.locator('[data-slot="sheet-content"][data-state="closed"]')).toHaveCount(0);
    const navigation = page.getByRole('navigation', {
      name: 'Workspaces and sessions',
      exact: true,
    });
    if (!(await navigation.isVisible()))
      await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
    await navigation.getByRole('button', { name: 'Configuration', exact: true }).click();
  }
  await expect(page.getByRole('menu', { name: 'Configuration menu', exact: true })).toBeVisible();
}

export async function openSessionSettings(page: Page) {
  await page
    .getByRole('form', { name: 'Message composer', exact: true })
    .getByRole('button', { name: 'Configuration', exact: true })
    .click();
  await page
    .getByRole('menu', { name: 'Configuration menu', exact: true })
    .getByRole('menuitem', { name: 'Model and thinking', exact: true })
    .click();
  await expect(page.getByRole('dialog', { name: 'Session settings', exact: true })).toBeVisible();
}

export function workbenchReturnControl(page: Page, desktopLabel: string) {
  if (page.viewportSize()!.width >= 1024 && desktopLabel === 'Open settings')
    return page
      .locator('#conversation-header')
      .getByRole('button', { name: 'Configuration', exact: true });
  return page.getByRole('button', {
    name: page.viewportSize()!.width < 1024 ? 'Open navigation' : desktopLabel,
    exact: true,
  });
}

export async function chooseSelectOption(selector: Locator, value: string) {
  const page = selector.page();
  await expect(page.getByRole('listbox')).toHaveCount(0);
  await selector.click();
  await expect(page.getByRole('listbox')).toBeVisible();
  await page
    .getByRole('option')
    .and(page.locator(`[data-option-value=${JSON.stringify(value)}]`))
    .click();
  await expect(page.getByRole('listbox')).toHaveCount(0);
}

export async function expectSelectOptions(selector: Locator, labels: string[]) {
  await selector.click();
  await expect(selector.page().getByRole('option')).toHaveText(labels);
  await selector.page().keyboard.press('Escape');
  await expect(selector.page().getByRole('listbox')).toHaveCount(0);
  await expect(selector).toBeFocused();
}

export async function connectAndCreate(page: Page) {
  await page.goto('/');
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  await page.getByLabel('API token', { exact: true }).fill('test-token');
  await page.getByRole('button', { name: 'Connect to API' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('button', { name: 'Create workspace', exact: true }).click();
  await page.getByLabel('Project directory').fill('/workspace/example-project');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Create workspace', exact: true })
    .click();
  await page.getByRole('button', { name: 'Create session', exact: true }).click();
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
}
