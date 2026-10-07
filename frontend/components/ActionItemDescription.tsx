'use client';

import { useEffect, useId, useRef, useState } from 'react';
import { ChevronDown } from 'lucide-react';

interface ActionItemDescriptionProps {
  description: string;
}

/**
 * Shared description renderer for action-item cards (Team Lead board and the
 * read-only Manager/Director/VP pod board). Clamps to 2 lines by default and
 * reveals a "Show more" toggle only when the text actually overflows that
 * clamp, so short descriptions never show a dead control.
 */
export default function ActionItemDescription({ description }: ActionItemDescriptionProps) {
  const paragraphId = useId();
  const textRef = useRef<HTMLParagraphElement>(null);
  const [isTruncated, setIsTruncated] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);

  useEffect(() => {
    // Truncation can only be measured while the 2-line clamp is applied, so
    // skip re-measuring once expanded — that would always read as "not
    // truncated" and hide the control needed to collapse back.
    if (isExpanded) return;

    const checkTruncation = () => {
      const el = textRef.current;
      if (!el) return;
      setIsTruncated(el.scrollHeight > el.clientHeight + 1);
    };
    checkTruncation();
    window.addEventListener('resize', checkTruncation);
    return () => window.removeEventListener('resize', checkTruncation);
  }, [description, isExpanded]);

  return (
    <div className="mt-1">
      <p
        id={paragraphId}
        ref={textRef}
        data-testid="action-item-description-text"
        className={`text-sm leading-relaxed text-gray-500 whitespace-pre-wrap break-words ${isExpanded ? '' : 'line-clamp-2'}`}
      >
        {description}
      </p>
      {isTruncated && (
        <button
          type="button"
          onClick={() => setIsExpanded((prev) => !prev)}
          aria-expanded={isExpanded}
          aria-controls={paragraphId}
          className="flex items-center gap-1 text-xs font-medium text-indigo-600 hover:text-indigo-800 mt-0.5"
          data-testid="action-item-description-toggle"
        >
          <ChevronDown
            className={`w-3 h-3 transition-transform duration-200 ${isExpanded ? 'rotate-180' : ''}`}
          />
          {isExpanded ? 'Show less' : 'Show more'}
        </button>
      )}
    </div>
  );
}
