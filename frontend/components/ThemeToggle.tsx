'use client';

import { useEffect, useState } from 'react';

const STORAGE_KEY = 'theme';

type Theme = 'light' | 'dark';

function getSystemTheme(): Theme {
  if (typeof window === 'undefined') {
    return 'light';
  }

  if (typeof window.matchMedia !== 'function') {
    return 'light';
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function getBootstrappedTheme(): Theme {
  if (typeof document === 'undefined') {
    return 'light';
  }

  const bootstrappedTheme = document.documentElement.getAttribute('data-theme');
  if (bootstrappedTheme === 'dark' || bootstrappedTheme === 'light') {
    return bootstrappedTheme;
  }

  if (document.documentElement.classList.contains('dark')) {
    return 'dark';
  }

  return getSystemTheme();
}

function getStoredTheme(): Theme | null {
  if (typeof window === 'undefined') {
    return null;
  }

  try {
    const storedTheme = window.localStorage.getItem(STORAGE_KEY);
    return storedTheme === 'dark' || storedTheme === 'light' ? storedTheme : null;
  } catch {
    return null;
  }
}

function applyTheme(theme: Theme): void {
  if (typeof document === 'undefined') {
    return;
  }

  document.documentElement.setAttribute('data-theme', theme);
  document.documentElement.classList.toggle('dark', theme === 'dark');
  document.documentElement.style.colorScheme = theme;
}

function persistTheme(theme: Theme): void {
  if (typeof window === 'undefined') {
    return;
  }

  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // Ignore storage errors in browser privacy modes.
  }
}

export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>('light');

  useEffect(() => {
    const resolvedTheme = getStoredTheme() ?? getBootstrappedTheme();
    setTheme(resolvedTheme);
    applyTheme(resolvedTheme);
    persistTheme(resolvedTheme);
  }, []);

  useEffect(() => {
    applyTheme(theme);
    persistTheme(theme);
  }, [theme]);

  const toggleTheme = () => {
    setTheme((currentTheme) => (currentTheme === 'dark' ? 'light' : 'dark'));
  };

  return (
    <button
      type="button"
      onClick={toggleTheme}
      className="fixed right-4 bottom-4 z-50 rounded-full border border-gray-300 bg-white/90 px-3 py-2 text-sm font-medium text-gray-800 shadow-sm backdrop-blur transition hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-900/90 dark:text-gray-100 dark:hover:bg-gray-800"
      aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}
      aria-pressed={theme === 'dark'}
    >
      {theme === 'dark' ? '☀️ Light' : '🌙 Dark'}
    </button>
  );
}
