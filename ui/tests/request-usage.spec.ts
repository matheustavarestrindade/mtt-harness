import { expect, test } from '@playwright/test';
import { latestRequestUsage } from '../src/lib/atoms/request-usage';
import type { Message, Session, Usage } from '../src/lib/atoms/types';
import { mockHarness } from './api-fixture';
import { connectAndCreate, openWorkbenchAction } from './helpers';

const report: Usage = {
  Input: 30000,
  CacheRead: 10000,
  CacheWrite: 2000,
  Output: 8000,
  Reasoning: 4000,
  Cost: { Currency: 'USD', Value: 0.005, Estimated: true },
};

function response(session: Session, identifier: string, usage: Usage | null): Message {
  return {
    ID: identifier,
    SessionID: session.ID,
    Seq: 1,
    Role: 'assistant',
    Content: [
      {
        Type: 'text',
        Text: 'Measured response',
        Data: null,
        MIME: '',
        URL: '',
        Filename: '',
        AudioID: '',
      },
    ],
    ToolCalls: null,
    ToolCallID: '',
    Usage: usage,
    CreatedAt: new Date().toISOString(),
  };
}

test('show one reported request instead of cumulative input, output, or cache totals', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  fixture.models[0].ContextMax = 100000;
  Object.assign(fixture.sessionUsage, {
    Calls: 70,
    Input: 1000000,
    CacheRead: 500000,
    Output: 450000,
    CacheHitPercentage: 99,
    Cost: { Currency: 'USD', Value: 1.25, Estimated: true },
    Costs: [{ Currency: 'USD', Value: 1.25, Estimated: true }],
  });
  await connectAndCreate(page);
  await expect(page.getByText('Events connected', { exact: true })).toBeAttached();
  const session = fixture.sessions[0];
  const message = response(session, 'report-one', report);
  fixture.histories.set(session.ID, [message]);
  fixture.event(session, 'model.call', { message_id: message.ID, model: session.Model });
  fixture.event(session, 'turn.end', {});
  const footer = page.getByLabel('Session usage', { exact: true });
  const context = footer.getByLabel('Last reported context', { exact: true });
  await expect(context).toContainText(/50[kK]\s*\/\s*100[kK]/);
  await expect(context).toContainText('(50.0%)');
  await expect(context).toHaveAttribute('title', /42,000 input \+ 8,000 output/);
  await expect(footer.getByLabel('Last response output')).toContainText('8,000');
  await expect(footer.getByLabel('Last request cache hit')).toContainText('23.8%');
  await expect(footer.getByLabel('Last request cache hit')).toHaveAttribute(
    'title',
    /Cache-write tokens: 2,000/,
  );
  await expect(footer.getByLabel('Conversation cost total')).toContainText('≈ $1.25');
  await expect(footer).not.toContainText('99.0%');

  // Child-agent accounting changes only the explicitly cumulative cost. The
  // selected session's provider report does not grow with other requests.
  Object.assign(fixture.sessionUsage, { Input: 2000000, Output: 800000 });
  fixture.event(session, 'agent.end', {});
  await expect(context).toContainText('(50.0%)');

  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 700 });
  for (const label of [
    'Last reported context',
    'Last response output',
    'Last request cache hit',
    'Conversation cost total',
  ]) {
    await expect(footer.getByLabel(label, { exact: true })).toBeVisible();
    await expect(footer.getByLabel(label, { exact: true })).toBeInViewport();
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath('reported-context-footer.png'),
    fullPage: true,
  });

  await openWorkbenchAction(page, 'Settings');
  await page.getByRole('dialog').getByRole('button', { name: 'Usage', exact: true }).click();
  const overview = page.getByRole('region', { name: 'Usage overview' });
  await expect(overview).toContainText('Total input tokens');
  await expect(overview).toContainText('2,500,000');
  await expect(overview).toContainText('Overall cache hit');
});

