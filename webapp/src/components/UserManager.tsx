import React, { useState, useEffect } from 'react';

interface User {
  id: string;
  username: string;
  role: 'admin' | 'operator' | 'viewer';
  created_at: string;
  last_seen: string;
}

interface AuditLog {
  id: string;
  user_id: string;
  username: string;
  action: string;
  resource: string;
  success: boolean;
  timestamp: string;
  details?: string;
}

interface UserManagerProps {
  onClose: () => void;
  getAuthHeaders: () => Record<string, string>;
}

const UserManager: React.FC<UserManagerProps> = ({ onClose, getAuthHeaders }) => {
  const [activeTab, setActiveTab] = useState<'users' | 'audit'>('users');
  const [users, setUsers] = useState<User[]>([]);
  const [auditLog, setAuditLog] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (activeTab === 'users') {
      fetchUsers();
    } else {
      fetchAuditLog();
    }
  }, [activeTab]);

  const fetchUsers = async () => {
    setLoading(true);
    try {
      const res = await fetch('/api/users', { headers: getAuthHeaders() });
      const data = await res.json();
      if (data.success) {
        setUsers(data.data || []);
      }
    } catch (err) {
      console.error('Failed to fetch users:', err);
    } finally {
      setLoading(false);
    }
  };

  const fetchAuditLog = async () => {
    setLoading(true);
    try {
      const res = await fetch('/api/audit?limit=100', { headers: getAuthHeaders() });
      const data = await res.json();
      if (data.success) {
        setAuditLog(data.data || []);
      }
    } catch (err) {
      console.error('Failed to fetch audit log:', err);
    } finally {
      setLoading(false);
    }
  };

  const updateRole = async (userId: string, role: string) => {
    try {
      const res = await fetch(`/api/users/${userId}/role`, {
        method: 'PUT',
        headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ role }),
      });
      const data = await res.json();
      if (data.success) {
        fetchUsers();
      }
    } catch (err) {
      console.error('Failed to update role:', err);
    }
  };

  const getRoleBadge = (role: string) => {
    const colors = {
      admin: 'bg-red-500',
      operator: 'bg-blue-500',
      viewer: 'bg-gray-500',
    };
    return (
      <span className={`px-2 py-1 rounded text-xs text-white ${colors[role as keyof typeof colors]}`}>
        {role.toUpperCase()}
      </span>
    );
  };

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleString();
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-gray-800 rounded-lg w-full max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
        <div className="p-4 border-b border-gray-700 flex justify-between items-center">
          <h2 className="text-xl font-bold text-white">👥 User Management</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-white text-2xl">×</button>
        </div>

        <div className="flex border-b border-gray-700">
          <button
            onClick={() => setActiveTab('users')}
            className={`flex-1 py-3 px-4 font-medium ${
              activeTab === 'users'
                ? 'bg-blue-600 text-white'
                : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
            }`}
          >
            Users
          </button>
          <button
            onClick={() => setActiveTab('audit')}
            className={`flex-1 py-3 px-4 font-medium ${
              activeTab === 'audit'
                ? 'bg-blue-600 text-white'
                : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
            }`}
          >
            Audit Log
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4">
          {loading ? (
            <div className="text-center py-8 text-gray-400">Loading...</div>
          ) : activeTab === 'users' ? (
            <div className="space-y-3">
              {users.length === 0 ? (
                <div className="text-center py-8 text-gray-400">No users found</div>
              ) : (
                users.map((user) => (
                  <div key={user.id} className="bg-gray-700 rounded-lg p-4">
                    <div className="flex items-center justify-between mb-2">
                      <div>
                        <div className="text-white font-medium">{user.username}</div>
                        <div className="text-gray-400 text-sm">ID: {user.id}</div>
                      </div>
                      {getRoleBadge(user.role)}
                    </div>
                    <div className="text-gray-400 text-sm mb-3">
                      <div>Created: {formatDate(user.created_at)}</div>
                      <div>Last seen: {formatDate(user.last_seen)}</div>
                    </div>
                    <div className="flex gap-2">
                      <select
                        value={user.role}
                        onChange={(e) => updateRole(user.id, e.target.value)}
                        className="flex-1 bg-gray-600 text-white px-3 py-2 rounded"
                      >
                        <option value="admin">Admin</option>
                        <option value="operator">Operator</option>
                        <option value="viewer">Viewer</option>
                      </select>
                    </div>
                  </div>
                ))
              )}
            </div>
          ) : (
            <div className="space-y-2">
              {auditLog.length === 0 ? (
                <div className="text-center py-8 text-gray-400">No audit logs</div>
              ) : (
                auditLog.map((log) => (
                  <div key={log.id} className="bg-gray-700 rounded p-3">
                    <div className="flex items-center justify-between mb-1">
                      <div className="text-white font-medium">{log.username || log.user_id}</div>
                      <div className={`text-xs px-2 py-1 rounded ${log.success ? 'bg-green-600' : 'bg-red-600'}`}>
                        {log.success ? '✓' : '✗'}
                      </div>
                    </div>
                    <div className="text-gray-300 text-sm mb-1">
                      <span className="font-medium">{log.action}</span> on <span className="text-blue-400">{log.resource}</span>
                    </div>
                    {log.details && (
                      <div className="text-gray-400 text-xs mb-1">{log.details}</div>
                    )}
                    <div className="text-gray-500 text-xs">{formatDate(log.timestamp)}</div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        <div className="p-4 border-t border-gray-700 bg-gray-750">
          <div className="text-gray-400 text-sm">
            <div className="font-medium mb-2">Role Permissions:</div>
            <div className="space-y-1">
              <div>🔴 <span className="text-red-400">Admin</span>: Full access to all features</div>
              <div>🔵 <span className="text-blue-400">Operator</span>: Manage containers (no delete, no user management)</div>
              <div>⚪ <span className="text-gray-400">Viewer</span>: View-only access (logs, stats)</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default UserManager;
