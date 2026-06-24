import { base } from '$app/paths';

export interface AppConfig {
  wsUrl: string;
  searchUrl: string;
  openResultsOnNewTab: boolean;
  hotkeys: Record<string, string>;
  authMode: 'token' | 'user' | 'none';
  authenticated: boolean;
  public: boolean;
  canWrite: boolean;
  historyEnabled: boolean;
  username?: string;
  userId?: number;
  oauthOnly?: boolean;
  disablePreviews?: boolean;
}

export interface ExtractorInfo {
  name: string;
  description: string;
  enabled: boolean;
  options?: Record<string, unknown>;
}

let _config: AppConfig | null = null;
let _csrf: string = '';

function getCsrf(): string {
  return _csrf;
}

function setCsrf(tok: string): void {
  _csrf = tok;
}

function getAuthMode(): string {
  return _config?.authMode ?? 'none';
}

function getUsername(): string {
  return _config?.username ?? '';
}

export function getUserId(): number | undefined {
  return _config?.userId;
}

export function resetConfig(): void {
  _config = null;
}

export async function fetchConfig(): Promise<AppConfig> {
  if (_config) return _config;
  const headers: Record<string, string> = {};
  const token = localStorage.getItem('access-token');
  if (token) {
    headers['X-Access-Token'] = token;
  }
  const res = await fetch('api/config', { headers, credentials: 'include' });
  if (res.status === 403) {
    window.location.href = base + '/auth';
    throw new Error('Authentication required');
  }
  const tok = res.headers.get('X-CSRF-Token');
  if (tok) _csrf = tok;
  _config = await res.json();
  return _config!;
}

export async function login(username: string, password: string): Promise<{ username: string }> {
  const res = await fetch('api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    throw new Error('Invalid credentials');
  }
  _config = null;
  return res.json();
}

export async function logout(): Promise<void> {
  try {
    await apiFetch('/logout', { method: 'POST', redirectOnForbidden: false });
  } finally {
    localStorage.removeItem('access-token');
    _config = null;
  }
}

interface ApiFetchOptions extends RequestInit {
  redirectOnForbidden?: boolean;
}

export async function apiFetch(url: string, options: ApiFetchOptions = {}): Promise<Response> {
  const { redirectOnForbidden = true, ...fetchOptions } = options;
  const headers: Record<string, string> = {
    ...(fetchOptions.headers as Record<string, string>),
  };
  if (_csrf && fetchOptions.method && fetchOptions.method.toUpperCase() !== 'GET') {
    headers['X-CSRF-Token'] = _csrf;
  }
  const token = localStorage.getItem('access-token');
  if (token) {
    headers['X-Access-Token'] = token;
  }
  const res = await fetch('api' + url, { ...fetchOptions, headers, credentials: 'include' });
  if (res.status === 403 && redirectOnForbidden) {
    window.location.href = base + '/auth';
    throw new Error('Authentication required');
  }
  const newTok = res.headers.get('X-CSRF-Token');
  if (newTok) _csrf = newTok;
  return res;
}

export async function fetchExtractors(): Promise<ExtractorInfo[]> {
  const res = await apiFetch('/extractors');
  if (!res.ok) {
    throw new Error('Failed to fetch extractors');
  }
  return res.json();
}
