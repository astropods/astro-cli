// WorkOS roles that administer an account; astro-server 403s writes from
// anyone else, so this must match the server's own check.
export function isOrgAdmin(role: string | null | undefined): boolean {
  return role === "admin" || role === "owner";
}
