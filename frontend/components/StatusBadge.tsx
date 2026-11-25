interface Props {
  /** False for a soft-deactivated person or team. Missing is treated as active. */
  isActive?: boolean;
  /** data-testid for the badge; defaults to the one the Users tab has always used. */
  testId?: string;
}

/**
 * Shows whether a person or team is currently Active or Inactive.
 *
 * The state is spelled out in words, not only colour, so it reads correctly for
 * screen readers and in grayscale. It is meant to sit immediately before a
 * person's or team's name. Inactive ones are never hidden: they stay visible
 * for historical records.
 */
export default function StatusBadge({ isActive = true, testId = "user-status-badge" }: Props) {
  const label = isActive ? "Active" : "Inactive";
  return (
    <span
      data-testid={testId}
      data-status={label.toLowerCase()}
      className={`mr-2 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
        isActive ? "bg-green-100 text-green-800" : "bg-gray-200 text-gray-700"
      }`}
    >
      {label}
    </span>
  );
}
