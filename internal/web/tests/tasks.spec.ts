import { test, expect } from './fixtures';
import * as g from './golden';
import type * as T from '../src/lib/types';

const A = `#/a/${g.SLUG}`;
const api = `/api/v1/accounts/${g.SLUG}`;
const run = g.taskRun;
const detail = g.taskRunDetail;

const runs = (seen: URL[]) => seen.filter((u) => u.pathname.endsWith('/task-runs'));

test.describe('task runs', () => {
  test('the Tasks section lists each run with its status, subject, trigger and what it dropped', async ({ page }) => {
    await g.mockApi(page, g.defaultApi());
    await page.goto(`/${A}`);
    await page.locator('.topbar').getByRole('navigation', { name: 'Sections' }).getByRole('link', { name: 'Tasks' }).click();
    await expect(page).toHaveURL(new RegExp(`${A}/tasks$`));
    await expect(page.locator('.sections .section-tab.active')).toHaveText('Tasks');
    const row = page.locator('table.task-runs tbody tr');
    await expect(row).toHaveCount(1);
    await expect(row.locator('.pill')).toHaveText(run.status);
    await expect(row).toContainText(run.task);
    await expect(row).toContainText(`${run.repository}#${run.subjectNumber}`);
    await expect(row).toContainText(run.trigger);
    await expect(row).toContainText(run.mode);
    await expect(row).toContainText(`${run.droppedCount} dropped`);
    await expect(row).toContainText('1m 30s');
  });

  test('a pull request run links to its pull request; an issue run names its number', async ({ page }) => {
    const pr: T.TaskRun = { ...run, id: 'tr-2', subjectKind: 'pull', subjectNumber: 7 };
    await g.mockApi(page, [[new RegExp(`${api}/task-runs$`), g.pageOf([run, pr])], ...g.defaultApi()]);
    await page.goto(`/${A}/tasks`);
    const rows = page.locator('table.task-runs tbody tr');
    await expect(rows.nth(0).getByRole('link', { name: `${run.repository}#${run.subjectNumber}` })).toHaveCount(0);
    await expect(rows.nth(1).getByRole('link', { name: `${run.repository}#7` })).toHaveAttribute('href', `${A}/pulls/alpha/one/7`);
  });

  test('filters by repository, task and status, in the URL', async ({ page }) => {
    const seen = await g.mockApi(page, g.defaultApi());
    await page.goto(`/${A}/tasks`);
    await expect(page.locator('table.task-runs tbody tr')).toHaveCount(1);
    await page.getByRole('combobox', { name: 'Search task runs' }).fill('repo:alpha/one triage');
    await expect(page).toHaveURL(new RegExp(`${A}/tasks\\?repo=alpha%2Fone&task=triage$`));
    await page.getByRole('radio', { name: 'Failed' }).click();
    await expect(page).toHaveURL(/status=failed/);
    await expect
      .poll(() =>
        runs(seen).some((u) => u.searchParams.get('repo') === 'alpha/one' && u.searchParams.get('task') === 'triage' && u.searchParams.get('status') === 'failed'),
      )
      .toBe(true);
    await page.reload();
    await expect(page.getByRole('combobox', { name: 'Search task runs' })).toHaveValue('repo:alpha/one task:triage status:failed');
  });

  test('an empty list says whether its filters emptied it, and clears them', async ({ page }) => {
    await g.mockApi(page, [[new RegExp(`${api}/task-runs$`), g.pageOf([])], ...g.defaultApi()]);
    await page.goto(`/${A}/tasks?status=failed`);
    await expect(page.locator('.state-msg')).toContainText('No task runs match these filters.');
    await page.getByRole('button', { name: 'Clear filters' }).click();
    await expect(page).toHaveURL(new RegExp(`${A}/tasks$`));
    await expect(page.locator('.state-msg')).toContainText('No task runs yet');
  });

  test('a task run event refetches the list', async ({ page }) => {
    const seen = await g.mockApi(page, g.defaultApi());
    const ev: T.LiveEvent = { ...g.liveEvent, kind: 'task_run', account: g.SLUG, id: run.id, reviewId: null };
    await page.route('**/api/events', (route) =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: `event: task_run\ndata: ${JSON.stringify(ev)}\n\n` }),
    );
    await page.goto(`/${A}/tasks`);
    await expect.poll(() => runs(seen).length).toBeGreaterThan(1);
  });
});

