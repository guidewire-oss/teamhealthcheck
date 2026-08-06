'use client';

import { useState } from 'react';
import { X, ChevronRight, ChevronLeft } from 'lucide-react';

interface OnboardingModalProps {
  userLevel: string; // e.g. 'level-5', 'level-4', 'level-3' ...
  onDismiss: () => void;
}

const SLIDES = [
  {
    title: 'Welcome to Teams360',
    content: (
      <div className="space-y-3 text-sm text-gray-600">
        <p>
          Teams360 is a safe space for your team to reflect on how you are working together.
          It is based on the{' '}
          <span className="font-medium text-gray-800">Spotify Squad Health Check Model</span>.
        </p>
        <p>
          Your answers help your team identify areas that need support and track improvements
          over time.
        </p>
      </div>
    ),
  },
  {
    title: 'How scoring works',
    content: (
      <div className="space-y-4 text-sm text-gray-600">
        <p>Each dimension gets one of three scores:</p>
        <div className="space-y-2.5">
          {[
            {
              color: 'bg-green-500',
              label: 'Green',
              desc: 'We\'re doing great here — no blockers, keep it up.',
            },
            {
              color: 'bg-yellow-400',
              label: 'Yellow',
              desc: 'Some issues, but we\'re aware and working on it.',
            },
            {
              color: 'bg-red-500',
              label: 'Red',
              desc: 'Struggling here — we need support in this area.',
            },
          ].map(({ color, label, desc }) => (
            <div key={label} className="flex items-start gap-3">
              <span className={`mt-0.5 inline-block w-4 h-4 rounded-full flex-shrink-0 ${color}`} />
              <p>
                <span className="font-semibold text-gray-800">{label}</span> — {desc}
              </p>
            </div>
          ))}
        </div>
        <p className="text-xs text-indigo-600 font-medium bg-indigo-50 rounded-lg px-3 py-2">
          Being honest matters more than looking good. Red means your team needs support — not that it has failed.
        </p>
      </div>
    ),
  },
  {
    title: 'What happens next',
    content: null, // rendered dynamically based on role
  },
];

function getRoleSlideContent(userLevel: string) {
  if (userLevel === 'level-5') {
    return (
      <div className="space-y-3 text-sm text-gray-600">
        <p>After you submit:</p>
        <ul className="space-y-2 list-none">
          {[
            'Your Team Lead sees aggregated results for the whole team.',
            'Individual responses are visible to your Team Lead — be honest.',
            'Your Team Lead creates action items to address weak areas.',
            'You\'ll be asked again next period to track progress.',
          ].map((item) => (
            <li key={item} className="flex items-start gap-2">
              <span className="mt-1 w-1.5 h-1.5 rounded-full bg-indigo-400 flex-shrink-0" />
              {item}
            </li>
          ))}
        </ul>
      </div>
    );
  }
  if (userLevel === 'level-4') {
    return (
      <div className="space-y-3 text-sm text-gray-600">
        <p>As a Team Lead you can:</p>
        <ul className="space-y-2 list-none">
          {[
            'View your team\'s health on the Dashboard — radar, distribution, individual responses, and trends.',
            'Use the Actions tab to create improvement tasks tied to specific dimensions.',
            'Take the survey yourself to contribute your own perspective.',
            'After the team workshop, fill out the post-workshop survey to record the collective outcome agreed upon by the team.',
            'Track your team\'s progress across assessment periods.',
          ].map((item) => (
            <li key={item} className="flex items-start gap-2">
              <span className="mt-1 w-1.5 h-1.5 rounded-full bg-indigo-400 flex-shrink-0" />
              {item}
            </li>
          ))}
        </ul>
      </div>
    );
  }
  // Manager / Director / VP
  return (
    <div className="space-y-3 text-sm text-gray-600">
      <p>As a manager you can:</p>
      <ul className="space-y-2 list-none">
        {[
          'See health trends across all teams you supervise on the Manager dashboard.',
          'Drill into any team\'s detail — scores, distributions, and individual responses.',
          'Monitor action item completion rates alongside health scores.',
          'Compare periods to spot systemic patterns across your org.',
        ].map((item) => (
          <li key={item} className="flex items-start gap-2">
            <span className="mt-1 w-1.5 h-1.5 rounded-full bg-indigo-400 flex-shrink-0" />
            {item}
          </li>
        ))}
      </ul>
    </div>
  );
}

export default function OnboardingModal({ userLevel, onDismiss }: OnboardingModalProps) {
  const [slide, setSlide] = useState(0);
  const isLast = slide === SLIDES.length - 1;

  const handleBackdropClick = () => {
    // Backdrop click closes but does NOT set the "done" flag — will re-show next login
    onDismiss();
  };

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
      data-testid="onboarding-modal"
      onClick={handleBackdropClick}
    >
      <div
        className="bg-white rounded-2xl shadow-2xl w-full max-w-md"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 pt-6 pb-4">
          <div className="flex gap-1.5">
            {SLIDES.map((_, i) => (
              <span
                key={i}
                className={`h-1.5 rounded-full transition-all duration-300 ${
                  i === slide ? 'w-6 bg-indigo-600' : 'w-1.5 bg-gray-200'
                }`}
              />
            ))}
          </div>
          <button
            onClick={onDismiss}
            className="text-gray-400 hover:text-gray-600 transition-colors"
            aria-label="Close"
            data-testid="onboarding-close-btn"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body */}
        <div className="px-6 pb-4 min-h-[200px]">
          <h2 className="text-lg font-semibold text-gray-900 mb-3">
            {SLIDES[slide].title}
          </h2>
          {slide === 2
            ? getRoleSlideContent(userLevel)
            : SLIDES[slide].content}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-gray-100">
          {slide > 0 ? (
            <button
              onClick={() => setSlide((s) => s - 1)}
              className="flex items-center gap-1 text-sm text-gray-500 hover:text-gray-700 transition-colors"
            >
              <ChevronLeft className="w-4 h-4" />
              Back
            </button>
          ) : (
            <div />
          )}

          {isLast ? (
            <button
              onClick={onDismiss}
              className="px-5 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 transition-colors"
              data-testid="onboarding-dismiss-btn"
            >
              Got it, let&apos;s go!
            </button>
          ) : (
            <button
              onClick={() => setSlide((s) => s + 1)}
              className="flex items-center gap-1.5 px-5 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 transition-colors"
            >
              Next
              <ChevronRight className="w-4 h-4" />
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
