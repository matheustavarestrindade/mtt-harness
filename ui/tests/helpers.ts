import { expect, type Page, type Locator } from '@playwright/test';

export async function openWorkbenchAction(
  page: Page,
  action: 'Tasks' | 'Memory' | 'Session activity' | 'Settings' | 'New session',
) {
  const headerNames = {
    Tasks: 'Open tasks panel',
    Memory: 'Open memory panel',
    'Session activity': 'Session activity',
    Settings: 'Open settings',
    'New session': 'New session',
  };
  const headerControl = page
    .locator('#conversation-header')
    .getByRole('button', { name: headerNames[action], exact: true });
  if (page.viewportSize()!.width >= 1024) {
    await headerControl.click();
    return;
  }
  const navigation = page.getByRole('navigation', { name: 'Workspaces and sessions', exact: true });
  if (!(await navigation.isVisible()))
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  await navigation.getByRole('button', { name: action, exact: true }).click();
}

export function workbenchReturnControl(page: Page, desktopLabel: string) {
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
