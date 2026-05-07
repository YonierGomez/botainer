import { useState, useEffect } from 'react'

interface NetworkContainer {
  id: string
  name: string
  ipv4: string
}

interface Network {
  id: string
  name: string
  driver: string
  scope: string
  containers: NetworkContainer[]
}

interface NetworkVisualizerProps {
  onClose: () => void
  getAuthHeaders: () => Record<string, string>
}

export default function NetworkVisualizer({ onClose, getAuthHeaders }: NetworkVisualizerProps) {
  const [networks, setNetworks] = useState<Network[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchNetworks()
  }, [])

  const fetchNetworks = async () => {
    setLoading(true)
    try {
      const response = await fetch('/api/networks', {
        headers: getAuthHeaders()
      })
      const result = await response.json()
      if (result.success) {
        setNetworks(result.data || [])
      }
    } catch (err) {
      console.error('Error fetching networks:', err)
    } finally {
      setLoading(false)
    }
  }

  const getDriverColor = (driver: string) => {
    switch (driver) {
      case 'bridge': return 'bg-blue-600'
      case 'host': return 'bg-purple-600'
      case 'overlay': return 'bg-green-600'
      case 'null': return 'bg-gray-600'
      default: return 'bg-gray-600'
    }
  }

  return (
    <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
      <div className="bg-gray-900 w-full sm:max-w-6xl sm:rounded-2xl shadow-2xl border border-gray-700 flex flex-col max-h-screen sm:max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <div className="flex items-center gap-3">
            <span className="text-2xl">🌐</span>
            <h2 className="text-lg font-bold text-white">Network Visualizer</h2>
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

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">
          {loading ? (
            <div className="text-center py-12">
              <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent"></div>
              <p className="mt-4 text-gray-400">Loading networks...</p>
            </div>
          ) : networks.length === 0 ? (
            <div className="text-center py-12">
              <span className="text-6xl">🌐</span>
              <p className="mt-4 text-gray-400">No networks found</p>
            </div>
          ) : (
            <div className="space-y-6">
              {networks.map(network => (
                <div key={network.id} className="bg-gray-800 rounded-xl p-6 border border-gray-700">
                  {/* Network Header */}
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex-1">
                      <div className="flex items-center gap-3 mb-2">
                        <h3 className="text-xl font-bold text-white">{network.name}</h3>
                        <span className={`px-3 py-1 rounded-full text-xs font-semibold text-white ${getDriverColor(network.driver)}`}>
                          {network.driver}
                        </span>
                        <span className="px-3 py-1 rounded-full text-xs font-semibold bg-gray-700 text-gray-300">
                          {network.scope}
                        </span>
                      </div>
                      <p className="text-sm text-gray-400">ID: {network.id}</p>
                    </div>
                    <div className="text-right">
                      <p className="text-2xl font-bold text-blue-400">{network.containers.length}</p>
                      <p className="text-xs text-gray-500">containers</p>
                    </div>
                  </div>

                  {/* Containers */}
                  {network.containers.length > 0 ? (
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                      {network.containers.map(container => (
                        <div
                          key={container.id}
                          className="bg-gray-900 rounded-lg p-3 border border-gray-700 hover:border-blue-500 transition-colors"
                        >
                          <div className="flex items-center gap-2 mb-1">
                            <span className="text-lg">🐳</span>
                            <p className="font-semibold text-white text-sm truncate flex-1">
                              {container.name}
                            </p>
                          </div>
                          <p className="text-xs text-gray-500 mb-1">ID: {container.id}</p>
                          <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-400">IP:</span>
                            <code className="text-xs text-blue-400 bg-gray-800 px-2 py-1 rounded">
                              {container.ipv4.split('/')[0]}
                            </code>
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="text-center py-6 bg-gray-900 rounded-lg border border-gray-700">
                      <p className="text-gray-500 text-sm">No containers connected</p>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-gray-700 bg-gray-800/50">
          <div className="flex items-center justify-between mb-3">
            <p className="text-sm text-gray-400">Network Drivers:</p>
            <div className="flex gap-2">
              <span className="px-2 py-1 rounded text-xs font-semibold bg-blue-600 text-white">bridge</span>
              <span className="px-2 py-1 rounded text-xs font-semibold bg-purple-600 text-white">host</span>
              <span className="px-2 py-1 rounded text-xs font-semibold bg-green-600 text-white">overlay</span>
            </div>
          </div>
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
