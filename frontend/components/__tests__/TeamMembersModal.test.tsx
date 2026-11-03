import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const getTeamMembers = vi.fn();
const addTeamMember = vi.fn();
const removeTeamMember = vi.fn();
const listAllUsers = vi.fn();

vi.mock('@/lib/api/admin', () => ({
  getTeamMembers: (...a: unknown[]) => getTeamMembers(...a),
  addTeamMember: (...a: unknown[]) => addTeamMember(...a),
  removeTeamMember: (...a: unknown[]) => removeTeamMember(...a),
  listAllUsers: (...a: unknown[]) => listAllUsers(...a),
}));

import TeamMembersModal from '../TeamMembersModal';

const user = (id: string, fullName = `User ${id}`) => ({ id, fullName, email: `${id}@x.com` });

beforeEach(() => {
  vi.clearAllMocks();
  getTeamMembers.mockResolvedValue({ members: [] });
  addTeamMember.mockResolvedValue(undefined);
});

const renderModal = () => render(<TeamMembersModal teamId="t1" teamName="Pod A" onClose={() => {}} />);

describe('TeamMembersModal — add-member picker', () => {
  it('loads the complete user set, not just the first page', async () => {
    listAllUsers.mockResolvedValue([user('u1')]);
    renderModal();

    await screen.findByTestId('member-search-input');

    expect(listAllUsers).toHaveBeenCalledTimes(1);
  });

  it('finds and adds a user that sits beyond the first 25', async () => {
    const many = Array.from({ length: 30 }, (_, i) => user(`u${i}`, `Person ${String(i).padStart(2, '0')}`));
    many.push(user('late', 'Zelda Late'));
    listAllUsers.mockResolvedValue(many);
    const u = userEvent.setup();
    renderModal();

    await u.type(await screen.findByTestId('member-search-input'), 'zelda');
    const rows = await screen.findAllByTestId('available-user-row');
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent('Zelda Late');

    await u.click(screen.getByTestId('add-member-btn'));

    await waitFor(() => expect(addTeamMember).toHaveBeenCalledWith('t1', 'late'));
  });

  it('excludes existing members from the picker', async () => {
    getTeamMembers.mockResolvedValue({ members: [{ userId: 'u1', userName: 'User u1', email: 'u1@x.com' }] });
    listAllUsers.mockResolvedValue([user('u1'), user('u2')]);
    renderModal();

    const rows = await screen.findAllByTestId('available-user-row');

    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent('User u2');
  });

  it('shows the empty state when every user is already a member', async () => {
    // Users exist, and every one of them is already on the team, so the
    // picker is empty only because members are filtered out.
    getTeamMembers.mockResolvedValue({
      members: [
        { userId: 'u1', userName: 'User u1', email: 'u1@x.com' },
        { userId: 'u2', userName: 'User u2', email: 'u2@x.com' },
      ],
    });
    listAllUsers.mockResolvedValue([user('u1'), user('u2')]);
    renderModal();

    expect(await screen.findByText('All users are already members.')).toBeInTheDocument();
    expect(screen.queryByTestId('available-user-row')).not.toBeInTheDocument();
    expect(screen.getAllByTestId('member-row')).toHaveLength(2);
  });

  it('shows the no-match message, not the all-members one, when a search excludes everyone', async () => {
    listAllUsers.mockResolvedValue([user('u1')]);
    const u = userEvent.setup();
    renderModal();

    await u.type(await screen.findByTestId('member-search-input'), 'zzz-no-such-user');

    expect(await screen.findByText('No matching users found.')).toBeInTheDocument();
    expect(screen.queryByText('All users are already members.')).not.toBeInTheDocument();
  });

  it('shows an error instead of a partial list when loading users fails', async () => {
    listAllUsers.mockRejectedValue(new Error('Network error'));
    renderModal();

    expect(await screen.findByText('Network error')).toBeInTheDocument();
    expect(screen.queryByTestId('available-user-row')).not.toBeInTheDocument();
  });

  it('aborts the in-flight load on unmount and ignores its late result', async () => {
    let signal: AbortSignal | undefined;
    let resolveUsers: (v: unknown) => void = () => {};
    listAllUsers.mockImplementation((p: { signal: AbortSignal }) => {
      signal = p.signal;
      return new Promise((resolve) => { resolveUsers = resolve; });
    });
    const { unmount } = renderModal();
    await waitFor(() => expect(signal).toBeDefined());

    unmount();

    expect(signal?.aborted).toBe(true);
    resolveUsers([user('u1')]); // must not throw or set state after unmount
  });

  it('passes an AbortSignal so a superseded load can be cancelled', async () => {
    listAllUsers.mockResolvedValue([]);
    renderModal();
    await screen.findByTestId('member-search-input');

    expect(listAllUsers.mock.calls[0][0].signal).toBeInstanceOf(AbortSignal);
  });
});

describe('TeamMembersModal — membership change notification', () => {
  it('notifies once per saved add, before the modal closes', async () => {
    listAllUsers.mockResolvedValue([user('u1')]);
    const onMembershipChange = vi.fn();
    const onClose = vi.fn();
    const u = userEvent.setup();
    render(<TeamMembersModal teamId="t1" teamName="Pod A" onClose={onClose} onMembershipChange={onMembershipChange} />);

    await u.click(await screen.findByTestId('add-member-btn'));

    await waitFor(() => expect(onMembershipChange).toHaveBeenCalledTimes(1));
    expect(onClose).not.toHaveBeenCalled();
  });

  it('notifies after a saved remove, and not at all when the save fails', async () => {
    getTeamMembers.mockResolvedValue({ members: [{ userId: 'u1', userName: 'User u1', email: 'u1@x.com' }] });
    listAllUsers.mockResolvedValue([]);
    removeTeamMember.mockRejectedValueOnce(new Error('nope')).mockResolvedValueOnce(undefined);
    const onMembershipChange = vi.fn();
    const u = userEvent.setup();
    render(<TeamMembersModal teamId="t1" teamName="Pod A" onClose={() => {}} onMembershipChange={onMembershipChange} />);

    await u.click(await screen.findByTestId('remove-member-btn'));
    await screen.findByText('nope');
    expect(onMembershipChange).not.toHaveBeenCalled();

    await u.click(screen.getByTestId('remove-member-btn'));
    await waitFor(() => expect(onMembershipChange).toHaveBeenCalledTimes(1));
  });
});
