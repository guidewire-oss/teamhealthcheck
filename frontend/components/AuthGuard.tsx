// SPDX-License-Identifier: Apache-2.0

'use client';

import { useEffect, useState } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { getCurrentUser } from '@/lib/auth';

const PUBLIC_PATHS = ['/', '/login', '/auth/callback'];

export default function AuthGuard({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [authorized, setAuthorized] = useState(false);

  useEffect(() => {
    setAuthorized(false);
    const user = getCurrentUser();
    const isPublicPath = PUBLIC_PATHS.includes(pathname);

    if (!user && !isPublicPath) {
      router.replace('/login');
      return;
    }

    if (user) {
      const hierarchyLevelId = user.hierarchyLevel || user.hierarchyLevelId;

      if (pathname === '/login') {
        if (hierarchyLevelId === 'admin' || hierarchyLevelId === 'level-admin') {
          router.replace('/admin');
        } else if (hierarchyLevelId === 'level-1' || hierarchyLevelId === 'level-2' || hierarchyLevelId === 'level-3') {
          router.replace('/manager');
        } else if (hierarchyLevelId === 'level-4') {
          router.replace('/dashboard');
        } else if (hierarchyLevelId === 'level-5') {
          router.replace('/home');
        } else {
          router.replace('/home');
        }
        return;
      }

      if (pathname === '/survey') {
        if (hierarchyLevelId !== 'level-4' && hierarchyLevelId !== 'level-5') {
          if (hierarchyLevelId === 'admin' || hierarchyLevelId === 'level-admin') {
            router.replace('/admin');
          } else {
            router.replace('/manager');
          }
          return;
        }
      }
    }

    setAuthorized(true);
  }, [pathname, router]);

  if (!authorized) {
    return null;
  }

  return <>{children}</>;
}
