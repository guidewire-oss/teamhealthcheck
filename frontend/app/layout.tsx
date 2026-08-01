import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { TelemetryProvider } from "@/components/TelemetryProvider";
import AuthGuard from "@/components/AuthGuard";
import ThemeToggle from "@/components/ThemeToggle";

const inter = Inter({ subsets: ["latin"] });

const themeScript = `
  (function() {
    try {
      const storageKey = 'theme';
      const storedTheme = window.localStorage.getItem(storageKey);
      const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      const theme = storedTheme === 'dark' || storedTheme === 'light' ? storedTheme : systemTheme;

      document.documentElement.setAttribute('data-theme', theme);
      document.documentElement.classList.toggle('dark', theme === 'dark');
      document.documentElement.style.colorScheme = theme;
    } catch (e) {
      document.documentElement.setAttribute('data-theme', 'light');
      document.documentElement.classList.remove('dark');
    }
  })();
`;

export const metadata: Metadata = {
  title: "Team360 Health Check",
  description: "Squad Health Check Model - Track and improve your team&apos;s health",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className={inter.className}>
        <ThemeToggle />
        <TelemetryProvider>
          <AuthGuard>
            {children}
          </AuthGuard>
        </TelemetryProvider>
      </body>
    </html>
  );
}
