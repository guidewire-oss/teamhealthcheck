import { describe, it, expect, vi, beforeEach } from 'vitest';
import { listUsers, listUsersLite } from '../admin';

// Mock the API client module so API_BASE_URL is always '' and auth is bypassed.
vi.mock('../client', () => ({
  API_BASE_URL: '',
  createApiClient: vi.fn(),
}));

import { createApiClient } from '../client';

beforeEach(() => {
  vi.clearAllMocks();
});

describe('listUsers', () => {
  it('requests the default page with no query params when called with no args', async () => {
    (createApiClient as any).mockResolvedValue({ users: [], pagination: {} });

    await listUsers();

    expect(createApiClient).toHaveBeenCalledWith('/api/v1/admin/users', undefined);
  });

  it('builds a query string from page, pageSize, search, and role', async () => {
    (createApiClient as any).mockResolvedValue({ users: [], pagination: {} });

    await listUsers({ page: 2, pageSize: 25, search: 'alice', role: 'level-3' });

    const [url] = (createApiClient as any).mock.calls[0];
    const parsed = new URL(url, 'http://localhost');
    expect(parsed.pathname).toBe('/api/v1/admin/users');
    expect(parsed.searchParams.get('page')).toBe('2');
    expect(parsed.searchParams.get('pageSize')).toBe('25');
    expect(parsed.searchParams.get('search')).toBe('alice');
    expect(parsed.searchParams.get('role')).toBe('level-3');
  });

  it('omits search/role params when empty', async () => {
    (createApiClient as any).mockResolvedValue({ users: [], pagination: {} });

    await listUsers({ page: 1, pageSize: 25, search: '', role: '' });

    const [url] = (createApiClient as any).mock.calls[0];
    expect(url).toBe('/api/v1/admin/users?page=1&pageSize=25');
  });

  it('passes the AbortSignal through to the underlying request', async () => {
    (createApiClient as any).mockResolvedValue({ users: [], pagination: {} });
    const controller = new AbortController();

    await listUsers({ page: 1, signal: controller.signal });

    const [, options] = (createApiClient as any).mock.calls[0];
    expect(options).toEqual({ signal: controller.signal });
  });
});

describe('listUsersLite', () => {
  it('requests the lite endpoint with no params', async () => {
    (createApiClient as any).mockResolvedValue({ users: [] });

    await listUsersLite();

    expect(createApiClient).toHaveBeenCalledWith('/api/v1/admin/users/lite');
  });
});
