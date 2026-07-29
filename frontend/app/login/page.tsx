// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

'use client';

import { Suspense, useState, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { getOrgConfig } from '@/lib/org-config';
import { setAuthData, LoginResponse } from '@/lib/auth';
import { startSSOFlow, OAuthConfig } from '@/lib/sso';
import { API_BASE_URL } from '@/lib/api/client';
import { Lock, User, AlertCircle, Users, LogIn } from 'lucide-react';

export default function LoginPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
        <p className="text-gray-500">Loading...</p>
      </div>
    }>
      <LoginPageContent />
    </Suspense>
  );
}

function LoginPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const sessionExpired = searchParams.get('expired') === 'true';
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [ssoLoading, setSSOLoading] = useState(false);
  const [ssoConfig, setSsoConfig] = useState<OAuthConfig | null>(null);
  const [isDemoMode, setIsDemoMode] = useState(false);
  const config = getOrgConfig();

  useEffect(() => {
    fetch(`${API_BASE_URL}/api/v1/config`)
      .then(res => res.json())
      .then(data => {
        if (data.appEnv === 'demo') setIsDemoMode(true);
        if (data.sso) setSsoConfig(data.sso as OAuthConfig);
      })
      .catch(() => {});
  }, []);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      // Call backend authentication API via Next.js proxy
      const response = await fetch(`${API_BASE_URL}/api/v1/auth/login`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ username, password }),
      });

      if (!response.ok) {
        const errorData = await response.json();
        setError(errorData.error || 'Invalid username or password');
        return;
      }

      const data: LoginResponse = await response.json();
      const user = data.user;

      // Store JWT tokens and user data using the auth module
      setAuthData(data);

      // Route based on permissions
      if (user.hierarchyLevel === 'admin' || user.hierarchyLevel === 'level-admin') {
        // Admins go to admin dashboard
        router.push('/admin');
      } else if (user.hierarchyLevel === 'level-1' || user.hierarchyLevel === 'level-2' || user.hierarchyLevel === 'level-3') {
        // VPs, Directors, Managers go to manager dashboard
        router.push('/manager');
      } else if (user.hierarchyLevel === 'level-4') {
        // Team leads go to dashboard (their own team view)
        router.push('/dashboard');
      } else {
        // Team members (level-5) go to home page with survey history
        router.push('/home');
      }
    } catch (err) {
      setError('Unable to connect. Please try again later.');
    }
  };


  const handleSSOLogin = async () => {
    if (!ssoConfig) return;
    setSSOLoading(true);
    setError('');
    try {
      await startSSOFlow(ssoConfig);
    } catch {
      setError('Failed to start SSO login. Please try again.');
      setSSOLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
      <div className="max-w-4xl w-full">
        <div className="bg-white rounded-2xl shadow-xl overflow-hidden">
          <div className={`grid ${isDemoMode ? 'md:grid-cols-2' : ''}`}>
            {/* Login Form */}
            <div className="p-8">
              <div className="text-center mb-8">
                <div className="inline-flex items-center justify-center w-16 h-16 bg-indigo-100 rounded-full mb-4">
                  <Lock className="w-8 h-8 text-indigo-600" />
                </div>
                <h1 className="text-3xl font-bold text-gray-900">Team Health Check</h1>
                <p className="text-gray-500 mt-2">Sign in to continue</p>
              </div>

              {sessionExpired && (
                <div
                  data-testid="session-expired-banner"
                  className="mb-6 flex items-center gap-2 text-amber-700 text-sm bg-amber-50 border border-amber-200 p-3 rounded-lg"
                >
                  <AlertCircle className="w-4 h-4 flex-shrink-0" />
                  <span>Your session has expired. Please log in again.</span>
                </div>
              )}

              <form onSubmit={handleLogin} className="space-y-6">
                <div>
                  <label htmlFor="username" className="block text-sm font-medium text-gray-700 mb-2">
                    Username
                  </label>
                  <div className="relative">
                    <input
                      id="username"
                      name="username"
                      type="text"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      className="w-full px-4 py-3 pl-10 border border-gray-300 rounded-lg text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent placeholder:text-gray-400"
                      placeholder="Enter username"
                      required
                    />
                    <User className="w-5 h-5 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2" />
                  </div>
                </div>

                <div>
                  <label htmlFor="password" className="block text-sm font-medium text-gray-700 mb-2">
                    Password
                  </label>
                  <div className="relative">
                    <input
                      id="password"
                      name="password"
                      type="password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      className="w-full px-4 py-3 pl-10 border border-gray-300 rounded-lg text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent placeholder:text-gray-400"
                      placeholder="Enter password"
                      required
                    />
                    <Lock className="w-5 h-5 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2" />
                  </div>
                </div>

                {error && (
                  <div className="flex items-center gap-2 text-red-600 text-sm bg-red-50 p-3 rounded-lg" data-testid="login-error">
                    <AlertCircle className="w-4 h-4" />
                    <span>{error}</span>
                  </div>
                )}

                <button
                  type="submit"
                  className="w-full bg-indigo-600 text-white py-3 rounded-lg font-semibold hover:bg-indigo-700 transition-colors"
                >
                  Sign In
                </button>
              </form>

              {ssoConfig && (
                <>
                  <div className="flex items-center gap-3 mt-6">
                    <div className="flex-1 h-px bg-gray-200" />
                    <span className="text-sm text-gray-400">or</span>
                    <div className="flex-1 h-px bg-gray-200" />
                  </div>
                  <button
                    onClick={handleSSOLogin}
                    disabled={ssoLoading}
                    className="w-full mt-4 flex items-center justify-center gap-2 border border-gray-300 text-gray-700 py-3 rounded-lg font-semibold hover:bg-gray-50 transition-colors disabled:opacity-60 disabled:cursor-not-allowed"
                  >
                    <LogIn className="w-5 h-5" />
                    {ssoLoading ? 'Redirecting\u2026' : 'Sign in with SSO'}
                  </button>
                </>
              )}
            </div>

            {/* Demo Credentials Info - only shown when APP_ENV=demo */}
            {isDemoMode && <div className="bg-gradient-to-br from-indigo-50 to-blue-50 p-8 border-l">
              <h3 className="text-lg font-semibold text-gray-900 mb-6 flex items-center gap-2">
                <Users className="w-5 h-5 text-indigo-600" />
                Demo Login Credentials
              </h3>

              <div className="space-y-4">
                <div className="bg-white rounded-lg p-4 border border-indigo-100">
                  <h4 className="font-semibold text-gray-900 mb-3 text-sm">Organizational Hierarchy</h4>
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between items-center py-1">
                      <span className="text-gray-700 font-medium">VP:</span>
                      <code className="bg-gray-200 px-2 py-1 rounded text-xs font-mono text-gray-800">vp/demo</code>
                    </div>
                    <div className="flex justify-between items-center py-1">
                      <span className="text-gray-700 font-medium">Director:</span>
                      <code className="bg-gray-200 px-2 py-1 rounded text-xs font-mono text-gray-800">director1/demo</code>
                    </div>
                    <div className="flex justify-between items-center py-1">
                      <span className="text-gray-700 font-medium">Manager:</span>
                      <code className="bg-gray-200 px-2 py-1 rounded text-xs font-mono text-gray-800">manager1/demo</code>
                    </div>
                    <div className="flex justify-between items-center py-1">
                      <span className="text-gray-700 font-medium">Team Lead:</span>
                      <code className="bg-gray-200 px-2 py-1 rounded text-xs font-mono text-gray-800">teamlead1/demo</code>
                    </div>
                    <div className="flex justify-between items-center py-1">
                      <span className="text-gray-700 font-medium">Team Member:</span>
                      <code className="bg-gray-200 px-2 py-1 rounded text-xs font-mono text-gray-800">demo/demo</code>
                    </div>
                    <div className="flex justify-between items-center py-1 border-t border-gray-200 mt-2 pt-2">
                      <span className="text-gray-700 font-medium">Admin:</span>
                      <code className="bg-red-100 px-2 py-1 rounded text-xs font-mono text-red-800 font-semibold">admin/admin</code>
                    </div>
                  </div>
                </div>

                <div className="bg-blue-50 rounded-lg p-4 border border-blue-200">
                  <p className="text-xs text-blue-700 leading-relaxed">
                    <strong className="block mb-2">All Accounts:</strong>
                    {'\u2022'} All passwords are <strong>&quot;demo&quot;</strong> except admin<br/>
                    {'\u2022'} Use director1, director2, manager1-3, teamlead1-5<br/>
                    {'\u2022'} Or team members: alice, bob, carol, david, eve<br/>
                    {'\u2022'} Admin password is <strong>&quot;admin&quot;</strong>
                  </p>
                </div>

                <div className="bg-green-50 rounded-lg p-4 border border-green-200">
                  <p className="text-xs text-green-700 leading-relaxed">
                    <strong className="block mb-1">What Each User Sees:</strong>
                    {'\u2022'} <strong>VP/Directors/Managers:</strong> Manager Dashboard<br/>
                    {'\u2022'} <strong>Team Leads:</strong> Team Dashboard<br/>
                    {'\u2022'} <strong>Team Members:</strong> Member Home (Survey History)<br/>
                    {'\u2022'} <strong>Admin:</strong> System Configuration
                  </p>
                </div>
              </div>
            </div>}
          </div>
        </div>
      </div>
    </div>
  );
}
