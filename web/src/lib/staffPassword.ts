/** Enforce twelve user-perceived code points and bcrypt's 72-byte limit. */
export function validStaffPassword(password: string): boolean {
  return Array.from(password).length >= 12 &&
    new TextEncoder().encode(password).length <= 72
}
