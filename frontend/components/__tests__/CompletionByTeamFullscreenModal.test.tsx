import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CompletionByTeamFullscreenModal from "@/components/CompletionByTeamFullscreenModal";
import type { SurveyCompletionData } from "@/components/SurveyCompletionDashboard";

const teamStats: SurveyCompletionData["teamStats"] = [
  { teamId: "t1", teamName: "Team One", completed: 3, total: 5, status: "in_progress", postWorkshopCompleted: null },
  { teamId: "t2", teamName: "Team Two", completed: 5, total: 5, status: "complete", postWorkshopCompleted: null },
];

function renderModal(onClose = vi.fn()) {
  const utils = render(<CompletionByTeamFullscreenModal teamStats={teamStats} onClose={onClose} />);
  return { onClose, ...utils };
}

describe("CompletionByTeamFullscreenModal accessibility", () => {
  it("moves initial focus to the close button", () => {
    renderModal();
    expect(screen.getByTestId("team-bars-fullscreen-close")).toHaveFocus();
  });

  it("restores focus to the trigger element on unmount", () => {
    const trigger = document.createElement("button");
    trigger.textContent = "Expand";
    document.body.appendChild(trigger);
    trigger.focus();

    const { unmount } = renderModal();
    expect(screen.getByTestId("team-bars-fullscreen-close")).toHaveFocus();

    unmount();
    expect(trigger).toHaveFocus();
    trigger.remove();
  });

  it("closes on Escape", async () => {
    const user = userEvent.setup();
    const { onClose } = renderModal();

    await user.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalled();
  });

  it("wraps Tab forward from the last focusable element back to the first", async () => {
    const user = userEvent.setup();
    renderModal();

    const dialog = screen.getByTestId("team-bars-fullscreen-modal");
    const focusable = dialog.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
    );
    expect(focusable.length).toBeGreaterThan(0);
    const last = focusable[focusable.length - 1];
    const first = focusable[0];

    last.focus();
    await user.keyboard("{Tab}");

    expect(first).toHaveFocus();
  });

  it("wraps Shift+Tab backward from the first focusable element to the last", async () => {
    const user = userEvent.setup();
    renderModal();

    const dialog = screen.getByTestId("team-bars-fullscreen-modal");
    const focusable = dialog.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
    );
    const last = focusable[focusable.length - 1];

    screen.getByTestId("team-bars-fullscreen-close").focus();
    await user.keyboard("{Shift>}{Tab}{/Shift}");

    expect(last).toHaveFocus();
  });
});
