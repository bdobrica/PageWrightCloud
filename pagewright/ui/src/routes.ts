export const CHAT_ROUTE = '/chat/:fqdn';

export function chatPath(fqdn: string): string {
  return `/chat/${encodeURIComponent(fqdn)}`;
}
