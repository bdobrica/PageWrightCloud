import type { SubmissionSnapshot } from './submission';

export interface Draft {
  text: string;
  conversationId?: string;
  question?: string;
  original?: string;
  identity?: SubmissionSnapshot;
}
export type DraftStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
const prefix = 'pagewright.draft.v1:';
export const draftKey = (owner: string, fqdn: string) => prefix + JSON.stringify([owner, fqdn]);

export function readDraft(storage: DraftStorage, owner: string, fqdn: string): Draft {
  const raw = storage.getItem(draftKey(owner, fqdn));
  if (!raw) return { text: '' };
  const value = JSON.parse(raw);
  if (!value || typeof value.text !== 'string' || value.text.length > 50000 ||
      ['conversationId', 'question', 'original'].some(key =>
        value[key] !== undefined && (typeof value[key] !== 'string' || value[key].length > 50000))) {
    throw new Error('Saved draft is invalid; it has not been overwritten.');
  }
  if (value.identity && (typeof value.identity.fingerprint !== 'string' ||
      typeof value.identity.key !== 'string' ||
      !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value.identity.key) ||
      value.identity.fingerprint !== JSON.stringify([fqdn, value.text, value.conversationId ?? '']))) {
    throw new Error('Saved retry identity is invalid; it has not been overwritten.');
  }
  return { text: value.text, conversationId: value.conversationId, question: value.question,
    original: value.original, identity: value.identity };
}

export function writeDraft(storage: DraftStorage, owner: string, fqdn: string, draft: Draft) {
  storage.setItem(draftKey(owner, fqdn), JSON.stringify(draft));
}

export function clearDrafts(storage: Storage) {
  for (let i = storage.length - 1; i >= 0; i--) {
    const key = storage.key(i);
    if (key?.startsWith(prefix)) storage.removeItem(key);
  }
  storage.removeItem('pagewright.resume');
}

export function safeReturnPath(path: unknown): string {
  return typeof path === 'string' && (/^\/chat\/(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(path) ||
    ['/dashboard', '/create-site', '/profile'].includes(path)) ? path : '/dashboard';
}

export function readOwner(storage: Pick<Storage, 'getItem'>): string | null {
  try {
    const user = JSON.parse(storage.getItem('user') || 'null');
    return typeof user?.id === 'string' ? user.id : null;
  } catch { return null; }
}

export function rememberReturn(storage: DraftStorage, owner: string, path: string) {
  storage.setItem('pagewright.resume', JSON.stringify({ owner, path: safeReturnPath(path) }));
}

export function consumeReturn(storage: DraftStorage, owner: string): string {
  try {
    const saved = JSON.parse(storage.getItem('pagewright.resume') || 'null');
    storage.removeItem('pagewright.resume');
    return saved?.owner === owner ? safeReturnPath(saved.path) : '/dashboard';
  } catch { return '/dashboard'; }
}

// Ignore login failures and old requests from a replaced session.
export function isSessionExpiry(status: number | undefined, path: string | undefined,
  sentToken: unknown, currentToken: string | null): boolean {
  return status === 401 && !!currentToken && sentToken === 'Bearer ' + currentToken &&
    !!path && (path.startsWith('/sites') || path === '/auth/update-password');
}
