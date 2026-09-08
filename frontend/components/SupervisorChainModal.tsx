'use client';

import { useState, useEffect } from 'react';
import { X, AlertCircle, Loader2 } from 'lucide-react';
import {
  getSupervisorChain,
  type SupervisorLink,
} from '@/lib/api/admin';

interface SupervisorChainModalProps {
  teamId: string;
  teamName: string;
  onClose: () => void;
}

export default function SupervisorChainModal({
  teamId,
  teamName,
  onClose,
}: SupervisorChainModalProps) {
  const [chain, setChain] = useState<SupervisorLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadData();
  }, [teamId]);

  const loadData = async () => {
    setLoading(true);
    setError(null);
    try {
      const chainRes = await getSupervisorChain(teamId);
      setChain(chainRes.supervisors || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div
        className="bg-white rounded-xl shadow-xl max-w-2xl w-full max-h-[90vh] overflow-y-auto"
        data-testid="supervisor-chain-modal"
      >
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              Supervisor Hierarchy
            </h2>
            <p className="text-sm text-gray-500 mt-1">{teamName}</p>
          </div>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
            data-testid="close-supervisor-modal"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body */}
        <div className="p-6">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin text-indigo-600" />
              <span className="ml-2 text-gray-600">Loading...</span>
            </div>
          ) : (
            <>
              {error && (
                <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
                  <AlertCircle className="w-4 h-4 text-red-600 mt-0.5" />
                  <p className="text-sm text-red-700">{error}</p>
                </div>
              )}

              <p className="text-sm text-gray-600 mb-4">
                This hierarchy is automatically derived from the team lead&apos;s reporting chain. To change it, update the &quot;Reports To&quot; field on the relevant users.
              </p>

              {chain.length === 0 ? (
                <p className="text-sm text-gray-400 italic py-4 text-center">
                  No supervisor chain found. Ensure the team lead has a &quot;Reports To&quot; user assigned.
                </p>
              ) : (
                <div className="space-y-3">
                  {chain.map((link, index) => (
                    <div
                      key={index}
                      className="flex items-center gap-3 p-3 bg-gray-50 rounded-lg"
                      data-testid="supervisor-row"
                    >
                      <span className="text-sm text-gray-500 w-6 text-right font-medium">
                        {index + 1}.
                      </span>
                      <div className="flex-1">
                        <p className="text-sm font-medium text-gray-900">
                          {link.userName || link.userId}
                        </p>
                        <p className="text-xs text-gray-500">
                          {link.levelName || link.levelId}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end p-6 border-t">
          <button
            onClick={onClose}
            className="px-4 py-2 text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
