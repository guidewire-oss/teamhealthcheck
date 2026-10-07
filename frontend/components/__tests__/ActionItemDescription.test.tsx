import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ActionItemDescription from '../ActionItemDescription';

// jsdom doesn't run layout, so scrollHeight/clientHeight are both 0 by
// default — which already reads as "not truncated". These helpers let each
// test simulate the clamped-overflow case explicitly.
function mockTruncated(scrollHeight: number, clientHeight: number) {
  vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(scrollHeight);
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(clientHeight);
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ActionItemDescription', () => {
  it('renders no toggle when the text fits within the clamp', () => {
    mockTruncated(40, 40); // scrollHeight === clientHeight -> fits
    render(<ActionItemDescription description="Short description." />);

    expect(screen.getByText('Short description.')).toBeInTheDocument();
    expect(screen.queryByTestId('action-item-description-toggle')).not.toBeInTheDocument();
  });

  it('renders a "Show more" toggle when the text is actually truncated', () => {
    mockTruncated(100, 40); // scrollHeight > clientHeight -> clamped
    render(<ActionItemDescription description="A much longer description that overflows two lines." />);

    expect(screen.getByTestId('action-item-description-toggle')).toHaveTextContent('Show more');
  });

  it('expands to "Show less" on click, and collapses back on a second click', async () => {
    mockTruncated(100, 40);
    const user = userEvent.setup();
    render(<ActionItemDescription description={'Line one.\nLine two.'} />);

    const toggle = screen.getByTestId('action-item-description-toggle');
    const text = screen.getByTestId('action-item-description-text');

    expect(toggle).toHaveTextContent('Show more');
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(text.className).toContain('line-clamp-2');

    await user.click(toggle);
    expect(toggle).toHaveTextContent('Show less');
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(text.className).not.toContain('line-clamp-2');

    await user.click(toggle);
    expect(toggle).toHaveTextContent('Show more');
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(text.className).toContain('line-clamp-2');
  });

  it('wires aria-expanded/aria-controls to the paragraph id, and keeps the full text in the DOM even while clamped', () => {
    mockTruncated(100, 40);
    const fullText = 'testing checking if expired date works and if manager is able to get assigned. my manager is john.';
    render(<ActionItemDescription description={fullText} />);

    const toggle = screen.getByTestId('action-item-description-toggle');
    const text = screen.getByTestId('action-item-description-text');

    expect(toggle.getAttribute('aria-controls')).toBe(text.id);
    expect(text).toHaveTextContent(fullText);
  });

  it('preserves line breaks and wraps long unbroken text via whitespace-pre-wrap and break-words', () => {
    mockTruncated(40, 40);
    render(<ActionItemDescription description={'line one\nline two'} />);
    const text = screen.getByTestId('action-item-description-text');
    expect(text.className).toContain('whitespace-pre-wrap');
    expect(text.className).toContain('break-words');
  });

  it('keeps expansion state independent across multiple description instances', async () => {
    mockTruncated(100, 40);
    const user = userEvent.setup();
    render(
      <>
        <ActionItemDescription description="First card's long truncated description text." />
        <ActionItemDescription description="Second card's long truncated description text." />
      </>
    );

    const toggles = screen.getAllByTestId('action-item-description-toggle');
    expect(toggles).toHaveLength(2);

    await user.click(toggles[0]);
    expect(toggles[0]).toHaveTextContent('Show less');
    expect(toggles[1]).toHaveTextContent('Show more');
  });
});
