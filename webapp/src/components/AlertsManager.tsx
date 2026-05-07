import { useState, useEffect } from 'react'

interface AlertConfig {
  container_id: string
  cpu_threshold: number
  mem_threshold: number
  enabled: boolean
}

interface Alert {
  id: string
  container_id: string
  container_name: string
  type: string
  threshold: number
  current_value: number
  triggered: boolean
  triggered_at: string
  message: string
}

interface Container {
  Id: string
  Names: string[]
  State: string
}

interface AlertsManagerProps {
  containers: Container[]
  onClose: () => void
  getAuthHeaders: () => Record<string, string>
}

export default function AlertsManager({ containers, onClose, getAuthHeaders }: AlertsManagerProps) {
  const [configs, setConfigs] = useState<AlertConfig[]>([])
  const [history, setHistory] = useState<Alert[]>([])
  const [tab, setTab] = useState<'config' | 'history'>('config')
  const [selectedContainer, setSelectedContainer] = useState('')
  const [cpuThreshold, setCpuThreshold] = useState(80)
  const [memThreshold, setMemThreshold] = useState(80)
  const [enabled, setEnabled] = useState(true)

  useEffect(() => {
    fetchConfigs()
    fetchHistory()
  }, [])

  const fetchConfigs = async () => {
    try {
      const response = await fetch('/api/alerts/configs', {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      if (result.success) {
        setConfigs(result.data || [])
      }
    } catch (err) {
      console.error('Error fetching configs:', err)
    }
  }

  const fetchHistory = async () => {
    try {
      const response = await fetch('/api/alerts/history?limit=50', {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      if (result.success) {
        setHistory(result.data || [])
      }
    } catch (err) {
      console.error('Error fetching history:', err)
    }
  }

  const handleSave = async () => {
    if (!selectedContainer) {
      alert('Select a container')
      return
    }

    try {
      await fetch('/api/alerts/configs', {
        method: 'POST',
        headers: {
          ...getAuthHeaders(),
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          container_id: selectedContainer,
          cpu_threshold: cpuThreshold,
          mem_threshold: memThreshold,
          enabled
        })
      })
      fetchConfigs()
      setSelectedContainer('')
    } catch (err) {
      console.error('Error saving config:', err)
      alert('Failed to save alert config')
    }
  }

  const handleDelete = async (containerId: string) => {
    try {
      await fetch(`/api/alerts/configs/${containerId}`, {
        method: 'DELETE',
        headers: getAuthHeaders()
      })
      fetchConfigs()
    } catch (err) {
      console.error('Error deleting config:', err)
    }
  }

  const runningContainers = containers.filter(c => c.State === 'running')

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
      <div className="bg-gray-900 w-full sm:max-w-4xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <div className="flex items-center gap-3">
            <span className="text-2xl">🚨</span>
            <h2 className="text-lg font-bold text-white">Alerts Manager</h2>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
          >
            <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-gray-700">
          <button
            onClick={() => setTab('config')}
            className={`flex-1 px-4 py-3 font-semibold transition-colors ${
              tab === 'config'
                ? 'text-blue-400 border-b-2 border-blue-400'
                : 'text-gray-400 hover:text-gray-300'
            }`}
          >
            ⚙️ Configuration
          </button>
          <button
            onClick={() => setTab('history')}
            className={`flex-1 px-4 py-3 font-semibold transition-colors ${
              tab === 'history'
                ? 'text-blue-400 border-b-2 border-blue-400'
                : 'text-gray-400 hover:text-gray-300'
            }`}
          >
            📋 History ({history.length})
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">
          {tab === 'config' ? (
            <div className="space-y-6">
              {/* New Alert Form */}
              <div className="bg-gray-800 rounded-xl p-4 space-y-4">
                <h3 className="font-semibold text-white">Create Alert</h3>
                
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">Container</label>
                  <select
                    value={selectedContainer}
                    onChange={(e) => setSelectedContainer(e.target.value)}
                    className="w-full px-4 py-3 bg-gray-900 border border-gray-700 rounded-xl text-white focus:outline-none focus:border-blue-500"
                  >
                    <option value="">Select container...</option>
                    {runningContainers.map(c => (
                      <option key={c.Id} value={c.Id.substring(0, 12)}>
                        {c.Names[0]?.replace('/', '')}
                      </option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    CPU Threshold: {cpuThreshold}%
                  </label>
                  <input
                    type="range"
                    min="0"
                    max="100"
                    value={cpuThreshold}
                    onChange={(e) => setCpuThreshold(Number(e.target.value))}
                    className="w-full"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Memory Threshold: {memThreshold}%
                  </label>
                  <input
                    type="range"
                    min="0"
                    max="100"
                    value={memThreshold}
                    onChange={(e) => setMemThreshold(Number(e.target.value))}
                    className="w-full"
                  />
                </div>

                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={enabled}
                    onChange={(e) => setEnabled(e.target.checked)}
                    className="w-5 h-5"
                  />
                  <label className="text-sm text-gray-300">Enable alerts</label>
                </div>

                <button
                  onClick={handleSave}
                  className="w-full px-4 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors"
                >
                  💾 Save Alert
                </button>
              </div>

              {/* Existing Configs */}
              <div className="space-y-2">
                <h3 className="font-semibold text-white">Active Alerts</h3>
                {configs.length === 0 ? (
                  <p className="text-gray-400 text-center py-8">No alerts configured</p>
                ) : (
                  configs.map(config => {
                    const container = containers.find(c => c.Id.startsWith(config.container_id))
                    return (
                      <div key={config.container_id} className="bg-gray-800 rounded-xl p-4 flex items-center justify-between">
                        <div>
                          <p className="font-semibold text-white">
                            {container?.Names[0]?.replace('/', '') || config.container_id}
                          </p>
                          <p className="text-sm text-gray-400">
                            CPU: {config.cpu_threshold}% | RAM: {config.mem_threshold}%
                          </p>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className={`px-3 py-1 rounded-full text-xs font-semibold ${
                            config.enabled ? 'bg-green-900 text-green-300' : 'bg-gray-700 text-gray-400'
                          }`}>
                            {config.enabled ? '✓ Enabled' : '✗ Disabled'}
                          </span>
                          <button
                            onClick={() => handleDelete(config.container_id)}
                            className="p-2 hover:bg-gray-700 rounded-lg transition-colors"
                          >
                            <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                            </svg>
                          </button>
                        </div>
                      </div>
                    )
                  })
                )}
              </div>
            </div>
          ) : (
            <div className="space-y-2">
              {history.length === 0 ? (
                <p className="text-gray-400 text-center py-8">No alerts triggered yet</p>
              ) : (
                history.map(alert => (
                  <div key={alert.id} className="bg-gray-800 rounded-xl p-4">
                    <div className="flex items-start justify-between">
                      <div className="flex-1">
                        <div className="flex items-center gap-2 mb-1">
                          <span className="text-xl">{alert.type === 'cpu' ? '⚠️' : '💾'}</span>
                          <p className="font-semibold text-white">{alert.container_name}</p>
                        </div>
                        <p className="text-sm text-gray-400 mb-2">{alert.message}</p>
                        <div className="flex gap-4 text-xs text-gray-500">
                          <span>Threshold: {alert.threshold.toFixed(1)}%</span>
                          <span>Current: {alert.current_value.toFixed(1)}%</span>
                          <span>{new Date(alert.triggered_at).toLocaleString()}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-gray-700">
          <button
            onClick={onClose}
            className="w-full px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
