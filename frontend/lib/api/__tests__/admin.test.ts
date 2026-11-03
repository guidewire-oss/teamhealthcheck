import { describe, it, expect, vi, beforeEach } from 'vitest';
import { listAllUsers, listUsers, listUsersLite } from '../admin';

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

describe('listAllUsers', () => {
  const user = (id: string) => ({ id, fullName: `User ${id}`, email: `${id}@x.com` });
  const page = (users: ReturnType<typeof user>[], pageNo: number, totalPages: number) => ({
    users,
    pagination: { page: pageNo, pageSize: 100, totalItems: 0, totalPages, hasNextPage: pageNo < totalPages, hasPreviousPage: pageNo > 1 },
  });

  it('walks every page at the backend max page size and returns the whole set', async () => {
    (createApiClient as any)
      .mockResolvedValueOnce(page([user('a'), user('b')], 1, 3))
      .mockResolvedValueOnce(page([user('c')], 2, 3))
      .mockResolvedValueOnce(page([user('d')], 3, 3));

    const all = await listAllUsers();

    expect(all.map((u) => u.id)).toEqual(['a', 'b', 'c', 'd']);
    expect(createApiClient).toHaveBeenCalledTimes(3);
    const pages = (createApiClient as any).mock.calls.map(([url]: [string]) => new URL(url, 'http://x').searchParams);
    expect(pages.map((p: URLSearchParams) => p.get('page'))).toEqual(['1', '2', '3']);
    expect(pages.every((p: URLSearchParams) => p.get('pageSize') === '100')).toBe(true);
  });

  it('makes a user beyond the first 25 reachable', async () => {
    const firstPage = Array.from({ length: 25 }, (_, i) => user(`u${i}`));
    (createApiClient as any)
      .mockResolvedValueOnce(page(firstPage, 1, 2))
      .mockResolvedValueOnce(page([user('u-26th')], 2, 2));

    const all = await listAllUsers();

    expect(all.find((u) => u.id === 'u-26th')).toBeDefined();
  });

  it('deduplicates users that appear on more than one page', async () => {
    (createApiClient as any)
      .mockResolvedValueOnce(page([user('a'), user('b')], 1, 2))
      .mockResolvedValueOnce(page([user('b'), user('c')], 2, 2));

    expect((await listAllUsers()).map((u) => u.id)).toEqual(['a', 'b', 'c']);
  });

  it('passes search and role filters to every page request', async () => {
    (createApiClient as any)
      .mockResolvedValueOnce(page([user('a')], 1, 2))
      .mockResolvedValueOnce(page([user('b')], 2, 2));

    await listAllUsers({ search: 'ali', role: 'level-4' });

    for (const [url] of (createApiClient as any).mock.calls) {
      const q = new URL(url, 'http://x').searchParams;
      expect(q.get('search')).toBe('ali');
      expect(q.get('role')).toBe('level-4');
    }
  });

  it('stops fetching further pages once aborted', async () => {
    const controller = new AbortController();
    (createApiClient as any).mockImplementationOnce(async () => {
      controller.abort();
      return page([user('a')], 1, 5);
    });

    await expect(listAllUsers({ signal: controller.signal })).rejects.toMatchObject({ name: 'AbortError' });
    expect(createApiClient).toHaveBeenCalledTimes(1);
  });

  it('handles an empty result without further requests', async () => {
    (createApiClient as any).mockResolvedValueOnce(page([], 1, 0));

    expect(await listAllUsers()).toEqual([]);
    expect(createApiClient).toHaveBeenCalledTimes(1);
  });

  it('propagates a page failure rather than returning a silently partial set', async () => {
    (createApiClient as any)
      .mockResolvedValueOnce(page([user('a')], 1, 2))
      .mockRejectedValueOnce(new Error('boom'));

    await expect(listAllUsers()).rejects.toThrow('boom');
  });

  it('leaves listUsers as a single bounded request', async () => {
    (createApiClient as any).mockResolvedValue(page([user('a')], 1, 9));

    await listUsers();

    expect(createApiClient).toHaveBeenCalledTimes(1);
  });
});
