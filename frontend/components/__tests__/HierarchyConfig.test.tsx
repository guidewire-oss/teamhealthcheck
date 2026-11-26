import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import HierarchyConfig from '../HierarchyConfig';
import * as adminApi from '@/lib/api/admin';

vi.mock('@/lib/api/admin', () => ({
  listHierarchyLevels: vi.fn(),
  createHierarchyLevel: vi.fn(),
  updateHierarchyLevel: vi.fn(),
  deleteHierarchyLevel: vi.fn(),
  clearAdminCacheKeys: vi.fn(),
}));

const basePermissions = {
  canViewAllTeams: false,
  canEditTeams: false,
  canManageUsers: false,
  canTakeSurvey: true,
  canViewAnalytics: false,
};

const levels = [
  {
    id: 'level-1',
    name: 'VP',
    position: 1,
    permissions: basePermissions,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
  },
  {
    id: 'level-2',
    name: 'Director',
    position: 2,
    permissions: basePermissions,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
  },
];

beforeEach(() => {
  vi.resetAllMocks();
  (adminApi.listHierarchyLevels as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(levels);
  (adminApi.createHierarchyLevel as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(levels[0]);
});

describe('HierarchyConfig', () => {
  it('does not present a color-setting affordance anywhere in the create or edit forms', async () => {
    const user = userEvent.setup();
    render(<HierarchyConfig />);
    await screen.findAllByTestId('hierarchy-level-row');

    await user.click(screen.getByTestId('add-level-btn'));
    expect(screen.queryByText('Color')).not.toBeInTheDocument();
    expect(screen.queryByTestId('level-color-input')).not.toBeInTheDocument();
    expect(screen.queryByTestId('level-color-swatch-input')).not.toBeInTheDocument();

    await user.click(screen.getAllByTestId('edit-level-btn')[0]);
    expect(screen.queryByTestId('edit-level-color-input')).not.toBeInTheDocument();
    expect(screen.queryByTestId('edit-level-color-swatch-input')).not.toBeInTheDocument();
  });

  it('creates a level without ever submitting a color or client-side position', async () => {
    const user = userEvent.setup();
    render(<HierarchyConfig />);
    await screen.findAllByTestId('hierarchy-level-row');

    await user.click(screen.getByTestId('add-level-btn'));
    await user.type(screen.getByTestId('level-name-input'), 'Team Coach');
    await user.click(screen.getByTestId('save-level-btn'));

    await waitFor(() => expect(adminApi.createHierarchyLevel).toHaveBeenCalledTimes(1));
    const payload = (adminApi.createHierarchyLevel as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(payload.name).toBe('Team Coach');
    expect(payload).not.toHaveProperty('color');
    expect(payload).not.toHaveProperty('position');
  });

  it('updates a level without ever submitting a color', async () => {
    const user = userEvent.setup();
    render(<HierarchyConfig />);
    await screen.findAllByTestId('hierarchy-level-row');

    await user.click(screen.getAllByTestId('edit-level-btn')[0]);
    await user.click(screen.getByTestId('save-edit-btn'));

    await waitFor(() => expect(adminApi.updateHierarchyLevel).toHaveBeenCalledTimes(1));
    const payload = (adminApi.updateHierarchyLevel as unknown as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(payload).not.toHaveProperty('color');
  });

  it('shows exactly the five supported permission checkboxes in the create form', async () => {
    const user = userEvent.setup();
    render(<HierarchyConfig />);
    await screen.findAllByTestId('hierarchy-level-row');

    await user.click(screen.getByTestId('add-level-btn'));

    const supported = [
      'canViewAllTeams',
      'canEditTeams',
      'canManageUsers',
      'canTakeSurvey',
      'canViewAnalytics',
    ];
    for (const key of supported) {
      expect(screen.getByTestId(`permission-${key}`)).toBeInTheDocument();
    }
    for (const key of ['canConfigureSystem', 'canViewReports', 'canExportData']) {
      expect(screen.queryByTestId(`permission-${key}`)).not.toBeInTheDocument();
    }
    expect(screen.queryByText('Configure System')).not.toBeInTheDocument();
    expect(screen.queryByText('View Reports')).not.toBeInTheDocument();
    expect(screen.queryByText('Export Data')).not.toBeInTheDocument();
  });

  it('shows exactly the five supported permission checkboxes in the edit form, and persists a toggle on save', async () => {
    const user = userEvent.setup();
    (adminApi.updateHierarchyLevel as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(levels[0]);
    render(<HierarchyConfig />);
    await screen.findAllByTestId('hierarchy-level-row');

    await user.click(screen.getAllByTestId('edit-level-btn')[0]);

    const supported = [
      'canViewAllTeams',
      'canEditTeams',
      'canManageUsers',
      'canTakeSurvey',
      'canViewAnalytics',
    ];
    for (const key of supported) {
      expect(screen.getByTestId(`edit-permission-${key}`)).toBeInTheDocument();
    }
    for (const key of ['canConfigureSystem', 'canViewReports', 'canExportData']) {
      expect(screen.queryByTestId(`edit-permission-${key}`)).not.toBeInTheDocument();
    }

    // level-1 starts with canViewAllTeams: false - toggle it on and save.
    await user.click(screen.getByTestId('edit-permission-canViewAllTeams'));
    await user.click(screen.getByTestId('save-edit-btn'));

    await waitFor(() => expect(adminApi.updateHierarchyLevel).toHaveBeenCalledTimes(1));
    const [, payload] = (adminApi.updateHierarchyLevel as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(payload.permissions).toEqual({
      canViewAllTeams: true,
      canEditTeams: false,
      canManageUsers: false,
      canTakeSurvey: true,
      canViewAnalytics: false,
    });
  });

  it('renders levels in the order the backend returns them, with no reorder controls', async () => {
    render(<HierarchyConfig />);
    const rows = await screen.findAllByTestId('hierarchy-level-row');

    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByText(/VP/)).toBeInTheDocument();
    expect(within(rows[1]).getByText(/Director/)).toBeInTheDocument();

    expect(screen.queryByTestId('move-up-btn')).not.toBeInTheDocument();
    expect(screen.queryByTestId('move-down-btn')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Move level up')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Move level down')).not.toBeInTheDocument();
  });

  it('renders a compact row with a position badge, name, and permission chips instead of a large card', async () => {
    render(<HierarchyConfig />);
    const rows = await screen.findAllByTestId('hierarchy-level-row');

    // Position shown as a plain, non-editable ordinal badge (zero-padded),
    // not an input or reorder control.
    expect(within(rows[0]).getByText('01')).toBeInTheDocument();
    expect(within(rows[1]).getByText('02')).toBeInTheDocument();

    // Only level-1/level-2's one enabled permission (canTakeSurvey) renders
    // as a chip; the other four are omitted rather than shown as disabled.
    expect(within(rows[0]).getByText('Take Survey')).toBeInTheDocument();
    expect(within(rows[0]).queryByText('View All Teams')).not.toBeInTheDocument();
  });

  it('shows an empty state directing the user to Add Level when there are no levels', async () => {
    (adminApi.listHierarchyLevels as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);
    render(<HierarchyConfig />);

    expect(await screen.findByTestId('hierarchy-empty-state')).toHaveTextContent(/add level/i);
    expect(screen.queryByTestId('hierarchy-level-row')).not.toBeInTheDocument();
  });

  it('shows the error message inside the create modal (not hidden behind the overlay) when saving fails', async () => {
    const user = userEvent.setup();
    (adminApi.createHierarchyLevel as unknown as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error('A hierarchy level named "VP" already exists.')
    );
    render(<HierarchyConfig />);
    await screen.findAllByTestId('hierarchy-level-row');

    await user.click(screen.getByTestId('add-level-btn'));
    await user.type(screen.getByTestId('level-name-input'), 'VP');
    await user.click(screen.getByTestId('save-level-btn'));

    const modal = await screen.findByTestId('create-level-form');
    expect(await within(modal).findByText(/already exists/i)).toBeInTheDocument();
  });

  it('shows the error message inside the edit modal (not hidden behind the overlay) when saving fails', async () => {
    const user = userEvent.setup();
    (adminApi.updateHierarchyLevel as unknown as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error('A hierarchy level named "Director" already exists.')
    );
    render(<HierarchyConfig />);
    await screen.findAllByTestId('hierarchy-level-row');

    await user.click(screen.getAllByTestId('edit-level-btn')[0]);
    await user.click(screen.getByTestId('save-edit-btn'));

    const modal = await screen.findByTestId('edit-level-form');
    expect(await within(modal).findByText(/already exists/i)).toBeInTheDocument();
  });

  it('shows a permission count summary and no permission chips for a level with none enabled', async () => {
    (adminApi.listHierarchyLevels as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([
      {
        id: 'level-3',
        name: 'Observer',
        position: 3,
        permissions: {
          canViewAllTeams: false,
          canEditTeams: false,
          canManageUsers: false,
          canTakeSurvey: false,
          canViewAnalytics: false,
        },
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      },
    ]);
    render(<HierarchyConfig />);
    const row = (await screen.findAllByTestId('hierarchy-level-row'))[0];

    expect(within(row).getByText(/no permissions/i)).toBeInTheDocument();
  });
});
