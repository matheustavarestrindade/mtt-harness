import { expect, test, type Page } from '@playwright/test';
import { mockHarness } from './api-fixture';
import type { DeviceLogin, Model, Provider } from '../src/lib/atoms/types';

async function providerFixture(page: Page) {
  const harness = await mockHarness(page);
  const keys = new Map<string, string>();
  const modelIDs: Record<string, string[]> = {
    openai: ['gpt-6-luna', 'gpt-6-sol'],
    deepseek: ['deepseek-flash', 'deepseek-v4-pro'],
    'openai-codex': ['gpt-5.5', 'gpt-6-sol'],
    test: ['test-model'],
  };
  const modelErrors = new Set<string>();
  let codingPlan = false;
  let approve = false;
  let login: DeviceLogin | null = null;
  const providers = (): Provider[] =>
    ['openai', 'deepseek', 'openai-codex', 'test'].map((Name) => ({
      Name,
      APIURL: '',
      ModelListURL: '',
      PriceTableURL: '',
      Interval: 0,
      Protocol: 'responses',
      Authentication: Name === 'openai-codex' ? 'chatgpt' : Name === 'test' ? 'none' : 'api_key',
      Connected: Name === 'test' || (Name === 'openai-codex' ? codingPlan : keys.has(Name)),
      ModelCount: modelIDs[Name].length,
    }));
  await page.route(
    (url) => url.pathname.startsWith('/api/providers'),
    async (route) => {
      const request = route.request();
      const parts = new URL(request.url()).pathname.split('/').filter(Boolean).slice(1);
      const reply = (body: unknown, status = 200) =>
        route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
      if (request.headers().authorization !== 'Bearer test-token')
        return reply({ error: 'unauthorized' }, 401);
      if (parts.length === 1) return reply(providers());
      const name = parts[1];
      if (parts[2] === 'key') {
        if (request.method() === 'PUT') {
          keys.set(name, request.postDataJSON().key);
          return reply({ status: 'stored' });
        }
        keys.delete(name);
        return reply({ status: 'deleted' });
      }
      if (parts[2] === 'models' || parts[2] === 'refresh') {
        if (parts[2] === 'models' && modelErrors.has(name))
          return reply({ error: 'model catalog unavailable' }, 502);
        if (keys.get(name) === 'invalid-key' && parts[2] === 'refresh')
          return reply({ error: 'provider: model list returned 401' }, 400);
        return reply(
          modelIDs[name].map((ID) => ({
            ID,
            Level: 1,
            Input: ['text'],
            Output: ['text'],
            Tools: true,
            ContextMax: 128000,
            Prices: null,
          })),
        );
      }
      if (parts[2] === 'auth' && parts.length === 3) {
        codingPlan = false;
        return reply({ status: 'disconnected' });
      }
      if (parts[3] === 'device' && parts.length === 4) {
        login = {
          ID: 'login-1',
          Provider: 'openai-codex',
          VerificationURL: 'https://auth.openai.com/codex/device',
          UserCode: 'TEST-CODE',
          ExpiresAt: new Date(Date.now() + 900000).toISOString(),
          Status: 'pending',
          Error: '',
        };
        return reply(login, 202);
      }
      if (login && parts[4] === login.ID) {
        if (request.method() === 'DELETE') login.Status = 'cancelled';
        else if (approve) {
          codingPlan = true;
          login.Status = 'connected';
          login.UserCode = '';
        }
        return reply(login);
      }
      return reply({ error: 'unknown provider route' }, 404);
    },
  );
  return {
    ...harness,
    keys,
    modelIDs,
    modelErrors,
    approve: () => {
      approve = true;
    },
  };
}

