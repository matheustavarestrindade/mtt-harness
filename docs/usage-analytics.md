# Usage Analytics

## Data

`GET /usage` gives usage buckets by UTC day and model. It reads `usage_records`.
It does not read conversation content or provider secrets.

The date range includes `from` and `to`. The default date range is 30 UTC days.
The last UTC day is the UTC day of the request.

```text
from: YYYY-MM-DD
to: YYYY-MM-DD
instance_id: optional workspace ID
session_id: optional conversation root ID
agent_prefix: optional workspace worker prefix, for example context.

Maximum date range: 366 days
Maximum database groups: 10000 day/model/currency groups
```

Use one of `instance_id` and `session_id`. `instance_id` is necessary for `agent_prefix`.
Session usage includes child agents. Workspace and harness usage also include
workspace agents. `SourceSessionID` does not set the usage scope.

## Response

```typescript
type UsageReport = {
  From: string; To: string; Timezone: 'UTC';
  Statistics: Statistics;
  PricedCalls: number; FailedCalls: number;
  Buckets: {
    Date: string; ModelID: string; Statistics: Statistics;
    PricedCalls: number; FailedCalls: number;
  }[];
};
```

The database gives usage buckets in one read snapshot. `Statistics` uses the usage buckets. An empty usage report has an empty `Buckets` array.

## Usage Values

```text
Input: uncached input tokens
Output: reported output tokens, including reasoning once
Token total: Input + Output
CacheRead and CacheWrite: separate cache usage
```

The `PricedCalls` value gives the number of requests with a recorded cost. A cost of 0 is known. If cost data is not available, the cost stays unknown. Currencies stay different. Cost estimates keep `Estimated: true`.

The cost comes from `usage_records`. The usage report does not calculate history
with new model prices. The usage report does not contain API key IDs.

## UI

`Settings → Usage` uses `shadcn-svelte` charts with LayerChart. The user can select a date
range, provider, model, and usage scope. Tokens, cost, requests, and cache have
different charts. The date range uses `RangeCalendar`.

The chart has a maximum of 7 model series and one `Other models` model series. The `Other models` model series has the usage for models not in the other model series. A data table gives correct values. The charts use usage buckets.

Worker usage uses the same chart components in Memory, Sidekick, `Instructions`,
and Watchers. The `PluginMetrics` response gives the charts for work status and `low`, `medium`, and `high` reminders. The charts from `PluginMetrics` do not give usage by UTC day.

The UI uses `import()` for chart code when a chart opens. The UI removes charts
and uses `AbortController` to stop requests when a view closes. Previous responses
must not replace data for a new date range, workspace, or session.
