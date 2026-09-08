export const passwordPolicyMessage = 'Password must contain at least 8 characters and at most 72 UTF-8 bytes';
export function validPassword(password: string): boolean {
  return Array.from(password).length >= 8 && new TextEncoder().encode(password).length <= 72;
}
