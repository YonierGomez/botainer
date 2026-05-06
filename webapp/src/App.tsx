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
  const [searchQuery, setSearchQuery] = useState('')
  const [filterState, setFilterState] = useState<'all' | 'running' | 'stopped'>('all')
  const [selectedContainer, setSelectedContainer] = useState<Container | null>(null)
  const [logs, setLogs] = useState<string>('')
  const [loadingLogs, setLoadingLogs] = useState(false)

  useEffect(() => {
    if (window.Telegram?.WebApp) {
      window.Telegram.WebApp.ready()
      window.Telegram.WebApp.expand()
    }
    fetchContainers()

    // Auto-refresh every 5 seconds
    const interval = setInterval(fetchContainers, 5000)
    return () => clearInterval(interval)
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

  const fetchLogs = async (container: Container) => {
    setSelectedContainer(container)
    setLoadingLogs(true)
    setLogs('')
    
    try {
      const response = await fetch(`/api/containers/${container.Id}/logs?tail=100`, {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      
      if (result.success) {
        setLogs(result.data || 'No logs available')
      } else {
        setLogs('Error: ' + (result.error || 'Failed to fetch logs'))
      }
    } catch (err) {
      setLogs('Error: ' + (err instanceof Error ? err.message : 'Unknown error'))
    } finally {
      setLoadingLogs(false)
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

  // Filter and search logic
  const filteredContainers = containers.filter(container => {
    // Search filter
    const matchesSearch = searchQuery === '' || 
      container.Names[0]?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      container.Image.toLowerCase().includes(searchQuery.toLowerCase())
    
    // State filter
    const matchesState = filterState === 'all' ||
      (filterState === 'running' && container.State === 'running') ||
      (filterState === 'stopped' && container.State === 'exited')
    
    return matchesSearch && matchesState
  })

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
              className="p-2 hover:bg-gray-700 rounded-lg transition-colors relative"
              title="Refresh (auto-updates every 5s)"
            >
              <svg className="w-5 h-5 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              <span className="absolute -top-1 -right-1 w-2 h-2 bg-emerald-500 rounded-full animate-pulse"></span>
            </button>
          </div>
        </div>
      </div>

      <div className="max-w-4xl mx-auto px-4 py-6 space-y-6">
        {/* Search and Filters */}
        <div className="space-y-3">
          {/* Search Bar */}
          <div className="relative">
            <input
              type="text"
              placeholder="Search containers..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full px-4 py-3 pl-11 bg-gray-800 border border-gray-700 rounded-xl text-white placeholder-gray-500 focus:outline-none focus:border-blue-500 transition-colors"
            />
            <svg className="absolute left-3 top-3.5 w-5 h-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            {searchQuery && (
              <button
                onClick={() => setSearchQuery('')}
                className="absolute right-3 top-3 text-gray-500 hover:text-gray-300"
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>

          {/* Filter Buttons */}
          <div className="flex gap-2">
            <button
              onClick={() => setFilterState('all')}
              className={`flex-1 px-4 py-2 rounded-xl font-semibold transition-colors ${
                filterState === 'all'
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
              }`}
            >
              All ({containers.length})
            </button>
            <button
              onClick={() => setFilterState('running')}
              className={`flex-1 px-4 py-2 rounded-xl font-semibold transition-colors ${
                filterState === 'running'
                  ? 'bg-emerald-600 text-white'
                  : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
              }`}
            >
              🟢 Running ({runningCount})
            </button>
            <button
              onClick={() => setFilterState('stopped')}
              className={`flex-1 px-4 py-2 rounded-xl font-semibold transition-colors ${
                filterState === 'stopped'
                  ? 'bg-red-600 text-white'
                  : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
              }`}
            >
              🔴 Stopped ({stoppedCount})
            </button>
          </div>
        </div>

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
          {filteredContainers.length === 0 ? (
            <div className="bg-gray-800 rounded-2xl shadow-xl p-12 text-center border border-gray-700">
              <div className="text-6xl mb-4">
                {searchQuery || filterState !== 'all' ? '🔍' : '📦'}
              </div>
              <p className="text-gray-400 font-medium">
                {searchQuery || filterState !== 'all' 
                  ? 'No containers match your filters' 
                  : 'No containers found'}
              </p>
              {(searchQuery || filterState !== 'all') && (
                <button
                  onClick={() => {
                    setSearchQuery('')
                    setFilterState('all')
                  }}
                  className="mt-4 px-4 py-2 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors"
                >
                  Clear filters
                </button>
              )}
            </div>
          ) : (
            filteredContainers.map((container) => (
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
                          onClick={() => fetchLogs(container)}
                          className="px-4 py-2 text-sm bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors shadow-lg whitespace-nowrap"
                        >
                          📋 Logs
                        </button>
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
                      <>
                        <button
                          onClick={() => fetchLogs(container)}
                          className="px-4 py-2 text-sm bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors shadow-lg whitespace-nowrap"
                        >
                          📋 Logs
                        </button>
                        <button
                          onClick={() => handleAction(container.Id, 'start')}
                          className="px-4 py-2 text-sm bg-emerald-600 text-white rounded-xl font-semibold hover:bg-emerald-700 transition-colors shadow-lg whitespace-nowrap"
                        >
                          ▶️ Start
                        </button>
                      </>
                    )}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {/* Logs Modal */}
      {selectedContainer && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
          <div className="bg-gray-900 w-full sm:max-w-4xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b border-gray-700">
              <div className="flex items-center gap-3">
                <span className="text-2xl">📋</span>
                <div>
                  <h2 className="text-lg font-bold text-white">Container Logs</h2>
                  <p className="text-sm text-gray-400">{selectedContainer.Names[0]?.replace('/', '')}</p>
                </div>
              </div>
              <button
                onClick={() => setSelectedContainer(null)}
                className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
              >
                <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* Logs Content */}
            <div className="flex-1 overflow-auto p-4">
              {loadingLogs ? (
                <div className="flex items-center justify-center h-full">
                  <div className="text-center">
                    <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent"></div>
                    <p className="mt-4 text-gray-400">Loading logs...</p>
                  </div>
                </div>
              ) : (
                <pre className="text-xs sm:text-sm text-gray-300 font-mono whitespace-pre-wrap break-words bg-gray-950 p-4 rounded-xl border border-gray-800">
                  {logs}
                </pre>
              )}
            </div>

            {/* Footer */}
            <div className="p-4 border-t border-gray-700 flex gap-2">
              <button
                onClick={() => fetchLogs(selectedContainer)}
                className="flex-1 px-4 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors"
              >
                🔄 Refresh
              </button>
              <button
                onClick={() => setSelectedContainer(null)}
                className="flex-1 px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default App