test.describe('task run', () => {
  test('shows what it applied and dropped, its fields, notes, event, answer and transcript', async ({ page }) => {
    await g.mockApi(page, g.defaultApi());
    await page.goto(`/${A}/tasks`);
    await page.getByRole('link', { name: run.task, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`${A}/tasks/${run.id}$`));
    await expect(page.locator('.page-head h1')).toContainText(run.task);
    await expect(page.locator('.page-head h1')).toContainText(`#${run.subjectNumber}`);
    await expect(page.locator('.meta-line')).toContainText(`${detail.modelCalls} model call`);

    const applied = page.locator('tr.action-applied');
    const a = detail.applied!;
    await expect(applied).toHaveCount(a.addLabels.length + a.removeLabels.length + a.inline.length + 1);
    await expect(applied.filter({ hasText: 'labels.add' })).toContainText(a.addLabels[0]!);
    await expect(applied.filter({ hasText: 'labels.remove' })).toContainText(a.removeLabels[0]!);
    const c = a.inline[0]!;
    await expect(applied.filter({ hasText: 'inline' })).toContainText(`${c.path}:${c.line}-${c.endLine}`);
    await expect(applied.filter({ hasText: 'comment' })).toContainText(a.comment);
    const dropped = page.locator('tr.action-dropped');
    const d = detail.dropped[0]!;
    await expect(dropped).toHaveCount(detail.dropped.length);
    await expect(dropped.locator('.pill')).toHaveText('dropped');
    await expect(dropped).toContainText(d.value);
    await expect(dropped).toContainText(d.reason);

    const fields = page.getByRole('region', { name: 'Fields', exact: true });
    await expect(fields.locator('dt')).toHaveText(Object.keys(detail.fields));
    await expect(fields.locator('dd').first()).toHaveText(String(detail.fields.priority));
    await expect(page.getByRole('region', { name: 'Left out of the context', exact: true })).toContainText(detail.notes[0]!);

    const event = page.getByRole('region', { name: 'Event', exact: true });
    await expect(event).toContainText(`${detail.event!.rawEvent}.${detail.event!.action}`);
    await expect(event).toContainText(detail.event!.sender);
    await expect(page.getByRole('region', { name: 'Model answer', exact: true })).toContainText(detail.proposed!.summary);

    await page.getByRole('button', { name: 'Transcript' }).click();
    await expect(page.locator('.turn')).toHaveCount(g.transcript.turns.length);

    await page.getByRole('link', { name: run.repository, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`${A}/tasks\\?repo=alpha%2Fone$`));
  });

  test('a queued run whose event was swept says so', async ({ page }) => {
    const queued: T.TaskRunDetail = {
      ...detail,
      run: { ...run, status: 'queued', startedAt: null, finishedAt: null, durationMs: null, droppedCount: 0 },
      event: null,
      fields: {},
      proposed: null,
      applied: null,
      dropped: [],
      notes: [],
      modelCalls: 0,
    };
    await g.mockApi(page, [[new RegExp(`${api}/task-runs/${run.id}$`), queued], ...g.defaultApi()]);
    await page.goto(`/${A}/tasks/${run.id}`);
    await expect(page.getByRole('region', { name: 'Actions', exact: true })).toContainText('No actions recorded yet.');
    await expect(page.getByRole('region', { name: 'Event', exact: true })).toContainText('past its retention');
    await expect(page.getByRole('region', { name: 'Fields', exact: true })).toContainText('No fields.');
    await expect(page.getByRole('region', { name: 'Model answer', exact: true })).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Left out of the context', exact: true })).toHaveCount(0);
  });

  test('a run that is not the account\'s is not found', async ({ page }) => {
    await g.mockApi(page, g.defaultApi());
    await page.goto(`/${A}/tasks/nope`);
    await expect(page.locator('.state-error')).toContainText('not found');
  });
});

