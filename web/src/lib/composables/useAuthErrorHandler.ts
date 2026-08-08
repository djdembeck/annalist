import { goto } from "$app/navigation";

export function handleAuthError(e: unknown, onUnauthorized?: () => void): boolean {
  if (e instanceof Error && e.message === "Unauthorized") {
    if (onUnauthorized) {
      onUnauthorized();
    } else {
      goto("/setup");
    }
    return true;
  }
  return false;
}

export function formatError(e: unknown, fallback: string): string {
  if (e instanceof Error && e.message !== "Unauthorized") {
    return e.message;
  }
  return fallback;
}
