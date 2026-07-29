// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

'use client';

import { useEffect, useState, useRef, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { setAuthData, LoginResponse } from '@/lib/auth';
import { API_BASE_URL } from '@/lib/api/client';
import { Loader2, AlertCircle } from 'lucide-react';

function CallbackHandler() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [error, setError] = useState('');
  const handled = useRef(false);

  useEffect(() => {
    if (handled.current) return;
    handled.current = true;
    const code = searchParams.get('code');
    const errorParam = searchParams.get('error');
    const stateParam = searchParams.get('state');

    if (errorParam) {
      setError('SSO login was cancelled or failed. Please try again.');
      return;
    }
    if (!code) {
      setError('Invalid callback: no authorization code received.');
      return;
    }

    // Verify the state nonce before anything else to prevent CSRF / session
    // mix-up attacks.  The value must match what was stored when the flow began.
    const storedState = sessionStorage.getItem('oauth_state');
    sessionStorage.removeItem('oauth_state');
    if (!storedState || stateParam !== storedState) {
      setError('Invalid login session. Please start the login process again.');
      return;
    }

    const codeVerifier = sessionStorage.getItem('pkce_verifier');
    if (!codeVerifier) {
      setError('Session expired. Please start the login process again.');
      return;
    }
    sessionStorage.removeItem('pkce_verifier');
  
    (async () => {
      try {
        const res = await fetch(`${API_BASE_URL}/api/v1/auth/sso/callback`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ code, code_verifier: codeVerifier }),
        });

        const data = await res.json();
        if (!res.ok) {
          setError(data.error || 'Login failed. Your account may not be registered.');
          return;
        }

        const loginData: LoginResponse = data;
        setAuthData(loginData);

        const { hierarchyLevel } = loginData.user;
        if (hierarchyLevel === 'admin' || hierarchyLevel === 'level-admin') {
          router.replace('/admin');
        } else if (['level-1', 'level-2', 'level-3'].includes(hierarchyLevel)) {
          router.replace('/manager');
        } else if (hierarchyLevel === 'level-4') {
          router.replace('/dashboard');
        } else {
          router.replace('/home');
        }
      } catch {
        setError('Network error during authentication. Please try again.');
      }
    })();
  }, [router, searchParams]);

  if (error) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
        <div className="bg-white rounded-xl shadow-lg p-8 max-w-sm w-full text-center">
          <AlertCircle className="w-12 h-12 text-red-500 mx-auto mb-4" />
          <h2 className="text-lg font-semibold text-gray-900 mb-2">Sign-in failed</h2>
          <p className="text-sm text-gray-600 mb-6">{error}</p>
          <button
            onClick={() => router.replace('/login')}
            className="w-full bg-indigo-600 text-white py-2.5 rounded-lg font-medium hover:bg-indigo-700 transition-colors"
          >
            Back to login
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
      <div className="bg-white rounded-xl shadow-lg p-8 max-w-sm w-full text-center">
        <Loader2 className="w-10 h-10 text-indigo-600 animate-spin mx-auto mb-4" />
        <p className="text-gray-600 font-medium">Completing sign-in...</p>
        <p className="text-sm text-gray-400 mt-1">Please wait while we verify your identity.</p>
      </div>
    </div>
  );
}

// useSearchParams requires a Suspense boundary in Next.js App Router
export default function AuthCallbackPage() {
  return (
    <Suspense
      fallback={
        <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
          <Loader2 className="w-10 h-10 text-indigo-600 animate-spin" />
        </div>
      }
    >
      <CallbackHandler />
    </Suspense>
  );
}
