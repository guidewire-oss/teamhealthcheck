// SPDX-License-Identifier: Apache-2.0

import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { TelemetryProvider } from "@/components/TelemetryProvider";
import AuthGuard from "@/components/AuthGuard";

const inter = Inter({ subsets: ["latin"] });

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
    <html lang="en">
      <body className={inter.className}>
        <TelemetryProvider>
          <AuthGuard>
            {children}
          </AuthGuard>
        </TelemetryProvider>
      </body>
    </html>
  );
}