test('offer every available model and submit model IDs rather than display names', async ({
  page,
}, testInfo) => {
  const fixture = await providerFixture(page);
  fixture.keys.set('deepseek', 'deepseek-example-key');
  const models: Model[] = [
    {
      ID: 'deepseek-flash',
      Name: 'DeepSeek-V4.1-Flash',
      Level: 1,
      Input: ['text', 'image'],
      Output: ['text'],
      Tools: true,
      ContextMax: 1000000,
      Prices: null,
    },
    // Missing/false capability metadata must not silently hide a returned ID.
    {
      ID: 'deepseek-v4-pro',
      Name: 'DeepSeek-V4-Pro-0813',
      Level: 2,
      Input: ['text'],
      Output: ['text'],
      Tools: false,
      ContextMax: 0,
      Prices: null,
    },
  ];
  await page.route(
    (url) => url.pathname === '/api/providers/deepseek/models',
    (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(models) }),
  );
  await page.route(
    (url) => /^\/api\/instances\/[^/]+\/models$/.test(url.pathname),
    (route) =>
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify(models.map((model) => ({ ...model, ID: `deepseek/${model.ID}` }))),
      }),
  );
  await openProviders(page, testInfo.project.name === 'mobile');
  await expect(page.getByRole('region', { name: 'DeepSeek models', exact: true })).toContainText(
    'DeepSeek-V4.1-Flash',
  );
  await page.getByRole('dialog').getByRole('button', { name: 'Close', exact: true }).click();
  await page.getByRole('button', { name: 'Create workspace', exact: true }).click();
  const workspaceModel = page.getByLabel('Default model');
  await expect(workspaceModel.locator('option')).toHaveCount(3);
  await expect(workspaceModel).toContainText('DeepSeek-V4.1-Flash (deepseek/deepseek-flash)');
  await expect(workspaceModel).toContainText('DeepSeek-V4-Pro-0813 (deepseek/deepseek-v4-pro)');
  await workspaceModel.selectOption('deepseek/deepseek-v4-pro');
  await page.getByLabel('Project directory').fill('/workspace/example-project');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Create workspace', exact: true })
    .click();
  await expect(page.getByRole('button', { name: 'Create session', exact: true })).toBeVisible();
  expect(fixture.instances[0].DefaultModel).toBe('deepseek/deepseek-v4-pro');
  const sessionModel = page.getByRole('dialog').getByLabel('Model', { exact: true });
  await expect(sessionModel.locator('option')).toHaveCount(2);
  await expect(sessionModel).toHaveValue('deepseek/deepseek-v4-pro');
  await sessionModel.selectOption('deepseek/deepseek-flash');
  await page.getByRole('button', { name: 'Create session', exact: true }).click();
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
  expect(fixture.sessions[0].Model).toBe('deepseek/deepseek-flash');
});