test('keep the measured model limit through a selection change and accept smaller new reports', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  fixture.models[0].ContextMax = 100000;
  fixture.models.push({
    ...fixture.models[0],
    ID: 'test/larger',
    Name: 'Larger model',
    ContextMax: 1000000,
  });
  await connectAndCreate(page);
  await expect(page.getByText('Events connected', { exact: true })).toBeAttached();
  const session = fixture.sessions[0];
  const first = response(session, 'old-model-report', report);
  fixture.histories.set(session.ID, [first]);
  fixture.event(session, 'model.call', { message_id: first.ID, model: session.Model });
  fixture.event(session, 'turn.end', {});
  const footer = page.getByLabel('Session usage', { exact: true });
  const context = footer.getByLabel('Last reported context', { exact: true });
  await expect(context).toContainText('(50.0%)');
  session.Model = 'test/larger';
  fixture.event(session, 'session.updated', {});
  await expect(page.locator('#conversation-header')).toContainText('test/larger');
  await expect(context).toContainText('(50.0%)');
  await expect(context).toHaveAttribute('title', /test\/test-model/);

  const second = response(session, 'smaller-new-report', {
    ...report,
    Input: 5000,
    CacheRead: 5000,
    CacheWrite: 0,
    Output: 1000,
    Reasoning: 500,
  });
  fixture.histories.set(session.ID, [first, second]);
  fixture.event(session, 'model.call', { message_id: second.ID, model: session.Model });
  fixture.event(session, 'turn.end', {});
  await expect(context).toContainText(/11[kK]\s*\/\s*1[mM]/);
  await expect(context).toContainText('(1.1%)');
  await expect(footer.getByLabel('Last request cache hit')).toContainText('50.0%');
  fixture.event(session, 'model.call', { message_id: first.ID, model: 'test/test-model' });
  await expect(context).toContainText('(1.1%)');
  // Many unrelated runtime events must not evict the matching request model.
  for (let index = 0; index < 90; index++) fixture.event(session, 'process.notification', {});
  await expect(context).toContainText('(1.1%)');

  const unavailable = response(session, 'unavailable-report', null);
  fixture.histories.set(session.ID, [first, second, unavailable]);
  fixture.event(session, 'model.call', { message_id: unavailable.ID, model: session.Model });
  fixture.event(session, 'turn.end', {});
  await expect(context).toContainText('—');
  await expect(context).not.toContainText('1.1%');
  await expect(footer.getByLabel('Last response output')).toContainText('—');
  await expect(footer.getByLabel('Last request cache hit')).toContainText('—');
});

test('clear request usage across sessions and do not invent a model limit or cache hit', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await expect(page.getByText('Events connected', { exact: true })).toBeAttached();
  const session = fixture.sessions[0];
  const first = response(session, 'no-model-association', report);
  fixture.histories.set(session.ID, [first]);
  fixture.event(session, 'turn.end', {});
  const footer = page.getByLabel('Session usage', { exact: true });
  const context = footer.getByLabel('Last reported context', { exact: true });
  await expect(context).toContainText(/50[kK]/);
  await expect(context).not.toContainText('%');
  await expect(context).toHaveAttribute('title', /Model limit unavailable/);
  await openWorkbenchAction(page, 'New session');
  await page.getByRole('button', { name: 'Create session', exact: true }).click();
  await expect(page.getByText('Events connected', { exact: true })).toBeAttached();
  await expect(context).toContainText('—');
  await expect(context).not.toContainText(/50[kK]/);

  const next = fixture.sessions[1];
  const zero = response(next, 'zero-usage', {
    ...report,
    Input: 0,
    CacheRead: 0,
    CacheWrite: 0,
    Output: 0,
    Reasoning: 0,
    Cost: null,
  });
  fixture.histories.set(next.ID, [zero]);
  fixture.event(next, 'model.call', { message_id: zero.ID, model: next.Model });
  fixture.event(next, 'turn.end', {});
  await expect(context).toContainText('0 /');
  await expect(context).toContainText('(0.0%)');
  await expect(footer.getByLabel('Last request cache hit')).toContainText('—');
});

test('ignore other sessions and keep missing provider fields unknown', () => {
  const session = { ID: 'main' } as Session;
  const first = response(session, 'own', report);
  const foreign = response({ ID: 'child' } as Session, 'foreign', { ...report, Input: 900000 });
  const result = latestRequestUsage([first, foreign], session.ID, new Map(), []);
  expect(result?.contextTokens).toBe(50000);
  expect(result?.contextLimit).toBeNull();
  const missing = response(session, 'missing-input', {
    ...report,
    Input: undefined,
  } as unknown as Usage);
  const unknown = latestRequestUsage([first, missing], session.ID, new Map(), []);
  expect(unknown?.inputTokens).toBeNull();
  expect(unknown?.contextTokens).toBeNull();
  expect(unknown?.cacheHitPercentage).toBeNull();
  expect(unknown?.outputTokens).toBe(8000);
});
