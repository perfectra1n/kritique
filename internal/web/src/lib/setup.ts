// What an instance still lacks before it reviews, from its setup status and
// the accounts its connections serve. Everything it names is set in the
// configuration file. Rune-free so tests can import it.
import type { AdminAccount, Connection, SetupStatus } from './types';
import { forgeName } from './forge';

export interface ChecklistItem {
  label: string;
  done: boolean;
  // optional is a step kritique reviews without.
  optional?: boolean;
  // how says what to do while the step is not done.
  how: string;
}

export function checklist(s: SetupStatus, accounts: AdminAccount[]): ChecklistItem[] {
  // A GitHub connection is a GitHub App, installed on the accounts it
  // serves; a Forgejo or Gitea one is a bot user, given access to them.
  const github = s.forges.includes('github');
  const others = s.forges.filter((f) => f !== 'github').map(forgeName);
  const onlyGitHub = github && others.length === 0;
  const reach: string[] = [];
  if (github || s.forges.length === 0) reach.push('Install the App on GitHub on an account its entry under apps lists.');
  if (others.length) reach.push(`Give the ${others.join(' or ')} bot access to a repository of an account its connection lists.`);
  return [
    {
      label: onlyGitHub ? 'A GitHub App is connected' : 'A forge is connected',
      done: s.connections.length > 0,
      how: 'Declare a GitHub App, or a Forgejo or Gitea connection, in the configuration file, its secrets referenced from a Secret.',
    },
    {
      label: onlyGitHub ? 'The App reaches a repository' : 'A connection reaches a repository',
      done: accounts.some((a) => a.live && a.repositories > 0),
      how: reach.join(' '),
    },
    {
      label: 'A review model is set',
      done: s.reviewModel !== '',
      how: 'Set review.model to a model one of the providers serves.',
    },
    {
      label: 'An embedder is set',
      done: s.embedding,
      optional: true,
      how: 'Set embedding to index each repository for similar code; reviews run without it.',
    },
  ];
}

// notReviewing says why the instance cannot review yet, '' when it can.
export function notReviewing(s: SetupStatus): string {
  if (s.connections.length === 0) return 'no forge is connected';
  if (!s.reviewModel) return 'no review model is set';
  return '';
}

// unsignedWebhooks is whether c's webhooks arrive with no signature, which
// kritique refuses: its last unsigned delivery is newer than its last
// verified one, so the webhook still has no secret.
export function unsignedWebhooks(c: Pick<Connection, 'lastWebhookAt' | 'lastUnsignedWebhookAt'>): boolean {
  if (!c.lastUnsignedWebhookAt) return false;
  return !c.lastWebhookAt || Date.parse(c.lastUnsignedWebhookAt) > Date.parse(c.lastWebhookAt);
}