async function openProviders(page: Page, mobile: boolean) {
  await page.goto('/');
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  await page.getByLabel('API token', { exact: true }).fill('test-token');
  await page.getByRole('button', { name: 'Connect to API', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  if (mobile) await page.getByRole('button', { name: 'Open navigation' }).click();
  if (mobile)
    await page
      .getByRole('dialog')
      .getByRole('button', { name: /^Settings/ })
      .click();
  else await page.getByRole('button', { name: 'Open settings', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Providers', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Providers', exact: true })).toBeVisible();
}

test('configure API providers, refresh catalogs, and remove a key', async ({ page }, testInfo) => {
  const fixture = await providerFixture(page);
  await openProviders(page, testInfo.project.name === 'mobile');
  const openai = page.getByRole('region', { name: 'OpenAI API setup', exact: true });
  await expect(
    openai.getByRole('region', { name: 'OpenAI model names', exact: true }).getByRole('listitem'),
  ).toHaveText(['openai/gpt-6-luna', 'openai/gpt-6-sol']);
  await expect(
    page
      .getByRole('region', { name: 'OpenAI coding plan model names', exact: true })
      .getByRole('listitem'),
  ).toHaveText(['openai-codex/gpt-5.5', 'openai-codex/gpt-6-sol']);
  await expect(
    page.getByRole('region', { name: 'Test provider model names', exact: true }),
  ).toContainText('test/test-model');
  await openai.getByLabel('OpenAI API key').fill('openai-example-key');
  await openai.getByRole('button', { name: 'Save API key' }).click();
  await expect(openai.getByText('Configured', { exact: true })).toBeVisible();
  await expect(openai.getByLabel('OpenAI API key')).toHaveValue('');
  expect(fixture.keys.get('openai')).toBe('openai-example-key');
  const deepseek = page.getByRole('region', { name: 'DeepSeek API setup', exact: true });
  await expect(deepseek.getByRole('listitem')).toHaveText([
    'deepseek/deepseek-flash',
    'deepseek/deepseek-v4-pro',
  ]);
  await deepseek.getByLabel('DeepSeek API key').fill('deepseek-example-key');
  await deepseek.getByRole('button', { name: 'Save API key' }).click();
  await expect(deepseek.getByText('Configured', { exact: true })).toBeVisible();
  const newModel = `coding-model-${'long-name-'.repeat(15)}`;
  fixture.modelIDs.deepseek = ['deepseek-flash', newModel];
  await deepseek.getByRole('button', { name: 'Refresh models', exact: true }).click();
  await expect(deepseek.getByRole('listitem')).toHaveText([
    'deepseek/deepseek-flash',
    `deepseek/${newModel}`,
  ]);
  expect(
    await page.evaluate(() =>
      JSON.stringify({ ...localStorage, ...sessionStorage }).includes('example-key'),
    ),
  ).toBe(false);
  await openai.getByRole('button', { name: 'Disconnect', exact: true }).click();
  await expect(openai.getByText('Not connected', { exact: true })).toBeVisible();
  expect(fixture.keys.has('openai')).toBe(false);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('providers.png'), fullPage: true });
  await page.getByRole('dialog').getByRole('button', { name: 'Close', exact: true }).click();
  await page.getByRole('button', { name: 'Create workspace', exact: true }).click();
  await expect(page.getByLabel('Default model')).toContainText('deepseek/deepseek-flash');
  await expect(page.getByLabel('Default model')).not.toContainText('openai/gpt-6-luna');
});

test('show catalog failures independently and distinguish an empty model list', async ({
  page,
}, testInfo) => {
  const fixture = await providerFixture(page);
  fixture.modelErrors.add('deepseek');
  await openProviders(page, testInfo.project.name === 'mobile');
  const deepseek = page.getByRole('region', { name: 'DeepSeek models', exact: true });
  await expect(deepseek.getByRole('alert')).toContainText(
    'Could not load model names: model catalog unavailable',
  );
  await expect(page.getByRole('region', { name: 'OpenAI model names', exact: true })).toContainText(
    'openai/gpt-6-sol',
  );
  fixture.modelErrors.delete('deepseek');
  fixture.modelIDs.deepseek = [];
  await page.getByRole('button', { name: 'Reload providers', exact: true }).click();
  await expect(deepseek.getByText('No models in the catalog.', { exact: true })).toBeVisible();
  await expect(deepseek.getByRole('alert')).toHaveCount(0);
});

test('complete a coding-plan sign-in and show key validation errors', async ({
  page,
}, testInfo) => {
  const fixture = await providerFixture(page);
  await openProviders(page, testInfo.project.name === 'mobile');
  await page.getByRole('button', { name: 'Sign in with ChatGPT' }).click();
  await expect(page.getByLabel('One-time sign-in code')).toHaveValue('TEST-CODE');
  await expect(page.getByRole('link', { name: 'Open OpenAI sign-in' })).toHaveAttribute(
    'href',
    'https://auth.openai.com/codex/device',
  );
  fixture.approve();
  const plan = page.getByRole('region', { name: 'OpenAI coding plan setup' });
  await expect(plan.getByText('Configured', { exact: true })).toBeVisible();
  await expect(page.getByLabel('One-time sign-in code')).toHaveCount(0);
  await plan.getByRole('button', { name: 'Disconnect coding plan' }).click();
  await expect(plan.getByText('Not connected', { exact: true })).toBeVisible();
  const openai = page.getByRole('region', { name: 'OpenAI API setup', exact: true });
  await openai.getByLabel('OpenAI API key').fill('invalid-key');
  await openai.getByRole('button', { name: 'Save API key' }).click();
  await expect(openai.getByRole('alert')).toContainText('Key saved, but model refresh failed');
  await expect(openai.getByLabel('OpenAI API key')).toHaveValue('');
});
