// SPDX-License-Identifier: Apache-2.0

import TeamResultsClient from './TeamResultsClient';

// Static export requires at least one param. The placeholder is never used —
// all real team pages are client-rendered via the SPA fallback in Go.
export async function generateStaticParams() {
  return [{ teamId: '_' }];
}

export default function TeamPage() {
  return <TeamResultsClient />;
}
