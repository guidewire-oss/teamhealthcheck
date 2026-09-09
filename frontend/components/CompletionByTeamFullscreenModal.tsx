"use client";

import { useEffect, useRef } from "react";
import { X } from "lucide-react";
import CompletionByTeamPanel from "./CompletionByTeamPanel";
import type { SurveyCompletionData } from "./SurveyCompletionDashboard";

interface CompletionByTeamFullscreenModalProps {
  teamStats: SurveyCompletionData["teamStats"];
  onClose: () => void;
}

const TITLE_ID = "team-bars-fullscreen-title";

/**
 * Fullscreen overlay for "Completion by team" -- renders the exact same
 * CompletionByTeamPanel the dashboard card does (status filters, search,
 * sorted bar chart), just at a larger size, so there is no separate copy
 * of the filter/search/sort logic to keep in sync with the card.
 */
export default function CompletionByTeamFullscreenModal({
  teamStats,
  onClose,
}: CompletionByTeamFullscreenModalProps) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);

    // Prevent page-level scrolling while the overlay is open; the panel's
    // own internal scroll area (team-bars-scroll-area) still scrolls
    // independently since it's a separate overflow-y-auto region.
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    // Move focus into the dialog on open (the close button is the first
    // focusable element), and restore it to whatever triggered the modal
    // (the expand button) on close -- a screen reader or keyboard user is
    // otherwise left with focus still "inside" a now-invisible page, or
    // dropped back to the top of the document instead of where they were.
    previouslyFocused.current = document.activeElement as HTMLElement | null;
    closeButtonRef.current?.focus();

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = previousOverflow;
      previouslyFocused.current?.focus();
    };
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4"
      data-testid="team-bars-fullscreen-overlay"
    >
      <div
        className="bg-white rounded-xl shadow-xl w-full h-full max-w-6xl max-h-[85vh] flex flex-col"
        data-testid="team-bars-fullscreen-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={TITLE_ID}
      >
        <div className="flex items-center justify-between p-5 border-b flex-shrink-0">
          <div>
            <h3 id={TITLE_ID} className="text-base font-semibold text-gray-900">
              Completion by team
            </h3>
            <p className="text-xs text-gray-500 mt-0.5">Filter by status, search, and review every pod</p>
          </div>
          <button
            ref={closeButtonRef}
            type="button"
            onClick={onClose}
            data-testid="team-bars-fullscreen-close"
            aria-label="Close fullscreen view"
            className="text-gray-400 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-indigo-500 rounded"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="flex-1 min-h-0 p-5">
          <CompletionByTeamPanel teamStats={teamStats} />
        </div>
      </div>
    </div>
  );
}