test.describe('repository tasks', () => {
  test('list each definition, where it is defined, and what the bounds left out', async ({ page }) => {
    await g.mockApi(page, g.defaultApi());
    await page.goto(`/${A}/repos/alpha/one`);
    const tasks = page.getByRole('region', { name: 'Tasks', exact: true });
    const [welcome, triage] = g.repoTasks.tasks;
    const rows = tasks.locator('tbody tr');
    await expect(rows).toHaveCount(g.repoTasks.tasks.length);
    await expect(rows.nth(0)).toContainText(welcome!.name);
    await expect(rows.nth(0)).toContainText('config file');
    await expect(rows.nth(0)).toContainText('always');
    await expect(rows.nth(1)).toContainText('.kritique.yaml');
    await expect(rows.nth(1)).toContainText(triage!.triggers.join(', '));
    await expect(rows.nth(1)).toContainText(triage!.when[0]!.expr);
    await expect(rows.nth(1)).toContainText(triage!.actions.join(', '));
    await expect(rows.nth(1).getByRole('link', { name: triage!.name })).toHaveAttribute('href', `${A}/tasks?repo=alpha%2Fone&task=${triage!.name}`);
    const note = g.repoTasks.notes[0]!;
    await expect(tasks.getByTestId('tasks-notes')).toContainText(`${note.task}: ${note.what}: ${note.reason}`);
    await expect(tasks.getByTestId('tasks-source')).toContainText('default branch tip');
    await expect(tasks.getByTestId('tasks-source')).toContainText(g.repoTasks.commit.slice(0, 7));
    await expect(tasks.getByTestId('tasks-ignored')).toContainText(`Ignored as a whole: ${g.repoTasks.ignored}`);
    await tasks.getByRole('link', { name: 'Task runs' }).click();
    await expect(page).toHaveURL(new RegExp(`${A}/tasks\\?repo=alpha%2Fone$`));
  });

  test('say when they come from the last review, before any task event', async ({ page }) => {
    await g.mockApi(page, [
      [new RegExp(`${api}/repos/alpha/one/tasks$`), { ...g.repoTasks, source: 'lastReview', commit: 'def4567', resolvedAt: null }],
      ...g.defaultApi(),
    ]);
    await page.goto(`/${A}/repos/alpha/one`);
    const source = page.getByRole('region', { name: 'Tasks', exact: true }).getByTestId('tasks-source');
    await expect(source).toContainText('the last review read at');
    await expect(source).toContainText('def4567');
  });

  test('a repository with no tasks says so', async ({ page }) => {
    await g.mockApi(page, [
      [new RegExp(`${api}/repos/alpha/one/tasks$`), { ...g.repoTasks, tasks: [], notes: [], ignored: undefined } satisfies T.RepoTasks],
      ...g.defaultApi(),
    ]);
    await page.goto(`/${A}/repos/alpha/one`);
    const tasks = page.getByRole('region', { name: 'Tasks', exact: true });
    await expect(tasks.locator('.state-msg')).toHaveText('No tasks are defined for this repository.');
    await expect(tasks.getByTestId('tasks-ignored')).toHaveCount(0);
    await expect(tasks.getByTestId('tasks-notes')).toHaveCount(0);
  });

  test('tasks that are off say how to turn them on, and nothing of what they would run', async ({ page }) => {
    const off: T.RepoTasks = {
      ...g.repoTasks,
      enabled: false,
      tasks: [],
      notes: g.repoTasks.tasks.map((t) => ({ task: t.name, what: 'task', reason: 'tasks are disabled' })),
    };
    await g.mockApi(page, [[new RegExp(`${api}/repos/alpha/one/tasks$`), off], ...g.defaultApi()]);
    await page.goto(`/${A}/repos/alpha/one`);
    const tasks = page.getByRole('region', { name: 'Tasks', exact: true });
    await expect(tasks.getByTestId('tasks-off')).toContainText('taskBounds.enabled');
    await expect(tasks.locator('table')).toHaveCount(0);
    await expect(tasks.getByTestId('tasks-notes')).toHaveCount(0);
  });
});
