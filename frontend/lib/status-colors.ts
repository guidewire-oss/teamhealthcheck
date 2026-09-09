/**
 * Single source of truth for survey-status labels and colors, shared by the
 * main completion table's status chips (SurveyCompletionDashboard) and the
 * reminder-target tree's legend/dots (ReminderTreeModal) — so a color always
 * means the same thing everywhere it appears.
 */

import type { SurveyStatus } from '@/lib/api/survey-completion';

export const STATUS_LABEL: Record<SurveyStatus, string> = {
  complete: 'Complete',
  in_progress: 'In progress',
  not_started: 'Not started',
  opted_out: 'Opted out',
};

// The chip's background color alone — used for the status chips themselves.
export const STATUS_CHIP_BG_CLASS: Record<SurveyStatus, string> = {
  complete: 'bg-green-100',
  in_progress: 'bg-amber-100',
  not_started: 'bg-red-100',
  opted_out: 'bg-gray-100',
};

const STATUS_CHIP_TEXT_CLASS: Record<SurveyStatus, string> = {
  complete: 'text-green-800',
  in_progress: 'text-amber-800',
  not_started: 'text-red-800',
  opted_out: 'text-gray-600',
};

// A small isolated dot (the reminder tree's leaves, its legend) has no text
// or border to lean on, so the chip's pale bg-*-100 reads as barely-there —
// use a more saturated shade instead. Tree dots and legend swatches both
// pull from this so the two stay identical to each other.
export const STATUS_DOT_CLASS: Record<SurveyStatus, string> = {
  complete: 'bg-green-500',
  in_progress: 'bg-amber-500',
  not_started: 'bg-red-500',
  opted_out: 'bg-gray-400',
};

export const STATUS_BADGE_CLASS: Record<SurveyStatus, string> = {
  complete: `${STATUS_CHIP_BG_CLASS.complete} ${STATUS_CHIP_TEXT_CLASS.complete}`,
  in_progress: `${STATUS_CHIP_BG_CLASS.in_progress} ${STATUS_CHIP_TEXT_CLASS.in_progress}`,
  not_started: `${STATUS_CHIP_BG_CLASS.not_started} ${STATUS_CHIP_TEXT_CLASS.not_started}`,
  opted_out: `${STATUS_CHIP_BG_CLASS.opted_out} ${STATUS_CHIP_TEXT_CLASS.opted_out}`,
};

// Canonical display order for anything that lists multiple statuses at once
// (e.g. the reminder tree's legend).
export const STATUS_ORDER: SurveyStatus[] = ['in_progress', 'not_started', 'complete', 'opted_out'];
