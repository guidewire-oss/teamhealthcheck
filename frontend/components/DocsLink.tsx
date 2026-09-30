'use client';

import { BookOpen } from 'lucide-react';

const DOCS_URL = process.env.NEXT_PUBLIC_DOCS_URL || '/docs/';

export default function DocsLink() {
  return (
    <a
      href={DOCS_URL}
      target="_blank"
      rel="noopener noreferrer"
      title="Documentation"
      className="flex items-center gap-2 px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors"
    >
      <BookOpen className="w-4 h-4" />
      Docs
    </a>
  );
}
