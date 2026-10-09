import { test, expect } from '@playwright/test';
import { forgeName, routeForge, slugForge } from '../src/lib/forge';
import { checklist, notReviewing } from '../src/lib/setup';
import type { AdminAccount, Forge, SetupStatus } from '../src/lib/types';
import * as g from './golden';

test.describe('forgeName', () => {
  for (const [forge, want] of [
    ['github', 'GitHub'],
    ['forgejo', 'Forgejo'],
    ['gitea', 'Gitea'],
    ['gitlab', 'the forge'],
    ['', 'the forge'],
    [undefined, 'the forge'],
  ] as const) {
    test(`${forge} is ${want}`, () => {
      expect(forgeName(forge)).toBe(want);
    });
  }
});

test('the forge of an account is its slug\'s, and none outside one', () => {
  expect(slugForge('forgejo/acme')).toBe('forgejo');
  expect(slugForge('acme')).toBe('');
  expect(routeForge({ name: 'pulls', slug: 'gitea/acme' })).toBe('gitea');
  expect(routeForge({ name: 'overview' })).toBe('');
});

test.describe('checklist', () => {
  const live: AdminAccount = { ...g.adminAccount, live: true, repositories: 2 };
  const status = (forges: Forge[]): SetupStatus => ({ ...g.setupStatus, connections: forges.map((f) => `${f}-bot`), forges });

  test('a GitHub instance names its App', () => {
    const [connected, reach] = checklist(status(['github']), []);
    expect(connected).toMatchObject({ label: 'A GitHub App is connected', done: true });
    expect(reach).toMatchObject({ label: 'The App reaches a repository', done: false });
    expect(reach!.how).toContain('Install the App on GitHub');
  });

  test('a Forgejo instance is connected without a GitHub App', () => {
    const [connected, reach] = checklist(status(['forgejo']), [live]);
    expect(connected).toMatchObject({ label: 'A forge is connected', done: true });
    expect(reach).toMatchObject({ label: 'A connection reaches a repository', done: true });
    const [, unreached] = checklist(status(['forgejo']), []);
    expect(unreached!.how).toBe('Give the Forgejo bot access to a repository of an account its connection lists.');
    expect(unreached!.how).not.toContain('GitHub');
    expect(notReviewing(status(['forgejo']))).toBe('');
  });

  test('a mixed instance says what each forge needs', () => {
    const [, reach] = checklist(status(['forgejo', 'github', 'gitea']), []);
    expect(reach!.how).toContain('Install the App on GitHub');
    expect(reach!.how).toContain('Give the Forgejo or Gitea bot access');
  });
});
