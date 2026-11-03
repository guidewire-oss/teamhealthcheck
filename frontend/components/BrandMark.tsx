'use client';

import { Building2 } from 'lucide-react';

interface BrandMarkProps {
  /** Admin-configured logo, as a data URI, or null/empty when none is configured. */
  logoUrl?: string | null;
  /** Admin-configured company name; falls back to "Team Health Check" when empty. */
  companyName?: string;
  /** 'header' (~36px) for in-app page headers, 'login' (~44px) for the sign-in card. */
  size?: 'header' | 'login';
  className?: string;
}

const SIZE_STYLES = {
  header: { box: 'w-9 h-9', icon: 'w-5 h-5', text: 'text-sm' },
  login: { box: 'w-11 h-11', icon: 'w-6 h-6', text: 'text-base' },
} as const;

/**
 * Compact organization identity mark: a fixed-size neutral avatar (logo or
 * Building2 fallback) beside the company name. Used consistently across
 * Login, Survey, and the Team Lead/Manager/Member dashboards so branding
 * never dominates the page it appears on.
 */
export default function BrandMark({
  logoUrl,
  companyName,
  size = 'header',
  className = '',
}: BrandMarkProps) {
  const s = SIZE_STYLES[size];
  const displayName = companyName || 'Team Health Check';

  return (
    <div className={`flex items-center gap-2.5 min-w-0 ${className}`} data-testid="brand-mark">
      <div
        className={`${s.box} flex-shrink-0 rounded-lg border border-gray-200 bg-gray-50 flex items-center justify-center overflow-hidden`}
      >
        {logoUrl ? (
          // eslint-disable-next-line @next/next/no-img-element -- small fixed-size avatar rendering an admin-configured data-URI logo
          <img
            src={logoUrl}
            alt={companyName ? `${companyName} logo` : 'Company logo'}
            className="w-full h-full object-contain p-1"
          />
        ) : (
          <Building2 className={`${s.icon} text-indigo-600`} />
        )}
      </div>
      <span className={`${s.text} font-semibold text-gray-600 truncate min-w-0`}>
        {displayName}
      </span>
    </div>
  );
}
