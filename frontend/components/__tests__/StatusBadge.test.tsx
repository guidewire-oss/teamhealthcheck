import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import StatusBadge from '../StatusBadge';

describe('StatusBadge', () => {
  it('says Active for an active person', () => {
    render(<StatusBadge isActive />);
    expect(screen.getByTestId('user-status-badge')).toHaveTextContent('Active');
    expect(screen.getByTestId('user-status-badge')).toHaveAttribute('data-status', 'active');
  });

  it('says Inactive for a soft-deactivated person, in words not just colour', () => {
    render(<StatusBadge isActive={false} />);
    expect(screen.getByTestId('user-status-badge')).toHaveTextContent('Inactive');
    expect(screen.getByTestId('user-status-badge')).toHaveAttribute('data-status', 'inactive');
  });

  it('treats a missing value as active', () => {
    render(<StatusBadge />);
    expect(screen.getByTestId('user-status-badge')).toHaveTextContent('Active');
  });

  it('uses a custom test id when asked, so teams and users can be told apart', () => {
    render(<StatusBadge isActive={false} testId="team-status-badge" />);
    expect(screen.getByTestId('team-status-badge')).toHaveTextContent('Inactive');
    expect(screen.queryByTestId('user-status-badge')).toBeNull();
  });
});
