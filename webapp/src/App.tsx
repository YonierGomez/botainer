import { useEffect, useState } from 'react'

declare global {
  interface Window {
    Telegram?: {
      WebApp: {
        ready: () => void
        expand: () => void
        MainButton: {
          setText: (text: string) => void
          show: () => void
          hide: () => void
        }
        initData: string
        initDataUnsafe: {
          user?: {
            id: number
            first_name: string
            last_name?: string
            username?: string
          }
        }
      }
    }
  }
}

interface Container {
  Id: string
  Names: string[]
  Image: string
  State: string
  Status: string
}

function App() {
  const [containers, setContainers] = useState<Container[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (window.Telegram?.WebApp) {
      window.Telegram.WebApp.ready()
      window.Telegram.WebApp.expand()
    }
    fetchContainers()
  }, [])

  const getAuthHeaders = () => {
    const initData = window.Telegram?.WebApp?.initData || ''
    return {
      'Content-Type': 'application/json',
      'X-Telegram-Init-Data': initData
    }
  }

  const fetchContainers = async () => {
    try {
      setLoading(true)
      const response = await fetch('/api/containers', {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      
      if (result.success) {
        setContainers(result.data)
      } else {
        setError(result.error || 'Failed to fetch containers')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  const handleAction = async (id: string, action: 'start' | 'stop' | 'restart') => {
    try {
      const response = await fetch(`/api/containers/${id}/${action}`, {
        method: 'POST',
        headers: getAuthHeaders()
      })
      const result = await response.json()
      
      if (result.success) {
        fetchContainers()
      } else {
        alert(result.error || 'Action failed')
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  const getStatusColor = (state: string) => {
    switch (state.toLowerCase()) {
      case 'running':
        return 'bg-emerald-500'
      case 'paused':
        return 'bg-amber-500'
      default:
        return 'bg-red-500'
    }
  }

  const getStatusIcon = (state: string) => {
    switch (state.toLowerCase()) {
      case 'running':
        return '🟢'
      case 'paused':
        return '🟡'
      default:
        return '🔴'
    }
  }

  const runningCount = containers.filter(c => c.State === 'running').length
  const stoppedCount = containers.filter(c => c.State === 'exited').length

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-gray-900 via-slate-900 to-gray-800 flex items-center justify-center p-4">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-16 w-16 border-4 border-blue-500 border-t-transparent"></div>
          <p className="mt-4 text-gray-300 font-medium">Loading containers...</p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-gray-900 via-slate-900 to-gray-800 flex items-center justify-center p-4">
        <div className="bg-gray-800 rounded-2xl shadow-xl p-8 max-w-md text-center border border-gray-700">
          <div className="text-6xl mb-4">⚠️</div>
          <h2 className="text-xl font-bold text-white mb-2">Error</h2>
          <p className="text-red-400 mb-6">{error}</p>
          <button
            onClick={fetchContainers}
            className="px-6 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors shadow-lg"
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-gray-900 via-slate-900 to-gray-800">
      {/* Header */}
      <div className="bg-gray-800/80 backdrop-blur-sm border-b border-gray-700 sticky top-0 z-10">
        <div className="max-w-4xl mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-3">
              <div className="text-3xl">🐳</div>
              <div>
                <h1 className="text-xl font-bold text-white">Botainer</h1>
                <p className="text-xs text-gray-400">Docker Management</p>
              </div>
            </div>
            <button
              onClick={fetchContainers}
              className="p-2 hover:bg-gray-700 rounded-lg transition-colors"
              title="Refresh"
            >
              <svg className="w-5 h-5 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>
          </div>
        </div>
      </div>

      <div className="max-w-4xl mx-auto px-4 py-6 space-y-6">
        {/* Stats Cards */}
        <div className="grid grid-cols-2 gap-4">
          <div className="bg-gray-800 rounded-2xl shadow-xl p-4 border border-gray-700">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-400 font-medium">Running</p>
                <p className="text-3xl font-bold text-emerald-400">{runningCount}</p>
              </div>
              <div className="text-4xl">🟢</div>
            </div>
          </div>
          <div className="bg-gray-800 rounded-2xl shadow-xl p-4 border border-gray-700">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-400 font-medium">Stopped</p>
                <p className="text-3xl font-bold text-red-400">{stoppedCount}</p>
              </div>
              <div className="text-4xl">🔴</div>
            </div>
          </div>
        </div>

        {/* Containers List */}
        <div className="space-y-3">
          {containers.length === 0 ? (
            <div className="bg-gray-800 rounded-2xl shadow-xl p-12 text-center border border-gray-700">
              <div className="text-6xl mb-4">📦</div>
              <p className="text-gray-400 font-medium">No containers found</p>
            </div>
          ) : (
            containers.map((container) => (
              <div
                key={container.Id}
                className="bg-gray-800 rounded-2xl shadow-xl p-4 border border-gray-700 hover:border-gray-600 transition-all"
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="flex items-start space-x-3 flex-1 min-w-0">
                    <div className="flex-shrink-0 mt-1">
                      <div className={`w-3 h-3 rounded-full ${getStatusColor(container.State)} shadow-lg`}></div>
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="text-lg">{getStatusIcon(container.State)}</span>
                        <h3 className="font-bold text-white truncate">
                          {container.Names[0]?.replace('/', '')}
                        </h3>
                      </div>
                      <p className="text-sm text-gray-300 truncate mb-1">{container.Image}</p>
                      <p className="text-xs text-gray-500">{container.Status}</p>
                    </div>
                  </div>

                  <div className="flex flex-col gap-2 flex-shrink-0">
                    {container.State === 'running' ? (
                      <>
                        <button
                          onClick={() => handleAction(container.Id, 'restart')}
                          className="px-4 py-2 text-sm bg-amber-600 text-white rounded-xl font-semibold hover:bg-amber-700 transition-colors shadow-lg whitespace-nowrap"
                        >
                          🔄 Restart
                        </button>
                        <button
                          onClick={() => handleAction(container.Id, 'stop')}
                          className="px-4 py-2 text-sm bg-red-600 text-white rounded-xl font-semibold hover:bg-red-700 transition-colors shadow-lg whitespace-nowrap"
                        >
                          ⏹️ Stop
                        </button>
                      </>
                    ) : (
                      <button
                        onClick={() => handleAction(container.Id, 'start')}
                        className="px-4 py-2 text-sm bg-emerald-600 text-white rounded-xl font-semibold hover:bg-emerald-700 transition-colors shadow-lg whitespace-nowrap"
                      >
                        ▶️ Start
                      </button>
                    )}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

export default App
