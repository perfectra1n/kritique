// What the dashboard calls the forge an account is on. An account's slug is
// "<forge>/<name>", so an account route says its forge. Rune-free so tests
// can import it.
import type { Route } from './routes';
import type { Forge } from './types';

const NAMES: Record<Forge, string> = { github: 'GitHub', forgejo: 'Forgejo', gitea: 'Gitea' };

function known(forge: string): forge is Forge {
  return Object.hasOwn(NAMES, forge);
}

// forgeName is forge's name, 'the forge' for one the dashboard does not
// know or none.
export function forgeName(forge: string | undefined): string {
  return forge && known(forge) ? NAMES[forge] : 'the forge';
}

// slugForge is the forge of the account slug, '' for a malformed one.
export function slugForge(slug: string): string {
  const i = slug.indexOf('/');
  return i > 0 ? slug.slice(0, i) : '';
}

// routeForge is the forge of the account r is in, '' outside one.
export function routeForge(r: Route): string {
  return 'slug' in r ? slugForge(r.slug) : '';
}
