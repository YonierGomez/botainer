import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react'

const HistoricalCharts = lazy(() => import('./components/HistoricalCharts'))
const ExportMetrics = lazy(() => import('./components/ExportMetrics'))
const AlertsManager = lazy(() => import('./components/AlertsManager'))
const ComposeManager = lazy(() => import('./components/ComposeManager'))
const ContainerCreator = lazy(() => import('./components/ContainerCreator'))
const NetworkVisualizer = lazy(() => import('./components/NetworkVisualizer'))
const UserManager = lazy(() => import('./components/UserManager'))
const TemplateLibrary = lazy(() => import('./components/TemplateLibrary'))

const ModalFallback = () => (
  <div className="fixed inset-0 bg-black/80 z-50 flex items-center justify-center">
    <div className="animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent" />
  </div>
)

// Version: 2.1.1 - Added Export Metrics

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
  const [selectedContainerLogs, setSelectedContainerLogs] = useState<Container | null>(null)
  const [selectedContainerStats, setSelectedContainerStats] = useState<Container | null>(null)
  const [selectedContainerCharts, setSelectedContainerCharts] = useState<Container | null>(null)
  const [selectedContainerInspect, setSelectedContainerInspect] = useState<Container | null>(null)
  const [inspectData, setInspectData] = useState<any>(null)
  const [loadingInspect, setLoadingInspect] = useState(false)
  const [showExportMetrics, setShowExportMetrics] = useState(false)
  const [showAlerts, setShowAlerts] = useState(false)
  const [showCompose, setShowCompose] = useState(false)
  const [showCreator, setShowCreator] = useState(false)
  const [showNetworks, setShowNetworks] = useState(false)
  const [showUsers, setShowUsers] = useState(false)
  const [showTemplates, setShowTemplates] = useState(false)
  const [showMenu, setShowMenu] = useState(false)
  const [bulkMode, setBulkMode] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [logs, setLogs] = useState<string>('')
  const [loadingLogs, setLoadingLogs] = useState(false)
  const [isInitialLoad, setIsInitialLoad] = useState(true)
  const [stats, setStats] = useState<any>(null)
  const [loadingStats, setLoadingStats] = useState(false)

  const getAuthHeaders = useCallback(() => {
    const initData = window.Telegram?.WebApp?.initData || ''
    return {
      'Content-Type': 'application/json',
      'X-Telegram-Init-Data': initData
    }
  }, [])

  const fetchContainers = useCallback(async (silent = false) => {
    try {
      if (!silent) {
        setLoading(true)
        setError(null)
      }
      
      const response = await fetch('/api/containers', {
        headers: getAuthHeaders()
      })
      
      // Try to parse JSON, catch if it fails
      let result
      try {
        result = await response.json()
      } catch (jsonError) {
        // Not JSON - probably HTML error page
        if (!silent) {
          setError('Please open this app from Telegram bot')
        }
        return
      }
      
      // Check response status after parsing
      if (!response.ok || !result.success) {
        if (!silent) {
          setError(result.error || 'Please open this app from Telegram bot')
        }
        return
      }
      
      // Success - update containers
      setContainers(result.data || [])
      if (isInitialLoad) setIsInitialLoad(false)
      
    } catch (err) {
      console.error('Fetch error:', err)
      if (!silent) {
        setError('Connection error. Please try again.')
      }
    } finally {
      if (!silent) {
        setLoading(false)
      }
    }
  }, [getAuthHeaders, isInitialLoad])

  useEffect(() => {
    let interval: ReturnType<typeof setInterval> | undefined
    let telegramCheckTimeout: ReturnType<typeof setTimeout> | undefined

    // Wait for Telegram WebApp SDK to load
    const checkTelegram = () => {
      if (!window.Telegram?.WebApp) {
        telegramCheckTimeout = setTimeout(checkTelegram, 100)
        return
      }

      // Check if running in Telegram
      if (!window.Telegram.WebApp.initData) {
        setLoading(false)
        setError('⚠️ Please open this app from Telegram bot\n\nGo to @botainerbot → /start → 🐳 Dashboard')
        return
      }

      window.Telegram.WebApp.ready()
      window.Telegram.WebApp.expand()
      fetchContainers()

      // Auto-refresh every 10 seconds (silent)
      interval = setInterval(() => fetchContainers(true), 10000)
    }

    checkTelegram()

    return () => {
      if (interval) clearInterval(interval)
      if (telegramCheckTimeout) clearTimeout(telegramCheckTimeout)
    }
  }, [fetchContainers])

  const handleAction = async (id: string, action: 'start' | 'stop' | 'restart' | 'delete') => {
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

  const handleBulkAction = async (action: 'start' | 'stop' | 'restart' | 'delete') => {
    if (selectedIds.size === 0) return
    
    try {
      const response = await fetch('/api/bulk', {
        method: 'POST',
        headers: {
          ...getAuthHeaders(),
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          container_ids: Array.from(selectedIds),
          action
        })
      })
      const result = await response.json()
      
      if (result.success) {
        setSelectedIds(new Set())
        setBulkMode(false)
        fetchContainers()
      }
    } catch (err) {
      alert('Bulk action failed')
    }
  }

  const toggleSelection = (id: string) => {
    const newSet = new Set(selectedIds)
    if (newSet.has(id)) {
      newSet.delete(id)
    } else {
      newSet.add(id)
    }
    setSelectedIds(newSet)
  }

  const selectAll = () => {
    setSelectedIds(new Set(filteredContainers.map(c => c.Id)))
  }

  const deselectAll = () => {
    setSelectedIds(new Set())
  }

  const fetchLogs = async (container: Container) => {
    setSelectedContainerLogs(container)
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

  const fetchInspect = async (container: Container) => {
    setSelectedContainerInspect(container)
    setLoadingInspect(true)
    setInspectData(null)
    
    try {
      const response = await fetch(`/api/containers/${container.Id}`, {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      
      if (result.success) {
        setInspectData(result.data)
      } else {
        setInspectData({ error: result.error || 'Failed to inspect container' })
      }
    } catch (err) {
      setInspectData({ error: err instanceof Error ? err.message : 'Unknown error' })
    } finally {
      setLoadingInspect(false)
    }
  }

  const handleCheckUpdates = async () => {
    try {
      const response = await fetch('/api/updates/check', {
        method: 'POST',
        headers: getAuthHeaders()
      })
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }
      
      const result = await response.json()
      
      if (result.success) {
        const updates = result.data.filter((u: any) => u.has_update)
        if (updates.length > 0) {
          alert(`✅ Found ${updates.length} update(s):\n\n${updates.map((u: any) => `• ${u.container_name}\n  ${u.current_image}`).join('\n\n')}`)
        } else {
          alert('✅ All containers are up to date!')
        }
      } else {
        alert('❌ Error: ' + (result.error || 'Unknown error'))
      }
    } catch (err) {
      alert('❌ Failed:\n' + (err instanceof Error ? err.message : 'Unknown error'))
    }
  }

  const colorizeLog = useCallback((line: string) => {
    const lower = line.toLowerCase()
    
    // Error patterns
    if (lower.includes('error') || lower.includes('exception') || lower.includes('fatal') || 
        lower.includes('panic') || lower.includes('failed') || lower.includes('failure')) {
      return 'text-red-400'
    }
    // Warning patterns
    if (lower.includes('warn') || lower.includes('warning') || lower.includes('deprecated')) {
      return 'text-yellow-400'
    }
    // Success patterns
    if (lower.includes('success') || lower.includes('complete') || lower.includes('started') ||
        lower.includes('listening') || lower.includes('ready')) {
      return 'text-green-400'
    }
    // Info patterns
    if (lower.includes('info') || lower.includes('debug')) {
      return 'text-blue-400'
    }
    
    return 'text-gray-300'
  }, [])

  const fetchStats = async (container: Container) => {
    setSelectedContainerStats(container)
    setLoadingStats(true)
    setStats(null)
    
    try {
      const response = await fetch(`/api/containers/${container.Id}/stats`, {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      
      if (result.success) {
        setStats(result.data)
      } else {
        setStats({ error: result.error || 'Failed to fetch stats' })
      }
    } catch (err) {
      setStats({ error: err instanceof Error ? err.message : 'Unknown error' })
    } finally {
      setLoadingStats(false)
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

  const runningCount = useMemo(() => containers.filter(c => c.State === 'running').length, [containers])
  const stoppedCount = useMemo(() => containers.filter(c => c.State === 'exited').length, [containers])

  const filteredContainers = useMemo(() => containers.filter(container => {
    const matchesSearch = searchQuery === '' || 
      container.Names[0]?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      container.Image.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesState = filterState === 'all' ||
      (filterState === 'running' && container.State === 'running') ||
      (filterState === 'stopped' && container.State === 'exited')
    return matchesSearch && matchesState
  }), [containers, searchQuery, filterState])

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
            onClick={() => fetchContainers(false)}
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
      {/* Header - Mobile Optimized */}
      <div className="bg-gray-800/95 backdrop-blur-md border-b border-gray-700/50 sticky top-0 z-50 shadow-lg">
        <div className="px-3 py-3">
          <div className="flex items-center justify-between">
            {/* Logo */}
            <div className="flex items-center gap-2">
              <div className="text-2xl">🐳</div>
              <div>
                <h1 className="text-base font-bold text-white">Botainer</h1>
                <p className="text-[10px] text-gray-400">Docker Manager</p>
              </div>
            </div>

            {/* Actions */}
            <div className="flex items-center gap-1">
              <button
                onClick={() => fetchContainers(false)}
                className="p-2 hover:bg-gray-700 rounded-lg transition-colors relative"
                title="Refresh"
              >
                <svg className="w-4 h-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                <span className="absolute top-0 right-0 w-2 h-2 bg-emerald-500 rounded-full animate-pulse"></span>
              </button>

              <button
                onClick={() => {
                  setBulkMode(!bulkMode)
                  setSelectedIds(new Set())
                }}
                className={`p-2 rounded-lg transition-colors ${
                  bulkMode ? 'bg-blue-600 text-white' : 'hover:bg-gray-700 text-gray-300'
                }`}
                title="Bulk"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
                </svg>
              </button>

              <div className="relative">
                <button
                  onClick={() => setShowMenu(!showMenu)}
                  className="p-2 hover:bg-gray-700 rounded-lg transition-colors"
                >
                  <svg className="w-5 h-5 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                  </svg>
                </button>

                {showMenu && (
                  <>
                    <div className="fixed inset-0 z-40" onClick={() => setShowMenu(false)}></div>
                    <div className="absolute right-0 mt-2 w-48 bg-gray-800 rounded-lg shadow-xl border border-gray-700 py-1 z-50">
                      <button onClick={() => { setShowCreator(true); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>➕</span> Create Container
                      </button>
                      <button onClick={() => { setShowCompose(true); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>🐳</span> Compose Manager
                      </button>
                      <button onClick={() => { setShowNetworks(true); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>🌐</span> Networks
                      </button>
                      <div className="border-t border-gray-700 my-1"></div>
                      <button onClick={() => { setShowTemplates(true); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>📦</span> Templates
                      </button>
                      <button onClick={() => { setShowUsers(true); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>👥</span> Users
                      </button>
                      <div className="border-t border-gray-700 my-1"></div>
                      <button onClick={() => { setShowAlerts(true); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>🚨</span> Alerts
                      </button>
                      <button onClick={() => { setShowExportMetrics(true); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>📥</span> Export Metrics
                      </button>
                      <button onClick={() => { handleCheckUpdates(); setShowMenu(false); }} className="w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700 flex items-center gap-2">
                        <span>🔄</span> Check Updates
                      </button>
                    </div>
                  </>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="max-w-6xl mx-auto px-3 py-3 space-y-3">
        {/* Search and Filters - Inline */}
        <div className="flex items-center gap-2 flex-wrap">
          {/* Search Bar */}
          <div className="relative flex-1 min-w-[200px]">
            <input
              type="text"
              placeholder="Search..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full px-3 py-2 pl-9 bg-gray-800/80 border border-gray-700/50 rounded-lg text-sm text-white placeholder-gray-500 focus:outline-none focus:border-blue-500 transition-colors"
            />
            <svg className="absolute left-2.5 top-2.5 w-4 h-4 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            {searchQuery && (
              <button
                onClick={() => setSearchQuery('')}
                className="absolute right-2.5 top-2.5 text-gray-500 hover:text-gray-300"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>

          {/* Filter Chips */}
          <button
            onClick={() => setFilterState('all')}
            className={`px-3 py-2 rounded-lg text-xs font-semibold transition-colors ${
              filterState === 'all'
                ? 'bg-blue-600 text-white'
                : 'bg-gray-800/80 text-gray-400 hover:bg-gray-700'
            }`}
          >
            All {containers.length}
          </button>
          <button
            onClick={() => setFilterState('running')}
            className={`px-3 py-2 rounded-lg text-xs font-semibold transition-colors ${
              filterState === 'running'
                ? 'bg-emerald-600 text-white'
                : 'bg-gray-800/80 text-gray-400 hover:bg-gray-700'
            }`}
          >
            🟢 {runningCount}
          </button>
          <button
            onClick={() => setFilterState('stopped')}
            className={`px-3 py-2 rounded-lg text-xs font-semibold transition-colors ${
              filterState === 'stopped'
                ? 'bg-red-600 text-white'
                : 'bg-gray-800/80 text-gray-400 hover:bg-gray-700'
            }`}
          >
            🔴 {stoppedCount}
          </button>
        </div>

        {/* Containers List */}
        <div className="space-y-2">
          {filteredContainers.length === 0 ? (
            <div className="bg-gray-800/80 rounded-lg p-8 text-center border border-gray-700/50">
              <div className="text-4xl mb-3">
                {searchQuery || filterState !== 'all' ? '🔍' : '📦'}
              </div>
              <p className="text-sm text-gray-400 font-medium">
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
                  className="mt-3 px-4 py-2 bg-blue-600 text-white text-sm rounded-lg font-semibold hover:bg-blue-700 transition-colors"
                >
                  Clear filters
                </button>
              )}
            </div>
          ) : (
            <>
              {/* Bulk Actions Bar */}
              {bulkMode && (
                <div className="bg-blue-900/30 border border-blue-700/50 rounded-lg p-3 mb-2">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-white text-sm font-semibold">
                      {selectedIds.size} selected
                    </span>
                    <div className="flex gap-2">
                      <button
                        onClick={selectAll}
                        className="text-xs text-blue-400 hover:text-blue-300"
                      >
                        All
                      </button>
                      <button
                        onClick={deselectAll}
                        className="text-xs text-blue-400 hover:text-blue-300"
                      >
                        None
                      </button>
                    </div>
                  </div>
                  {selectedIds.size > 0 && (
                    <div className="flex gap-1.5">
                      <button
                        onClick={() => handleBulkAction('start')}
                        className="flex-1 px-2 py-1.5 bg-emerald-600 text-white rounded text-xs font-semibold hover:bg-emerald-700 transition-colors"
                      >
                        ▶️ Start
                      </button>
                      <button
                        onClick={() => handleBulkAction('restart')}
                        className="flex-1 px-2 py-1.5 bg-amber-600 text-white rounded text-xs font-semibold hover:bg-amber-700 transition-colors"
                      >
                        🔄 Restart
                      </button>
                      <button
                        onClick={() => handleBulkAction('stop')}
                        className="flex-1 px-2 py-1.5 bg-orange-600 text-white rounded text-xs font-semibold hover:bg-orange-700 transition-colors"
                      >
                        ⏹️ Stop
                      </button>
                      <button
                        onClick={() => {
                          if (confirm(`Delete ${selectedIds.size} containers?`)) {
                            handleBulkAction('delete')
                          }
                        }}
                        className="flex-1 px-2 py-1.5 bg-red-600 text-white rounded text-xs font-semibold hover:bg-red-700 transition-colors"
                      >
                        🗑️ Delete
                      </button>
                    </div>
                  )}
                </div>
              )}

              {filteredContainers.map((container) => (
              <div
                key={container.Id}
                className="bg-gray-800/80 backdrop-blur-sm rounded-lg border border-gray-700/50 p-3 hover:border-gray-600 transition-all"
              >
                <div className="flex items-start gap-3">
                  {bulkMode && (
                    <input
                      type="checkbox"
                      checked={selectedIds.has(container.Id)}
                      onChange={() => toggleSelection(container.Id)}
                      className="mt-1 w-4 h-4 rounded border-gray-600 text-blue-600 focus:ring-blue-500"
                    />
                  )}
                  
                  <div className="flex-1 min-w-0">
                    {/* Header */}
                    <div className="flex items-center gap-2 mb-1.5">
                      <div className={`w-2 h-2 rounded-full ${getStatusColor(container.State)} flex-shrink-0`}></div>
                      <span className="text-base">{getStatusIcon(container.State)}</span>
                      <h3 className="font-bold text-white text-sm truncate flex-1">
                        {container.Names[0]?.replace('/', '')}
                      </h3>
                    </div>
                    
                    {/* Info */}
                    <p className="text-xs text-gray-400 truncate mb-1">{container.Image}</p>
                    <p className="text-[10px] text-gray-500 mb-2">{container.Status}</p>
                    
                    {/* Actions */}
                    {!bulkMode && (
                      <div className="flex flex-wrap gap-1.5">
                        {container.State === 'running' ? (
                          <>
                            <button
                              onClick={() => fetchStats(container)}
                              className="px-2.5 py-1 text-[11px] bg-purple-600 text-white rounded font-semibold hover:bg-purple-700 transition-colors"
                            >
                              📊 Stats
                            </button>
                            <button
                              onClick={() => setSelectedContainerCharts(container)}
                              className="px-2.5 py-1 text-[11px] bg-indigo-600 text-white rounded font-semibold hover:bg-indigo-700 transition-colors"
                            >
                              📈 Charts
                            </button>
                            <button
                              onClick={() => fetchLogs(container)}
                              className="px-2.5 py-1 text-[11px] bg-blue-600 text-white rounded font-semibold hover:bg-blue-700 transition-colors"
                            >
                              📋 Logs
                            </button>
                            <button
                              onClick={() => fetchInspect(container)}
                              className="px-2.5 py-1 text-[11px] bg-cyan-600 text-white rounded font-semibold hover:bg-cyan-700 transition-colors"
                            >
                              🔍 Inspect
                            </button>
                            <button
                              onClick={() => handleAction(container.Id, 'restart')}
                              className="px-2.5 py-1 text-[11px] bg-amber-600 text-white rounded font-semibold hover:bg-amber-700 transition-colors"
                            >
                              🔄 Restart
                            </button>
                            <button
                              onClick={() => handleAction(container.Id, 'stop')}
                              className="px-2.5 py-1 text-[11px] bg-orange-600 text-white rounded font-semibold hover:bg-orange-700 transition-colors"
                            >
                              ⏹️ Stop
                            </button>
                            <button
                              onClick={() => {
                                if (confirm(`Delete ${container.Names[0]?.replace('/', '')}?`)) {
                                  handleAction(container.Id, 'delete')
                                }
                              }}
                              className="px-2.5 py-1 text-[11px] bg-red-600 text-white rounded font-semibold hover:bg-red-700 transition-colors"
                            >
                              🗑️ Delete
                            </button>
                          </>
                        ) : (
                          <>
                            <button
                              onClick={() => fetchLogs(container)}
                              className="px-2.5 py-1 text-[11px] bg-blue-600 text-white rounded font-semibold hover:bg-blue-700 transition-colors"
                            >
                              📋 Logs
                            </button>
                            <button
                              onClick={() => fetchInspect(container)}
                              className="px-2.5 py-1 text-[11px] bg-cyan-600 text-white rounded font-semibold hover:bg-cyan-700 transition-colors"
                            >
                              🔍 Inspect
                            </button>
                            <button
                              onClick={() => handleAction(container.Id, 'start')}
                              className="px-2.5 py-1 text-[11px] bg-emerald-600 text-white rounded font-semibold hover:bg-emerald-700 transition-colors"
                            >
                              ▶️ Start
                            </button>
                            <button
                              onClick={() => {
                                if (confirm(`Delete ${container.Names[0]?.replace('/', '')}?`)) {
                                  handleAction(container.Id, 'delete')
                                }
                              }}
                              className="px-2.5 py-1 text-[11px] bg-red-600 text-white rounded font-semibold hover:bg-red-700 transition-colors"
                            >
                              🗑️ Delete
                            </button>
                          </>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            ))}
            </>
          )}
        </div>
      </div>

      {/* Stats Modal */}
      {selectedContainerStats && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
          <div className="bg-gray-900 w-full sm:max-w-2xl sm:rounded-2xl shadow-2xl border border-gray-700">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b border-gray-700">
              <div className="flex items-center gap-3">
                <span className="text-2xl">📊</span>
                <div>
                  <h2 className="text-lg font-bold text-white">Container Stats</h2>
                  <p className="text-sm text-gray-400">{selectedContainerStats.Names[0]?.replace('/', '')}</p>
                </div>
              </div>
              <button
                onClick={() => setSelectedContainerStats(null)}
                className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
              >
                <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* Stats Content */}
            <div className="p-6">
              {loadingStats ? (
                <div className="flex items-center justify-center py-12">
                  <div className="text-center">
                    <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent"></div>
                    <p className="mt-4 text-gray-400">Loading stats...</p>
                  </div>
                </div>
              ) : stats?.error ? (
                <div className="text-center py-12">
                  <div className="text-6xl mb-4">⚠️</div>
                  <p className="text-red-400">{stats.error}</p>
                </div>
              ) : stats ? (
                <div className="space-y-6">
                  {/* CPU */}
                  <div>
                    <div className="flex justify-between mb-2">
                      <span className="text-gray-300 font-medium">CPU Usage</span>
                      <span className="text-white font-bold">{stats.cpu_percent?.toFixed(2)}%</span>
                    </div>
                    <div className="w-full bg-gray-800 rounded-full h-4 overflow-hidden">
                      <div 
                        className="bg-gradient-to-r from-blue-500 to-purple-500 h-full transition-all duration-300"
                        style={{ width: `${Math.min(stats.cpu_percent || 0, 100)}%` }}
                      ></div>
                    </div>
                  </div>

                  {/* Memory */}
                  <div>
                    <div className="flex justify-between mb-2">
                      <span className="text-gray-300 font-medium">Memory Usage</span>
                      <span className="text-white font-bold">
                        {stats.memory_usage?.toFixed(2)} GB / {stats.memory_limit?.toFixed(2)} GB
                        ({stats.memory_percent?.toFixed(1)}%)
                      </span>
                    </div>
                    <div className="w-full bg-gray-800 rounded-full h-4 overflow-hidden">
                      <div 
                        className="bg-gradient-to-r from-emerald-500 to-teal-500 h-full transition-all duration-300"
                        style={{ width: `${Math.min(stats.memory_percent || 0, 100)}%` }}
                      ></div>
                    </div>
                  </div>
                </div>
              ) : null}
            </div>

            {/* Footer */}
            <div className="p-4 border-t border-gray-700 flex gap-2">
              <button
                onClick={() => fetchStats(selectedContainerStats)}
                className="flex-1 px-4 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors"
              >
                🔄 Refresh
              </button>
              <button
                onClick={() => setSelectedContainerStats(null)}
                className="flex-1 px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Logs Modal */}
      {selectedContainerLogs && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
          <div className="bg-gray-900 w-full sm:max-w-4xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b border-gray-700">
              <div className="flex items-center gap-3">
                <span className="text-2xl">📋</span>
                <div>
                  <h2 className="text-lg font-bold text-white">Container Logs</h2>
                  <p className="text-sm text-gray-400">{selectedContainerLogs.Names[0]?.replace('/', '')}</p>
                </div>
              </div>
              <button
                onClick={() => setSelectedContainerLogs(null)}
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
                <div className="text-xs sm:text-sm font-mono bg-gray-950 p-4 rounded-xl border border-gray-800">
                  {logs.split('\n').map((line, i) => (
                    <div key={i} className={colorizeLog(line)}>
                      {line || '\u00A0'}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="p-4 border-t border-gray-700 flex gap-2">
              <button
                onClick={() => fetchLogs(selectedContainerLogs)}
                className="flex-1 px-4 py-3 bg-blue-600 text-white rounded-xl font-semibold hover:bg-blue-700 transition-colors"
              >
                🔄 Refresh
              </button>
              <button
                onClick={() => setSelectedContainerLogs(null)}
                className="flex-1 px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Inspect Modal */}
      {selectedContainerInspect && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
          <div className="bg-gray-900 w-full sm:max-w-4xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b border-gray-700">
              <div className="flex items-center gap-3">
                <span className="text-2xl">🔍</span>
                <div>
                  <h2 className="text-lg font-bold text-white">Container Inspect</h2>
                  <p className="text-sm text-gray-400">{selectedContainerInspect.Names[0]?.replace('/', '')}</p>
                </div>
              </div>
              <button
                onClick={() => setSelectedContainerInspect(null)}
                className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
              >
                <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* Inspect Content */}
            <div className="flex-1 overflow-auto p-4">
              {loadingInspect ? (
                <div className="flex items-center justify-center h-full">
                  <div className="text-center">
                    <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-cyan-500 border-t-transparent"></div>
                    <p className="mt-4 text-gray-400">Loading inspect data...</p>
                  </div>
                </div>
              ) : (
                <div className="text-xs sm:text-sm font-mono bg-gray-950 p-4 rounded-xl border border-gray-800">
                  <pre className="text-gray-300 whitespace-pre-wrap break-words">
                    {JSON.stringify(inspectData, null, 2)}
                  </pre>
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="p-4 border-t border-gray-700 flex gap-2">
              <button
                onClick={() => fetchInspect(selectedContainerInspect)}
                className="flex-1 px-4 py-3 bg-cyan-600 text-white rounded-xl font-semibold hover:bg-cyan-700 transition-colors"
              >
                🔄 Refresh
              </button>
              <button
                onClick={() => setSelectedContainerInspect(null)}
                className="flex-1 px-4 py-3 bg-gray-800 text-white rounded-xl font-semibold hover:bg-gray-700 transition-colors"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Historical Charts Modal */}
      {selectedContainerCharts && (
        <Suspense fallback={<ModalFallback />}>
          <HistoricalCharts
            containerId={selectedContainerCharts.Id.substring(0, 12)}
            containerName={selectedContainerCharts.Names[0]?.replace('/', '')}
            onClose={() => setSelectedContainerCharts(null)}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}

      {/* Export Metrics Modal */}
      {showExportMetrics && (
        <Suspense fallback={<ModalFallback />}>
          <ExportMetrics
            onClose={() => setShowExportMetrics(false)}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}

      {/* Alerts Manager Modal */}
      {showAlerts && (
        <Suspense fallback={<ModalFallback />}>
          <AlertsManager
            containers={containers}
            onClose={() => setShowAlerts(false)}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}

      {/* Compose Manager Modal */}
      {showCompose && (
        <Suspense fallback={<ModalFallback />}>
          <ComposeManager
            onClose={() => setShowCompose(false)}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}

      {/* Container Creator Modal */}
      {showCreator && (
        <Suspense fallback={<ModalFallback />}>
          <ContainerCreator
            onClose={() => setShowCreator(false)}
            onSuccess={() => fetchContainers()}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}

      {/* Network Visualizer Modal */}
      {showNetworks && (
        <Suspense fallback={<ModalFallback />}>
          <NetworkVisualizer
            onClose={() => setShowNetworks(false)}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}

      {/* User Manager Modal */}
      {showUsers && (
        <Suspense fallback={<ModalFallback />}>
          <UserManager
            onClose={() => setShowUsers(false)}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}

      {/* Template Library Modal */}
      {showTemplates && (
        <Suspense fallback={<ModalFallback />}>
          <TemplateLibrary
            onClose={() => setShowTemplates(false)}
            getAuthHeaders={getAuthHeaders}
          />
        </Suspense>
      )}
    </div>
  )
}

export default App
