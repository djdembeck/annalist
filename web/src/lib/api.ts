// /api contract client for the annalist backend.
// Matches the Go httpapi endpoints exactly. Admin routes require
// `Authorization: Bearer <ADMIN_TOKEN>`; the token is kept in localStorage.

const TOKEN_KEY = "annalist-admin-token";

export type Status = {
  github: boolean;
  forgejo: boolean;
  admin: boolean;
};

export type Repo = {
  platform: string;
  owner: string;
  repo: string;
  enabled: boolean;
  tone: string | null;
  instructions: string | null;
  model: string | null;
  temperature: number | null;
  trigger: string;
  effective: {
    tone: string | null;
    model: string | null;
    temperature: number | null;
  };
};

export type AvailableRepo = {
  platform: string;
  owner: string;
  repo: string;
  fork?: boolean;
  ownNamespace?: boolean;
};

export type RepoSettingUpdate = {
  enabled?: boolean;
  tone?: string | null;
  instructions?: string | null;
  model?: string | null;
  temperature?: number | null;
  trigger?: string;
};

export type GenerateResult = {
  notes: string;
  release_id: string | null;
  published: boolean;
};

export type GenerateRequest = {
  to_tag: string;
  from_tag?: string | null;
  force?: boolean;
};

export type Settings = {
  tone: string | null;
  instructions: string | null;
  model: string | null;
  temperature: number | null;
  llm: {
    base_url: string;
    model: string;
  };
  github: boolean;
  forgejo: boolean;
};

export type SettingsUpdate = {
  tone?: string | null;
  instructions?: string | null;
  model?: string | null;
  temperature?: number | null;
};

export function getToken(): string {
  return typeof localStorage !== "undefined"
    ? localStorage.getItem(TOKEN_KEY) ?? ""
    : "";
}

export function setToken(t: string): void {
  if (t) {
    localStorage.setItem(TOKEN_KEY, t);
  } else {
    localStorage.removeItem(TOKEN_KEY);
  }
}

async function apiFetch<T>(path: string, opts?: RequestInit): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...((opts?.headers as Record<string, string>) ?? {}),
  };

  const res = await fetch(path, { ...opts, headers });

  if (res.status === 401) {
    localStorage.removeItem(TOKEN_KEY);
    throw new Error("Unauthorized");
  }
  if (!res.ok) {
    throw new Error(`Request failed: ${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export function getStatus(): Promise<Status> {
  return apiFetch<Status>("/api/status");
}

export function getRepos(): Promise<Repo[]> {
  return apiFetch<Repo[]>("/api/repos");
}

export function getAvailableRepos(): Promise<AvailableRepo[]> {
  return apiFetch<AvailableRepo[]>("/api/repos/available");
}

export function addRepo(repo: {
  platform: string;
  owner: string;
  repo: string;
}): Promise<Repo> {
  return apiFetch<Repo>("/api/repos", {
    method: "POST",
    body: JSON.stringify(repo),
  });
}

export function putRepoSettings(
  platform: string,
  owner: string,
  repo: string,
  data: RepoSettingUpdate,
): Promise<void> {
  return apiFetch<void>(
    `/api/repos/${platform}/${owner}/${repo}/settings`,
    {
      method: "PUT",
      body: JSON.stringify(data),
    },
  );
}

export function generate(
  platform: string,
  owner: string,
  repo: string,
  data: GenerateRequest,
): Promise<GenerateResult> {
  return apiFetch<GenerateResult>(
    `/api/repos/${platform}/${owner}/${repo}/generate`,
    {
      method: "POST",
      body: JSON.stringify(data),
    },
  );
}

export function getSettings(): Promise<Settings> {
  return apiFetch<Settings>("/api/settings");
}

export function putSettings(data: SettingsUpdate): Promise<Settings> {
  return apiFetch<Settings>("/api/settings", {
    method: "PUT",
    body: JSON.stringify(data),
  });
}
